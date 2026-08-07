# Bot Panic Isolation — recover at the trust boundary

Why this exists: a single `panic` escaping any goroutine kills the whole
tingly-box process, and the remote bot stack is the most exposed surface —
seven third-party IM SDKs, network callbacks, and adapter code parsing
untrusted platform payloads.

Trigger case (2026-08): dingtalk-stream-sdk-go v0.9.1 raced its ping-timeout
goroutine against its read-error goroutine, both sending on a `closeChan`
that the process loop closes on exit → `panic: send on closed channel`
inside a goroutine **the SDK itself spawned** → whole process down.

## The rule

`recover()` only works on the goroutine that panicked, so containment is
placed where the risk actually lives — the trust boundary where third-party
SDK code runs or third-party payloads are parsed — and nowhere else:

1. **SDK-invoked callbacks** (the SDK calls us on its goroutine):
   first line is `defer b.RecoverCallback("...")` — contain, drop the one
   message, keep the connection.
2. **Our receive loops** (they run SDK code and adapters):
   `defer b.RecoverLoop("...")` — contain, flip to disconnected, emit the
   disconnect event so the manager's auto-reconnect rebuilds the connection.
   Contained must never mean zombie: a swallowed panic that leaves a
   deaf-but-"running" bot is worse than a crash, because nobody notices.
3. **SDK-internal goroutines** are NOT containable in-process. Policy: pin a
   fixed version, audit its goroutine spawn points when adopting/bumping an
   IM SDK, upgrade or patch on any panic report. The trigger case was fixed
   by upgrading dingtalk-stream-sdk-go to v0.9.2-beta.1 — the upstream fix
   for exactly that race (issues #27/#28/#32), which also adds a read-loop
   recover and reconnect backoff.

Goroutines that run only our own code (session cleanup loops, prompt
drivers, sync workers, shutdown plumbing) are **deliberately not wrapped**:
they sit behind the existing supervision layers below, and blanket
recover-wrapping ritual code adds noise without protecting against a real
failure mode. This scope was an explicit decision, not an omission.

## Existing supervision this builds on

- **Per-bot lifecycle** (`remote/control/bot/manager.go`,
  `runBotSupervised`): each bot runs in one supervised goroutine with
  recover + stack log + running-map cleanup. A dying bot touches neither
  the process nor its sibling bots.
- **Handler dispatch** (`imbot/manager.go`, `imbot/core/base.go`): every
  OnMessage/OnError/... handler runs on its own goroutine behind
  `recoverHandler`, at both the manager and BaseBot level. A panicking
  consumer (remote_agent, notify, prompt router) drops one event only.

## Boundary coverage (imbot/core/safego.go)

| Platform | RecoverLoop | RecoverCallback |
|---|---|---|
| telegram | polling goroutine (`api.Start`) | message + callback-query handlers |
| slack | RTM event loop | — (messages flow through the loop) |
| weixin | long-poll loop | — (same) |
| feishu | websocket goroutine (`wsClient.Start`) | P2 message + card action handlers |
| wecom | — (lifecycle goroutine is trivial) | OnMessage / OnEvent / OnError |
| dingtalk | — (SDK owns the loop; see rule 3) | chatbot callback router |
| whatsapp | — (webhook mode, no loop body) | — |

## Review checklist

1. New SDK callback registration (message handler, event dispatcher,
   command router)? → first line `defer b.RecoverCallback("...")`.
2. New receive loop that runs SDK code or parses platform payloads?
   → `defer b.RecoverLoop("...")`, registered after cleanup defers
   (`wg.Done`, `close(ch)`) so those still run on panic.
3. New third-party IM SDK, or bumping one? → grep its source for
   `go func` / `go x.` and check each spawn for an internal recover; a
   panic there is uncontainable in-process and needs an upstream fix or a
   fork before shipping.
4. Never recover to *continue* the panicking operation — contain, log,
   drop or reconnect. Recover-and-retry hides real bugs.
5. Goroutines running only our own code: don't wrap them; fix bugs.

## Considered and rejected

- **Blanket recover on every goroutine in the bot path** (implemented,
  then removed): uniform rule, but most wrapped sites were pure channel
  plumbing that cannot realistically panic — ritual, not protection.
- **Per-bot OS processes** — true isolation even for SDK-internal panics,
  but a massive architecture change (IPC for the channel/prompter seams,
  credential handoff, process supervision) for a failure class that
  upgrading one SDK fixed. Revisit only if such incidents recur across
  SDKs.
- **A global panic hook** — Go has none; `recover` in `main` covers only
  main's goroutine.

## Tests

- `imbot/core/safego_test.go` — RecoverCallback containment; RecoverLoop
  flips connection state and emits disconnect.
- Supervision layers covered by the existing manager tests
  (`remote/control/bot/manager_*_test.go`).
