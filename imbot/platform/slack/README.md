# platform/slack

Slack bot implementation using [slack-go/slack](https://github.com/slack-go/slack).

## Authentication

**Type:** `token`

Create a Slack App at [api.slack.com/apps](https://api.slack.com/apps), add a
bot user, install the app to your workspace, and copy the **Bot User OAuth
Token** (`xoxb-...`).

```go
Auth: core.AuthConfig{
    Type:  "token",
    Token: "xoxb-YOUR_BOT_TOKEN",
}
```

## Configuration

```go
&core.Config{
    Platform: core.PlatformSlack,
    Auth: core.AuthConfig{
        Type:  "token",
        Token: os.Getenv("SLACK_BOT_TOKEN"),
    },
    Options: map[string]interface{}{
        // Optional: App-Level Token for Socket Mode (xapp-...).
        // If provided, the bot uses Socket Mode instead of RTM.
        "appToken": os.Getenv("SLACK_APP_TOKEN"),
    },
}
```

### Required OAuth scopes

`channels:history`, `channels:read`, `chat:write`, `groups:history`,
`groups:read`, `im:history`, `im:read`, `mpim:history`, `mpim:read`,
`reactions:write`, `users:read`.

## Connection model

`Connect` calls `client.AuthTest()` to verify credentials, then starts an
**RTM (Real-Time Messaging)** session via `slack.NewRTM()`.  The RTM loop
runs in a background goroutine and delivers events over a channel.

If an `appToken` option is set, Socket Mode is used instead of RTM, which
does not require a public inbound URL.

## Message ID format

Slack encodes message references as `"channelID:timestamp"` (colon-separated),
where `timestamp` is the Slack message timestamp string (e.g.
`"C012AB3CD:1512085950.000216"`).

For threaded replies the format is extended to
`"channelID:timestamp:thread:threadTimestamp"`.

`React`, `EditMessage`, and `DeleteMessage` all parse this format.

## Capabilities

| Feature | Supported |
|---|---|
| Send text | ✅ |
| Send media (files) | ✅ |
| Block Kit buttons and menus | ✅ |
| Reactions | ✅ |
| Edit message | ✅ |
| Delete message | ✅ |
| Threads | ✅ |
| DMs | ✅ |
| Text limit | 40 000 chars |

## Files

| File | Purpose |
|---|---|
| `slack.go` | `Bot` struct, lifecycle, send/react/edit/delete |
| `slack_test.go` | Unit tests |
