# imbot/platform — Platform Implementations

Each subdirectory implements the `core.Bot` interface for one messaging platform.
All platforms share the same connection lifecycle, event model, and message types
defined in `imbot/core`; differences are handled inside each package via an
**Adapter** and optional **Interaction/Menu** adapters.

## Directory layout

```
platform/
├── registry.go      # Global factory — maps Platform → BotCreator func
├── telegram/
├── discord/
├── slack/
├── feishu/
├── lark/            # Feishu alias (larksuite.com domain)
├── dingtalk/
├── weixin/          # WeChat personal / official account
├── wecom/           # WeCom (Enterprise WeChat AI Bot)
├── whatsapp/
└── tingly/          # Internal test platform + E2E harness
```

## How the registry works

`registry.go` holds a global `Registry` that maps `core.Platform` values to
constructor functions (`BotCreator`).  The factory is called through:

```go
// Create a bot from a config (uses global registry)
bot, err := platform.Create(config)

// Check support before creating
if platform.IsSupported(core.PlatformSlack) { ... }
```

Custom platforms can be registered at startup:

```go
platform.Register(core.Platform("myplatform"), func(cfg *core.Config) (core.Bot, error) {
    return mypkg.NewBot(cfg), nil
})
```

---

## Platform reference

Each platform has its own README with full configuration and design details:

| Platform | README | Auth | Connection |
|---|---|---|---|
| Telegram | [telegram/README.md](telegram/README.md) | token | Long-polling |
| Discord | [discord/README.md](discord/README.md) | token | WebSocket (Gateway) |
| Slack | [slack/README.md](slack/README.md) | token | RTM |
| Feishu | [feishu/README.md](feishu/README.md) | oauth | WebSocket (Event Push) |
| Lark | [lark/README.md](lark/README.md) | oauth | WebSocket (Event Push) |
| DingTalk | [dingtalk/README.md](dingtalk/README.md) | oauth | Stream SDK |
| Weixin | [weixin/README.md](weixin/README.md) | token | WebSocket |
| WeCom | [wecom/README.md](wecom/README.md) | oauth | WebSocket |
| WhatsApp | [whatsapp/README.md](whatsapp/README.md) | token | REST (Meta Cloud API) |
| Tingly | [tingly/README.md](tingly/README.md) | none | InProcess / pluggable |

---

### Telegram

