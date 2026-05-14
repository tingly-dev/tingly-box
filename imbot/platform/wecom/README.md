# platform/wecom

WeCom (企业微信, Enterprise WeChat) AI Bot implementation using the
[tingly-dev/weixin/wecom](https://github.com/tingly-dev/weixin) SDK.

## Authentication

**Type:** `oauth`

Obtain the **Bot ID** and **Bot Secret** from the WeCom admin console
(企业微信管理后台) under the AI Bot configuration.

```go
Auth: core.AuthConfig{
    Type:         "oauth",
    ClientID:     "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", // Bot ID
    ClientSecret: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",      // Bot Secret
}
```

## Configuration

```go
&core.Config{
    Platform: core.PlatformWecom,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     os.Getenv("WECOM_BOT_ID"),
        ClientSecret: os.Getenv("WECOM_BOT_SECRET"),
    },
}
```

Both `ClientID` and `ClientSecret` are required; the bot will return an error
during construction if either is empty.

## Connection model

The bot wraps `wecom.WecomBot` from the tingly-dev weixin SDK.  The underlying
transport is a **WebSocket** connection managed by the SDK.  `Connect` starts
the WebSocket session and `Disconnect` stops it.

## Capabilities

| Feature | Supported |
|---|---|
| Send text | ✅ |
| Send media | ✅ |
| Group and direct chat | ✅ |
| Reactions | ❌ |
| Edit / Delete message | ❌ |

## Files

| File | Purpose |
|---|---|
| `wecom.go` | `Bot` struct, lifecycle, send methods |
| `adapter.go` | Converts WeCom message payloads → `core.Message` |
