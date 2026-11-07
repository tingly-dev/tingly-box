---
date: 2026-11-08
---

# Remote outbound: one agent-facing port for "tell this chat"

> Status: **O1–O5 done** (see git log); behavior unchanged. Was: proposal. Stacks on [remote-layering.pencil.md](./remote-layering.pencil.md) (L1–L5 done). Goal
> (YZ, 2026-11-08): unify the two outbound vocabularies if it gives cleaner decoupling and layering **with identical
> behavior**; design first.

## Problem

"Send something to an IM chat" exists twice, and the join between them lives in the wrong place.

| | Projects (agent side) | Remote (IM side) |
|---|---|---|
| Text | `ChannelSink.SendChannelText(ctx, platform, chatID, text)` | `channel.Channel.Send(Target, Notification)` |
| File | `ChannelFileSender.SendChannelFile(...)` | `imchannel.Channel.SendFile` — **not** on `channel.Channel` |
| Approval buttons | `ChannelPrompter.SendChannelPrompt` / `RestateChannelPrompt` | `imchannel.Channel.SendActions` / `Restate` — **not** on `channel.Channel` |
| Which bot | `ChannelBot(ctx)` smuggled in `context` | the caller picks a `Channel` |

`internal/server/module/projects/chatbridge.go` (host) is the translator, and it does Remote's job:
it asserts anonymous interfaces on `imchannel` (`SendFile`, `actionChannel`), imports `imchannel` and `chat`
(`ProjectAnswerPayload`), keeps the remembered-bot map, and routes a message to a bot (chosen bot, else the remembered
one, else each bot of the platform) while applying the blocklist (`disabled`, `targetBlocked`). Bot selection and
chat access policy are Remote's concerns, but they sit in the host, and `channel.Channel` is too thin to carry what
Projects needs.

```
now:
 Projects ──Channel*(+ctx bot)──▶ host bridge ──anonymous asserts──▶ imchannel ──▶ imbot
                                  (routing, blocklist, bot memory)
```

## Target

Mirror the inbound port. Inbound is `agent.Chat` (human → agent). Add the outbound port `agent.Outbox`
(agent → human), **defined by Remote and implemented by Remote** over its bots. Projects (or any agent) only sees the
port; the routing and policy move into Remote where they belong.

```
 agent (Projects)                     Remote (library)                        imbot
 ┌─────────────┐  agent.Outbox   ┌──────────────────────────┐   channel.Channel   ┌──────┐
 │ relay       │ ───────────────▶│ outbox: pick bot,        │ ───────────────────▶│ bots │
 │ (chat-      │  Text/File/     │  remember bot,           │  + optional         └──────┘
 │  addressed) │  Prompt/Restate │  blocklist, ErrNoChannel │  FileSender,
 └─────────────┘                 └──────────────────────────┘  ActionSender
        ▲                                                       (standard, in channel)
        │  agent.Chat (inbound, unchanged)  ◀── host: gate + runtime start + remember bot
```

