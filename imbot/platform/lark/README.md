# platform/lark

Lark is the international edition of Feishu.  This package is a **thin wrapper**
around [`platform/feishu`](../feishu/README.md) that targets
`open.larksuite.com` instead of `open.feishu.cn`.

All behaviour, configuration keys, capabilities, and platform-specific APIs are
identical to Feishu — only the base URL differs.

## Authentication

**Type:** `oauth`

Create a **self-built app** in [Lark Developer Console](https://open.larksuite.com/).

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
    Platform: core.PlatformLark,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     os.Getenv("LARK_APP_ID"),
        ClientSecret: os.Getenv("LARK_APP_SECRET"),
    },
}
```

## Implementation notes

- `lark.NewBot(config)` calls `feishu.NewBot(config, feishu.DomainLark)`.
- The Lark SDK client is initialised with `lark.LarkBaseUrl`
  (`https://open.larksuite.com`) as the HTTP base URL.
- The WebSocket event-push endpoint also targets Lark's infrastructure.
- `PlatformInfo()` returns `"lark"` as the platform identifier.

## Webhook helpers

`GetWebhookURL(path)` returns `/webhook/lark/<path>` — use this if you
register a webhook endpoint in your HTTP server for Lark event verification.

## Files

| File | Purpose |
|---|---|
| `lark.go` | Thin `Bot` wrapper; delegates all calls to `feishu.Bot` |
| `menu.go` | Lark-specific menu helpers (re-uses Feishu card format) |
