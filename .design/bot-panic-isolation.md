# Bot Panic Isolation — layered containment for the remote bot stack

Why this exists: a single `panic` escaping any goroutine kills the whole
tingly-box process. The remote bot stack is the most exposed surface —
seven third-party IM SDKs, network callbacks, adapter code parsing
untrusted platform payloads, plus the agentboot executor — and one bad
update on one platform used to take down the LLM gateway, every other
bot, and every in-flight session with it.

Trigger case (2026-08): dingtalk-stream-sdk-go v0.9.1 raced its ping-timeout
goroutine against its read-error goroutine, both sending on a `closeChan`
that the process loop closes on exit → `panic: send on closed channel`
inside a goroutine **the SDK itself spawned** → whole process down. That
crash is unreachable by any recover we write, which is what forced the
layering below to be explicit about what in-process recovery can and
cannot do.

## The one rule

`recover()` only works on the goroutine that panicked. Therefore:

> Every goroutine we spawn, and every entry point a third-party SDK calls
> back into, must recover for itself. A bare `go` statement anywhere in the
> bot path is a review error.

And its corollary:

> A panic inside a goroutine that a third-party SDK spawned for itself is
> **not containable in-process**. Those are fixed at the SDK (upgrade,
> patch, or fork) — never papered over in our callers.

## The four layers

```
 L1  bot lifecycle          remote/control/bot/manager.go
     one supervised goroutine per bot (runBotSupervised): recover +
     stack log + running-map cleanup. A bot that dies does not touch
     the process or its sibling bots.
        │
 L2  event dispatch         imbot/manager.go + imbot/core/base.go
     every OnMessage/OnError/... handler runs on its own goroutine
     behind recoverHandler. A panicking consumer (remote_agent,
     notify, prompt router) drops that one event, nothing else.
        │
 L3  our goroutines + SDK-invoked callbacks          (this pass)
     · imbot/core/safego.go — SafeGo / RecoverPanic, plus BaseBot:
         RecoverCallback  SDK called us on its goroutine: contain,
                          drop the one message, keep the connection.
         RecoverLoop      our receive loop died: contain, flip to
                          disconnected, EmitDisconnected → manager
                          auto-reconnect rebuilds the connection
                          (recovery, not just not-crashing).
     · remote/safego — Go / Recover for the main module (session
       loops, scenario plugin prompts, notify module, server module).
     · agentboot/internal/safego — same for the executor module
       (decoder loop, event pump, feeder, shutdown workers).
        │
 L4  SDK-internal goroutines                 NOT containable in-process
     policy: pin a fixed version, audit its spawn points, upgrade or
     patch on any panic report. dingtalk-stream-sdk-go → v0.9.2-beta.1
     (upstream fix for exactly the closeChan race: issues #27/#28/#32;
     also adds read-loop recover + reconnect backoff).
```

## Recovery semantics — what happens after a contained panic

| Where it panicked | After recovery |
|---|---|
| Consumer / handler (L2) | one event dropped, bot keeps running |
| SDK callback (L3, `RecoverCallback`) | one message dropped, connection kept |
| Receive loop (L3, `RecoverLoop`) | bot marked disconnected, disconnect emitted, imbot manager auto-reconnects (bounded attempts) |
| Bot goroutine (L1) | bot removed from running map; next Sync/Start can bring it back |
| Interactive prompt driver (claudecode `run`, notify `runPrompt`) | interaction resolves by its own budget timeout; hook gets the fallback decision |
| agentboot pump/decoder | stream closes, run finishes with whatever was decoded; session cleanup unaffected |

The receive-loop → reconnect behavior is deliberate: a swallowed panic that
leaves a deaf-but-"running" bot is worse than a crash, because nobody
notices. Contained must never mean zombie.

## Where each helper lives (three modules, one shape)

The repo has three Go modules on the bot path, and helpers cannot cross
them without creating wrong-direction dependencies:

- `imbot/core/safego.go` — imbot module; uses `core.Logger`, and the
  Base-bot variants carry the disconnect side effect.
- `remote/safego` — main module; logrus. `remote/*` must not import
  `internal/*`, and `internal/*` may import `remote/*`, so it lives under
  `remote/` and serves both.
- `agentboot/internal/safego` — agentboot module; logrus.

All three log the same shape: goroutine name, panic value, full stack,
"contained" wording — grep for `Panic contained` / `panic in` across logs.

## Audit checklist (PR review)

1. New `go` statement in imbot / remote / agentboot / server bot modules?
   → must use SafeGo/safego.Go, or carry a deferred Recover* as its first
   effective defer (register cleanup defers like `wg.Done`/`close(ch)`
   BEFORE Recover so they still run on panic).
2. New SDK callback registration (message handler, event dispatcher,
   command router)? → first line `defer b.RecoverCallback("...")`.
3. New platform receive loop? → `defer b.RecoverLoop("...")` so death
   feeds the reconnect path.
4. New third-party IM SDK, or bumping one? → grep its source for
   `go func` / `go x.` and check each spawn for an internal recover; a
   panic there is L4 and needs an upstream fix or a fork before shipping.
5. Never recover to *continue* the panicking operation — contain, log,
   drop or reconnect. Recover-and-retry hides real bugs.

## Considered and rejected

- **Per-bot OS processes** — true isolation even for L4, but a massive
  architecture change (IPC for the channel/prompter seams, credential
  handoff, process supervision) for a failure class that upgrading one
  SDK fixed. Revisit only if L4 incidents recur across SDKs.
- **A global panic hook** — Go has none; `recover` in `main` covers only
  main's goroutine. There is no unified in-process net; that is exactly
  why the per-goroutine rule (L3) has to be a convention with helpers,
  and why L4 exists as a policy rather than code.

## Tests

- `imbot/core/safego_test.go` — SafeGo containment; RecoverCallback;
  RecoverLoop flips connection state and emits disconnect.
- `remote/safego/safego_test.go` — Go/Recover containment.
- L1/L2 behavior covered by the existing manager tests
  (`remote/control/bot/manager_*_test.go`, imbot handler recover paths).
