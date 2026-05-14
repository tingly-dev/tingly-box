# platform/weixin

WeChat (微信) personal and official account bot implementation using the
[tingly-dev/weixin](https://github.com/tingly-dev/weixin) SDK.

## Authentication

**Type:** `token` (combined format)

Credentials are sourced from multiple `AuthConfig` fields that are reused for
WeChat-specific semantics:

| `AuthConfig` field | WeChat meaning |
|---|---|
| `Token` | `"bot_id:token_key"` combined token string |
| `AccountID` | WeChat bot / account ID |
| `AuthDir` | WeChat user ID |

```go
Auth: core.AuthConfig{
    Type:      "token",
    Token:     "mybot:secretkey123",
    AccountID: "mybot",
    AuthDir:   "target_user_id",
},
```

## Configuration

```go
&core.Config{
    Platform: core.PlatformWeixin,
    Auth: core.AuthConfig{
        Type:      "token",
        Token:     os.Getenv("WEIXIN_TOKEN"),
        AccountID: os.Getenv("WEIXIN_BOT_ID"),
        AuthDir:   os.Getenv("WEIXIN_USER_ID"),
    },
    Options: map[string]interface{}{
        // Required: base URL of the WeChat gateway service.
        "baseUrl": "https://weixin-gateway.example.com",
        // Alternative key name also accepted:
        // "base_url": "https://weixin-gateway.example.com",
    },
}
```

## Connection model

The bot wraps `wechat.WechatBot` from the tingly-dev weixin SDK and connects
via a **WebSocket** to the configured `baseUrl`. Each bot is associated with a
single WeChat account identified by `accountID`.

## Capabilities

| Feature | Supported |
|---|---|
| Send text | ✅ |
| Send media (image, audio, video, document) | ✅ |
| Basic message routing per account | ✅ |
| Reactions | ❌ |
| Edit / Delete message | ❌ |

## Files

| File | Purpose |
|---|---|
| `weixin.go` | `Bot` struct, lifecycle, send methods |
| `adapter.go` | Converts WeChat message types → `core.Message` |
| `interaction.go` | Interaction adapter (text-mode fallback) |
| `registration.go` | Registers the platform in the global registry |
