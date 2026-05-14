# platform/whatsapp

WhatsApp Business bot implementation using the
[Meta Cloud API](https://developers.facebook.com/docs/whatsapp/cloud-api).

## Authentication

**Type:** `token` or `qr`

The `token` type is recommended for production.  Obtain a permanent or
temporary access token from [Meta for Developers](https://developers.facebook.com/),
set up a WhatsApp Business App, and note the **Phone Number ID**.

```go
Auth: core.AuthConfig{
    Type:  "token",
    Token: "EAAxxxxxxxxxx...", // Meta access token
}
```

`qr` auth requires a pre-configured API key from a Baileys-based session and
is not recommended for server deployments.

## Configuration

```go
&core.Config{
    Platform: core.PlatformWhatsApp,
    Auth: core.AuthConfig{
        Type:  "token",
        Token: os.Getenv("WHATSAPP_TOKEN"),
    },
    Options: map[string]interface{}{
        // Required: WhatsApp Business phone-number ID (not the phone number itself).
        "phoneId": os.Getenv("WHATSAPP_PHONE_ID"),

        // Optional: Meta Graph API base URL (default: https://graph.facebook.com/v20.0).
        "apiUrl": "https://graph.facebook.com/v20.0",

        // Optional: local directory for session storage.
        "authDir": "./whatsapp-auth",
    },
}
```

## Connection model

WhatsApp uses a **stateless REST** design:

- **Sending** — Each `SendMessage` call is an HTTPS POST to
  `{apiUrl}/{phoneId}/messages` with a 30-second timeout.
- **Receiving** — Incoming messages arrive as webhook events from Meta. These
  are fed to the bot from the external HTTP server via the adapter; the bot
  itself does not open an inbound socket.

`Connect` calls an authentication probe against the Meta API and starts a
background goroutine for polling/webhook receipt.

## Message structure

Incoming messages are parsed from Meta's
[`MessageEvent`](whatsapp.go) structure which wraps the standard webhook
payload format (`object → entry[].changes[].value.messages[]`).

## Capabilities

| Feature | Supported |
|---|---|
| Send text | ✅ |
| Send media (image, audio, video, document) | ✅ |
| Read receipts | ✅ |
| Reactions | ❌ |
| Edit / Delete message | ❌ |
| Interactive buttons | ❌ (planned via template messages) |
| Text limit | 4 096 chars |

## Files

| File | Purpose |
|---|---|
| `whatsapp.go` | `Bot` struct, lifecycle, send methods; `MessageEvent` / `SendMessageResponse` types |
