# DingTalk Bot Example

A DingTalk (钉钉) bot example using the imbot framework.

## Features

- Command handling system (`/start`, `/ping`, `/echo`, `/time`, `/info`, `/status`, `/about`)
- User whitelist support
- Media handling (images, videos, audio, documents)
- Error handling and auto-reconnect
- Stream Mode connection for real-time events

## Prerequisites

1. Create an enterprise internal bot app in [DingTalk Open Platform](https://open.dingtalk.com/)
2. Obtain the following credentials:
   - **AppKey** (also called Client ID)
   - **AppSecret** (also called Client Secret)
   - **Stream URL** for Stream Mode connection

3. Configure your bot app permissions:
   - Enable message receiving
   - Enable Stream Mode
   - Configure webhook/subscription if needed

## Setup

### 1. Set Environment Variables

```bash
export DINGTALK_APP_KEY="your_app_key"
export DINGTALK_APP_SECRET="your_app_secret"
export DINGTALK_STREAM_URL="wss://your_stream_url"
```

### 2. Run the Bot

```bash
cd imbot/examples/dingtalk
go run main.go
```

## Configuration Options

| Option | Description | Default |
|---------|-------------|----------|
| `DINGTALK_APP_KEY` | Your DingTalk AppKey | Required |
| `DINGTALK_APP_SECRET` | Your DingTalk AppSecret | Required |
| `DINGTALK_STREAM_URL` | WebSocket URL for Stream Mode | Required |

## Commands

| Command | Description | Aliases |
|---------|-------------|----------|
| `/start` | Show welcome message and help | `help` |
| `/ping` | Check bot latency | - |
| `/echo <text>` | Repeat the message back | - |
| `/time` | Show current time | - |
| `/info` | Show user information | - |
| `/status` | Show bot connection status | - |
| `/about` | Show bot information | - |

## User Whitelist

To restrict bot usage to specific users, modify the `WHITE_LIST` in `main.go`:

```go
var WHITE_LIST = []string{
    "user_id_1",
    "user_id_2",
}
```

When the whitelist is empty, all users can interact with the bot.

## DingTalk API Notes

### Stream Mode vs Webhook Mode

This example uses **Stream Mode**, which:
- Provides real-time event delivery via WebSocket
- Is easier to set up than traditional webhooks
- Requires less infrastructure (no public URL needed)

### Conversation Types

DingTalk has two conversation types:
- **Type "1"**: 1-on-1 chat (direct message)
- **Type "2"**: Group chat

### Message Types

The bot supports:
- Text messages
- Images
- Videos
- Audio files
- Documents

### Rate Limits

DingTalk API has rate limits (typically 50 requests/minute). The bot implements auto-reconnect for connection issues.

## Troubleshooting

### Connection Failures

- Verify your AppKey and AppSecret are correct
- Check that Stream Mode is enabled for your app
- Ensure your bot has the necessary permissions

### Authentication Errors

- Confirm your bot app is approved for your organization
- Check that the AppSecret hasn't expired

### Message Not Sending

- Verify the conversation ID is correct
- Check bot has permission to send to that conversation
- Review rate limit status

## Resources

- [DingTalk Open Platform](https://open.dingtalk.com/)
- [Stream Mode Documentation](https://open.dingtalk.com/document/development/introduction-to-stream-mode)
- [DingTalk Go SDK](https://github.com/open-dingtalk/dingtalk-stream-sdk-go)
- [imbot Framework](https://github.com/tingly-dev/tingly-box)

## License

This example is part of the tingly-box project.