| | |
|---|---|
| Package | `platform/telegram` |
| Auth | `token` — bot token from [@BotFather](https://t.me/botfather) |
| Connection | Long-polling (default) or WebSocket |
| SDK | `github.com/go-telegram/bot` |
| Text limit | 4 096 characters |

**Config:**

```go
&core.Config{
    Platform: core.PlatformTelegram,
    Auth: core.AuthConfig{
        Type:  "token",
        Token: "123456:ABC-DEF...",  // or "$TELEGRAM_BOT_TOKEN"
    },
    Options: map[string]interface{}{
        "proxy": "socks5://user:pass@host:1080",  // optional SOCKS5/HTTP proxy
        "debug": false,
    },
}
```

**Capabilities:** inline keyboards, reply keyboards, chat menu button,
native commands, reactions, message edit/delete, threads, polls, media
(image, video, audio, document, sticker, GIF).

**Platform-specific methods** (cast via `imbot.AsTelegramBot`):

```go
if tg, ok := imbot.AsTelegramBot(bot); ok {
    tg.SetCommandList(registry.BuildTelegramMenuCommands())
    tg.SetMenuButton(telegram.MenuButtonConfig{Type: telegram.MenuButtonTypeCommands})
    chatID, _ := tg.ResolveChatID("@username")
}
```

---

### Discord

| | |
|---|---|
| Package | `platform/discord` |
| Auth | `token` — bot token from the Discord Developer Portal |
| Connection | WebSocket (Gateway API via `discordgo`) |
| SDK | `github.com/bwmarrin/discordgo` |
| Text limit | 2 000 characters |

**Config:**

```go
&core.Config{
    Platform: core.PlatformDiscord,
    Auth: core.AuthConfig{
        Type:  "token",
        Token: "Bot MTxxxxxxx...",   // "Bot " prefix is added automatically if omitted
    },
    Options: map[string]interface{}{
        // Gateway intents (default: Guilds + DirectMessages + GuildMessages + MessageContent)
        "intents": []interface{}{"guilds", "directMessages", "guildMessages", "messageContent"},
    },
}
```

**Capabilities:** components (buttons, select menus), threads, reactions,
message edit/delete, mentions, media (image, video, audio, document).

---

### Slack

| | |
|---|---|
| Package | `platform/slack` |
| Auth | `token` — bot token (`xoxb-...`) from Slack App settings |
| Connection | RTM (Real-Time Messaging) |
| SDK | `github.com/slack-go/slack` |
| Text limit | 40 000 characters |

**Config:**

```go
&core.Config{
    Platform: core.PlatformSlack,
    Auth: core.AuthConfig{
        Type:  "token",
        Token: "xoxb-...",
    },
    Options: map[string]interface{}{
        "appToken": "xapp-...", // optional App-Level Token for Socket Mode
    },
}
```

**Capabilities:** Block Kit buttons/menus, reactions, threads, message
edit, media.

---

### Feishu

| | |
|---|---|
| Package | `platform/feishu` |
| Auth | `oauth` — App ID + App Secret from Feishu Open Platform |
| Connection | WebSocket (Feishu event-push via `larkws`) |
| SDK | `github.com/larksuite/oapi-sdk-go/v3` |
| Text limit | 30 720 characters |
| Domain | `feishu.cn` |

**Config:**

```go
&core.Config{
    Platform: core.PlatformFeishu,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     "cli_xxxxxxxxxxxxxxxx",   // App ID
        ClientSecret: "xxxxxxxxxxxxxxxxxxxxxxxx", // App Secret
    },
}
```

**Capabilities:** interactive cards (full card builder), quick actions
(slash-like `/` menu), reactions, threads, message edit/delete, media.

**Platform-specific methods** (cast via `imbot.AsFeishuBot`):

```go
if fs, ok := imbot.AsFeishuBot(bot); ok {
    fs.SetQuickActions(actions)
    actions, _ := fs.GetQuickActions()
}
```

**ID routing:** The adapter automatically selects `receive_id_type` based
on the target ID prefix (`ou_` → `user_id`, `cli_` → `chat_id`,
`oc_` → `open_id`).

---

### Lark

| | |
|---|---|
| Package | `platform/lark` |
| Auth | `oauth` — same as Feishu |
| Domain | `larksuite.com` (international) |

Lark is the international edition of Feishu.  The `lark` package is a
thin wrapper around the `feishu` package that sets `DomainLark` so the
SDK targets `larksuite.com` instead of `feishu.cn`.  Configuration and
capabilities are identical to Feishu.

```go
&core.Config{
    Platform: core.PlatformLark,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     "cli_...",
        ClientSecret: "...",
    },
}
```

---

### DingTalk

| | |
|---|---|
| Package | `platform/dingtalk` |
| Auth | `oauth` — AppKey + AppSecret from DingTalk Open Platform |
| Connection | DingTalk Stream SDK (persistent TCP stream) |
| SDK | `github.com/open-dingtalk/dingtalk-stream-sdk-go` |

**Config:**

```go
&core.Config{
    Platform: core.PlatformDingTalk,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     "dingxxxxxxxxxxxxxxxxxx",  // AppKey
        ClientSecret: "xxxxxxxxxxxxxxxxxxxxxxxx", // AppSecret
    },
}
```

**Design notes:**

- Receiving messages uses the **Stream SDK** (no public webhook URL needed).
- Sending and reactions use a **REST API** with a cached access token
  (auto-refreshed before expiry).
- Incoming webhook URLs for sending replies are stored per
  `conversationID` and populated on first message receipt.

**Capabilities:** chatbot message handling, reactions via REST API, media.

---

### Weixin

| | |
|---|---|
| Package | `platform/weixin` |
| Auth | Token + AccountID |
| Connection | WebSocket via `github.com/tingly-dev/weixin` SDK |

Weixin supports WeChat personal and official accounts through the
tingly-dev weixin SDK.

**Config:**

```go
&core.Config{
    Platform: core.PlatformWeixin,
    Auth: core.AuthConfig{
        Type:      "token",
        Token:     "bot_id:token_key",  // combined token
        AccountID: "bot_id",            // WeChat bot/account ID
        AuthDir:   "user_id",           // reused field for user ID
    },
    Options: map[string]interface{}{
        "baseUrl": "https://weixin-gateway.example.com",
    },
}
```

**Capabilities:** text, media (image, audio, video, document), basic
message routing per account.

---

### WeCom

| | |
|---|---|
| Package | `platform/wecom` |
| Auth | `oauth` — Bot ID + Bot Secret |
| Connection | WebSocket via `github.com/tingly-dev/weixin/wecom` SDK |

WeCom (企业微信) AI Bot integration for enterprise group chats.

**Config:**

```go
&core.Config{
    Platform: core.PlatformWecom,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", // Bot ID
        ClientSecret: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",      // Bot Secret
    },
}
```

**Capabilities:** text and media messages inside WeCom group/direct chats.

---

### WhatsApp

| | |
|---|---|
| Package | `platform/whatsapp` |
| Auth | `token` — API key from Meta Business |
| Connection | HTTP (Meta Business API webhooks) — stateless REST calls |

**Config:**

```go
&core.Config{
    Platform: core.PlatformWhatsApp,
    Auth: core.AuthConfig{
        Type:    "token",
        Token:   "EAAxxxxxxxxxx...",  // Meta permanent or temporary token
        AuthDir: "/path/to/auth/dir", // optional: local session storage
    },
    Options: map[string]interface{}{
        "phoneId": "1234567890",  // WhatsApp Business phone-number ID
        "apiUrl":  "https://graph.facebook.com/v18.0",
    },
}
```

**Design notes:** Unlike persistent-connection platforms, WhatsApp uses
stateless REST calls for sending.  Incoming messages arrive via Meta
webhook events handled externally and fed to the bot via the adapter.

**Capabilities:** text, images, documents, audio, read receipts.

---

### Tingly (internal / test)

| | |
|---|---|
| Package | `platform/tingly` |
| Auth | `none` |
| Connection | Pluggable `Transport` interface |

Tingly is the internal test platform used by the E2E harness under
`platform/tingly/testenv/`.  It provides **full feature parity** with
Telegram (inline keyboards, menus, commands, reactions, media) over an
in-process or network transport.

```go
// Register a custom in-process transport
tingly.RegisterTransport(botUUID, myTransport)

// Create bot (no real auth needed)
&core.Config{
    Platform: core.PlatformTingly,
    Auth:     core.AuthConfig{Type: "none"},
}
```

The `testenv` sub-package spins up a complete Tingly bot in-process,
making it possible to write deterministic integration tests without a
live messaging account.

---

## Adapter pattern

Every platform that receives events implements an **Adapter** that
converts raw SDK types to the unified `core.Message`:

```
Raw SDK event
    └─► Adapter.AdaptMessage() / AdaptCallback()
            └─► core.Message  (emitted to Manager handlers)
```

Platforms that support interactive elements additionally implement
`interaction.Adapter` and optionally `menu.Adapter`, wired up in
`interaction.go` / `menu.go` inside each package.

## Adding a new platform

1. Create `platform/<name>/<name>.go` embedding `*core.BaseBot`.
2. Implement `core.Bot` (connect, disconnect, send, react, etc.).
3. Add an `Adapter` that converts raw events to `core.Message` and calls
   `b.EmitMessage(msg)`.
4. Register the constructor in `registry.go` under
   `RegisterBuiltinPlatforms()`.
5. Optionally implement `interaction.Adapter` and `menu.Adapter` for
   richer interactivity.