Two levels, on purpose: `agent.Outbox` is **chat-addressed** (tell this chat; the agent knows nothing of bots), and
`channel.Channel` is **bot-addressed** (this bot's surface). The old Projects family and the host's ad hoc asserts
collapse into those two, each at its own level.

## The port (in `remote/agent`)

```go
// Outbox is how an agent speaks to a chat. Remote implements it over its bots.
type Outbox interface {
	Text(ctx context.Context, to ChatRef, text string) error
	File(ctx context.Context, to ChatRef, f OutFile) error
	// Prompt sends an approval with buttons; ref names the message (and the bot) for Restate.
	Prompt(ctx context.Context, to ChatRef, p Prompt) (ref string, err error)
	Restate(ctx context.Context, to ChatRef, ref, text string) error
}

type OutFile struct{ Path, Name, MediaType, Caption string }
type Choice struct{ Value, Label string }
type Prompt struct{ ThreadID, RequestID, Text string; Choices []Choice }

var (
	ErrNoButtons = errors.New("this chat cannot show buttons")
	ErrNoChannel = errors.New("no running bot for this chat")
)
```

- `to.BotUUID` is the binding's chosen bot (what `ChannelBot(ctx)` carried); empty means "the bot the chat last wrote
  through, else the platform's bots", exactly as `through` does today.
- `ThreadID`/`RequestID` keep their current names to preserve behavior; renaming them to agent-neutral words is a
  separate, optional step.
- The button callback payload stays `[pj, thread, request, value]` and `ProjectAnswerPayload` stays in `remote/chat`;
  only its construction moves from the host into Remote's outbox.

`remote/channel` gains the optional capabilities the bridge asserts anonymously today, as named interfaces:

```go
type FileSender interface { SendFile(ctx, chatID, path, name, mediaType, caption string) error }
type ActionSender interface {
	SendActions(ctx, chatID, text string, buttons []Button) (messageID string, err error)
	Restate(ctx, chatID, messageID, text string) error
}
type Button struct{ Label string; Payload []string }   // moved from imchannel; imchannel keeps an alias
```

`remote/outbox` (new package) implements `agent.Outbox`: the registry routing, remembered-bot map, `disabled` and
`targetBlocked` hooks, and the prompt-ref format, **moved unchanged** from `chatbridge.go` together with its unit tests.

## What changes where

| Area | Change |
|---|---|
| `remote/channel` | `FileSender`, `ActionSender`, `Button` (new); `imchannel` implements them (no behavior change) |
| `remote/agent` | `Outbox`, `OutFile`, `Choice`, `Prompt`, `ErrNoButtons`, `ErrNoChannel` (new) |
| `remote/outbox` | new; `through`, `checkTarget`, bot memory, prompt/restate moved from the host bridge |
| `projects` | `ChannelSink`/`ChannelFileSender`/`ChannelPrompter`/`ChannelPrompt`/`ChannelChoice`/`ErrNoButtons`/`ChannelBot` removed; the relay talks to an `agent.Outbox` the host offers (`Outbox() agent.Outbox`, optional, nil standalone); six send sites in `relay.go` and `service.go:60` change; three test files' fakes change |
| host `module/projects` | bridge keeps only the **inbound** side (`agent.Chat` + `Enabled` + runtime start); `hostchannel.go` shrinks to returning the outbox; the host wires one `outbox.Outbox` with its `disabled`/`targetBlocked` |

## Invariants (must not change; each has a test today or gets one first)

1. Bot choice: explicit bot (must be a running bot of that platform) → remembered bot → each bot of the platform in
   turn until one succeeds; the winner is remembered.
2. Blocklisted or target-blocked chat: text and prompts are dropped without error, restate is a no-op; an access
   lookup failure prevents delivery and is returned.
3. No running bot → `ErrNoChannel` (also "bot stopped" on restate).
4. A channel without buttons, or a platform that rejects them → `ErrNoButtons`, and the relay falls back to text.
5. The prompt ref round-trips (`bot UUID \x00 message id`); restate goes through the same bot even if another bot
   last received the chat.
6. A channel that cannot send files → "file delivery is not supported by <platform>".
7. Button callbacks and `Answer` are untouched.
8. Standalone Projects (no outbox) is still a no-op relay.

Tests that already cover most of this: the six bridge unit tests (move with the code), `channels_test.go`,
`chat_prompts_test.go`, `decision_im_test.go` (Projects fakes), and the Tingly IM harness. Missing: a unit test for
"file not supported" and for "restate through a stopped bot"; add before moving.

## As built

`agent.Outbox` (Text, File, Prompt, Restate) is implemented by `remote/outbox` over `channel.Channel` plus the optional
`channel.FileSender`/`ActionSender`. Projects' `Relay` speaks to an `agent.Outbox` the host offers through the optional
`OutboxHost.Outbox()`; its own `Channel*` family and the context-carried bot are gone (the bound bot rides in
`ChatRef.BotUUID`). The host bridge keeps only the inbound side and owns one outbox, which also remembers the bot each
chat wrote through. Open items below stayed at their defaults (`Outbox`, bot in `ChatRef`, Flow A untouched).

## Phases (one reviewable commit or small group each, behavior unchanged)

| # | Step |
|---|---|
| O1 | `channel.FileSender`/`ActionSender`/`Button`; `imchannel` implements and aliases |
| O2 | `agent.Outbox` types; `remote/outbox` with the moved routing and its tests; two missing tests first |
| O3 | Host bridge uses `outbox.Outbox` for its old `Channel*` methods (still implementing Projects' old interfaces) — proves equivalence before Projects changes |
| O4 | Projects switches to `agent.Outbox`; delete its `Channel*` family and the ctx bot; adjust three test fakes |
| O5 | Host: drop the old methods and the `imchannel`/`chat` imports from the bridge; docs and boundary notes |

## Risks / watch

- O4 touches the relay's send path (the only place Projects speaks to a chat). Keep `relaySendTimeout` and the
  fallback-to-text logic exactly; the diff there should be mechanical.
- Moving the remembered-bot map into Remote means inbound (`touch`) must call it; keep that call in the host gate.
- `Prompt.ThreadID/RequestID` leak Projects vocabulary into the port; accepted for now to keep behavior identical.

## Open

1. Name: `Outbox` vs `Speaker`/`Notifier`. (Default: `Outbox`.)
2. Should Flow A (hook-driven notify/ask via `channel.Channel` + registry) later use `Outbox` too? Out of scope here.
3. Is `to.BotUUID` the right place for the chosen bot, or a separate argument? (Default: in `ChatRef`, as inbound.)
