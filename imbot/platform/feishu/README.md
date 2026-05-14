# platform/feishu

Feishu (飞书) bot implementation using the official
[Lark OpenAPI SDK](https://github.com/larksuite/oapi-sdk-go).

## Authentication

**Type:** `oauth`

Create a **self-built app** in the [Feishu Open Platform](https://open.feishu.cn/),
enable the required permissions, and copy the **App ID** and **App Secret**.

```go
Auth: core.AuthConfig{
    Type:         "oauth",
    ClientID:     "cli_xxxxxxxxxxxxxxxx",    // App ID
    ClientSecret: "xxxxxxxxxxxxxxxxxxxxxxxx", // App Secret
}
```

## Configuration

```go
&core.Config{
    Platform: core.PlatformFeishu,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     os.Getenv("FEISHU_APP_ID"),
        ClientSecret: os.Getenv("FEISHU_APP_SECRET"),
    },
}
```

No additional options are required for standard usage.

## Connection model

`Connect` performs two steps:

1. **Authentication check** — calls `GetTenantAccessTokenBySelfBuiltApp` to
   verify the credentials immediately.
2. **WebSocket long connection** — starts `larkws.Client` which maintains a
   persistent WebSocket to Feishu's event-push endpoint
   (`open.feishu.cn`). Events are dispatched by `dispatcher.EventDispatcher`.

No public inbound webhook URL is required; the bot initiates the connection.

## ID routing

`SendMessage` automatically selects `receive_id_type` based on the target ID
prefix:

| Prefix | Type | Usage |
|---|---|---|
| `cli_` | `chat_id` | Group chat |
| `ou_` | `user_id` | User (cross-app, global) |
| `oc_` | `open_id` | User (app-specific) |
| *(other)* | `open_id` | Default fallback |

## Capabilities

| Feature | Supported |
|---|---|
| Send text | ✅ |
| Send media (image, audio, video, document) | ✅ |
| Interactive cards (full Feishu card builder) | ✅ |
| Quick actions (`/` command menu) | ✅ |
| Native commands | ✅ |
| Reactions | ✅ |
| Edit message | ✅ |
| Delete message | ✅ |
| Threads | ✅ |
| Text limit | 30 720 chars |

## Platform-specific API

```go
import "github.com/tingly-dev/tingly-box/imbot/platform/feishu"

if fs, ok := bot.(*feishu.Bot); ok {
    // Register quick actions shown when the user types "/".
    fs.SetQuickActions(actions)

    // Retrieve the current quick-action configuration.
    actions, err := fs.GetQuickActions()
}
```

`imbot.AsFeishuBot(bot)` is a convenience wrapper for the type assertion.

## Files

| File | Purpose |
|---|---|
| `bot_sdk.go` | `Bot` struct, lifecycle, send/react/edit/delete; domain constants |
| `adapter.go` | Converts Feishu event payloads → `core.Message` |
| `types.go` | Feishu-specific event and payload structs |
| `interaction.go` | Builds interactive cards from `interaction.Interaction` |
| `menu.go` | Converts `menu.Menu` → Feishu interactive card buttons |
| `menu_setup.go` | Registers quick actions via Feishu API |
| `registration.go` | Registers the platform in the global registry |
