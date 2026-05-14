# imbot/platform

Each subdirectory implements `core.Bot` for one messaging platform. All platforms share the same lifecycle, event model, and message types from `imbot/core`; differences are encapsulated in per-package **Adapter**, **Interaction**, and **Menu** adapters.

## Platforms

| Platform | README | Auth | Connection |
|---|---|---|---|
| Telegram | [telegram/](telegram/README.md) | token | Long-polling |
| Discord | [discord/](discord/README.md) | token | WebSocket (Gateway) |
| Slack | [slack/](slack/README.md) | token | RTM |
| Feishu | [feishu/](feishu/README.md) | oauth | WebSocket (Event Push) |
| Lark | [lark/](lark/README.md) | oauth | WebSocket (Event Push) |
| DingTalk | [dingtalk/](dingtalk/README.md) | oauth | Stream SDK |
| Weixin | [weixin/](weixin/README.md) | token | WebSocket |
| WeCom | [wecom/](wecom/README.md) | oauth | WebSocket |
| WhatsApp | [whatsapp/](whatsapp/README.md) | token | REST (Meta Cloud API) |
| Tingly | [tingly/](tingly/README.md) | none | InProcess / pluggable |

## Registry

`registry.go` maps `core.Platform` values to constructor functions. Use the global helpers:

```go
bot, err := platform.Create(config)
ok := platform.IsSupported(core.PlatformSlack)
```

Register a custom platform at startup:

```go
platform.Register(core.Platform("myplatform"), func(cfg *core.Config) (core.Bot, error) {
    return mypkg.NewBot(cfg), nil
})
```

## Adding a new platform

1. Create `platform/<name>/<name>.go` embedding `*core.BaseBot`.
2. Implement `core.Bot` (connect, disconnect, send, react, edit, delete).
3. Add an `Adapter` that converts raw SDK events to `core.Message` and calls `b.EmitMessage(msg)`.
4. Register the constructor in `registry.go` → `RegisterBuiltinPlatforms()`.
5. Optionally implement `interaction.Adapter` and `menu.Adapter` for richer interactivity.
6. Add a `README.md` following the pattern of existing platforms.
