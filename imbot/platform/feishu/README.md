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

## ID kinds and routing

Feishu writes the kind of an ID into its prefix. `SendMessage` selects
`receive_id_type` from it (`feishu.go`, `getReceiveIdType`):

| Prefix | `receive_id_type` | Meaning | Receiver? |
|---|---|---|---|
| `oc_` | `chat_id` | Conversation (group or one-to-one) | ✅ |
| `ou_` | `open_id` | User, scoped to this app | ✅ |
| `on_` | `union_id` | User, scoped to the app developer | ✅ (the send API lists `union_id` among its `receive_id_type` values) |
| *(contains `@`)* | `email` | Email address | ✅ |
| *(no prefix)* | `user_id` | Tenant-assigned user id | ✅ |
| `od-` | — | Department (`open_department_id`) | ❌ not a message receiver |
| `cli_` | — | App id | ❌ not a message receiver |
| tenant key | — | Tenant identity (16 hex chars, no prefix) | ❌ not a message receiver; indistinguishable from a `user_id` by shape |

What the adapter stores as `core.Message.Recipient.ID` (`feishu.go`,
`convertLarkMessageToCore`):

- **Direct message** — the sender's `user_id` when present, otherwise their
  `open_id`; the p2p conversation's `oc_` id is kept only in
  `Metadata["original_chat_id"]`.
- **Group / channel** — the event's `chat_id` (`oc_…`).
- **Card press** — the `chat` value the card was sent with when it belongs to
  the same DM (`card_callback.go`), so a DM stays keyed by its user id/open id.

The Projects bind selector offers only the receiver kinds above; `od-` and
`cli_` rows stay visible in IM settings with the reason but cannot be bound.
See [`.design/projects-im.md`](../../../.design/projects-im.md#绑定候选的可发送性id-类型)
for that policy and `.research/im-id-types.md` for the full evidence.

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

The preferred path for the quick-action (`/`) menu is the package helper, which
builds the actions from a `core.CommandRegistry`:

```go
import "github.com/tingly-dev/tingly-box/imbot/platform/feishu"

if err := feishu.SetupQuickActions(bot, registry); err != nil {
    // handle error
}
```

For lower-level control, cast the `core.Bot` directly to `*feishu.Bot`:

```go
if fs, ok := bot.(*feishu.Bot); ok {
    // Register quick actions shown when the user types "/".
    fs.SetQuickActions(actions)

    // Retrieve the current quick-action configuration.
    actions, err := fs.GetQuickActions()
}
```

(There is no `imbot.AsFeishuBot` helper — use the direct `bot.(*feishu.Bot)`
assertion shown above.)

## Files

| File | Purpose |
|---|---|
| `feishu.go` | `Bot` struct, lifecycle, send/react/edit/delete; receive-id routing; `Domain` (feishu/lark) |
| `adapter.go` | Converts Feishu/Lark event payloads → `core.Message` |
| `types.go` | Feishu-specific event and payload structs |
| `action_render.go` | Renders a `core.ActionSet` as a Feishu action card |
| `card_render.go` | `CardRenderer` — converts a platform-neutral card into Feishu card JSON |
| `card_callback.go` | Handles card button-press callback events |
| `menu_setup.go` | Installs quick actions (the `/` command menu) from a `core.CommandRegistry` |
| `registration.go` | One-click app registration (QR-based provisioning) outcome types |
