# platform/dingtalk

DingTalk (钉钉) bot implementation using the official
[DingTalk Stream SDK](https://github.com/open-dingtalk/dingtalk-stream-sdk-go).

## Authentication

**Type:** `oauth`

Create a **chatbot application** in the
[DingTalk Open Platform](https://open.dingtalk.com/). Copy the **AppKey** and
**AppSecret** from the app credentials page.

```go
Auth: core.AuthConfig{
    Type:         "oauth",
    ClientID:     "dingxxxxxxxxxxxxxxxxxx", // AppKey
    ClientSecret: "xxxxxxxxxxxxxxxxxxxxxxxx", // AppSecret
}
```

## Configuration

```go
&core.Config{
    Platform: core.PlatformDingTalk,
    Auth: core.AuthConfig{
        Type:         "oauth",
        ClientID:     os.Getenv("DINGTALK_APP_KEY"),
        ClientSecret: os.Getenv("DINGTALK_APP_SECRET"),
    },
}
```

No additional options are required.

## Connection model

The bot uses a **dual-channel** design:

| Channel | Purpose |
|---|---|
| **Stream SDK** (`StreamClient`) | Receives incoming chatbot messages over a persistent TCP stream |
| **REST API** | Sends messages and reactions; requires a cached access token |

The stream connection is initiated by `Connect` via `cli.Start(ctx)`. No public
inbound URL is needed — the bot calls DingTalk's stream endpoint.

### Access token caching

REST API calls (e.g. reactions) require an access token obtained via
`/gettoken`. The bot caches the token in memory and refreshes it automatically
before expiry (`tokenExpiry`). The cache is protected by `tokenMu`.

### Webhook map

When a chatbot message arrives, DingTalk includes a per-conversation
`sessionWebhook` URL. The bot stores these in `webhookMap`
(`conversationID → webhookURL`) so that replies can be routed back to the
correct conversation without a separate target-resolution call.

## Capabilities

| Feature | Supported |
|---|---|
| Send text | ✅ |
| Send media | ✅ |
| Reactions (via REST API) | ✅ |
| Card buttons | ✅ |
| Edit message | ✅ |
| Delete message | ✅ |
| Text limit | 20 000 chars |

## Files

| File | Purpose |
|---|---|
| `dingtalk.go` | `Bot` struct, lifecycle, send/react/edit/delete; token cache; webhook map |
| `adapter.go` | Converts `chatbot.BotCallbackDataModel` → `core.Message` |
| `interaction.go` | Builds DingTalk card button layouts from `interaction.Interaction` |
