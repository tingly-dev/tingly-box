# platform/tingly

Tingly is the **internal test platform** for imbot. It provides full feature
parity with a real messaging platform (inline keyboards, reactions, edit,
delete, media) over a pluggable **Transport** seam, making it possible to write
deterministic integration and E2E tests without a live messaging account.

## Design

```
tingly.Bot
  └── Transport  ← seam
        ├── InProcessTransport   (used by tests and testenv)
        └── (future WebSocketTransport for a real tingly server)
```

`Bot` never touches a network directly. Every outbound operation (`SendMessage`,
`React`, `EditMessage`, `DeleteMessage`) is forwarded to the `Transport`.
Inbound messages are injected into the bot via `Transport.Subscribe`.

## Authentication

**Type:** `none`

No credentials are required.

```go
Auth: core.AuthConfig{Type: "none"}
```

## Configuration

```go
&core.Config{
    UUID:     "my-test-bot",   // used by the transport registry
    Platform: core.PlatformTingly,
    Auth:     core.AuthConfig{Type: "none"},
}
```

## Using in tests with InProcessTransport

### Basic setup

```go
import (
    "github.com/tingly-dev/tingly-box/imbot/platform/tingly"
    "github.com/tingly-dev/tingly-box/imbot/core"
)

transport := tingly.NewInProcessTransport()

bot, _ := tingly.NewBot(&core.Config{
    UUID:     "test-bot",
    Platform: core.PlatformTingly,
    Auth:     core.AuthConfig{Type: "none"},
}, transport)

bot.Connect(context.Background())
```

### Injecting inbound messages

```go
transport.Inject(core.Message{
    ID:       "msg-1",
    Platform: core.PlatformTingly,
    Sender:   core.Sender{ID: "user-1", DisplayName: "Alice"},
    Recipient: core.Recipient{ID: "chat-1"},
    Content:  core.NewTextContent("hello"),
})
```

### Asserting outbound events

```go
// Snapshot of all recorded bot actions.
events := transport.Events()

// Filter to a single chat.
chatEvents := transport.EventsForChat("chat-1")

// Wait for the next event on a chat (blocking).
ch := transport.Channel("chat-1")
evt := <-ch

switch evt.Kind {
case tingly.EventSend:
    fmt.Println("bot sent:", evt.Text)
    fmt.Println("keyboard:", evt.Keyboard) // decoded inline keyboard
case tingly.EventReact:
    fmt.Println("bot reacted:", evt.Emoji)
case tingly.EventEdit:
    fmt.Println("bot edited message:", evt.MessageID)
}
```

### Event kinds

| `EventKind` | Triggered by |
|---|---|
| `EventSend` | `SendMessage` / `SendText` with text |
| `EventMedia` | `SendMessage` / `SendMedia` with media only |
| `EventEdit` | `EditMessage` |
| `EventDelete` | `DeleteMessage` |
| `EventReact` | `React` |

### Keyboard decoding

The `Event.Keyboard` field is automatically populated when the outbound
message carries an inline keyboard. Each `Button` exposes `Label`,
`CallbackData`, and `URL`.

## Using the testenv harness

`platform/tingly/testenv` provides higher-level helpers that wrap the
transport and expose a chat-centric API:

```go
import "github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"

env := testenv.New()
chat := env.NewChat("user-1")

chat.Send("hello")                          // inject a user message
reply := chat.WaitForReply(time.Second)     // wait for the bot to respond
```

## Registering a transport by UUID

When using `imbot.Manager`, the bot is created via the global registry (not
directly). Pre-register the transport before adding the bot to the manager:

```go
tingly.Register("my-test-bot-uuid", transport)
defer tingly.Unregister("my-test-bot-uuid")

manager.AddBot(&core.Config{
    UUID:     "my-test-bot-uuid",
    Platform: core.PlatformTingly,
    Auth:     core.AuthConfig{Type: "none"},
})
```

`NewBotFromConfig` (the registry factory) calls `lookup(config.UUID)` to find
the pre-registered transport. If none is found, a fresh `InProcessTransport`
is created automatically.

## Files

| File | Purpose |
|---|---|
| `tingly.go` | `Bot` struct and lifecycle; delegates all ops to `Transport` |
| `transport.go` | `Transport` interface; `InProcessTransport`; `Event` types; transport registry |
| `adapter.go` | Decodes reply markup into `Keyboard` / `Button` structs |
| `interaction.go` | Builds tingly interaction responses |
| `testenv/` | High-level E2E harness wrapping `InProcessTransport` |
