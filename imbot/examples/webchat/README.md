# WebChat Bot Example

This example demonstrates how to use the WebChat bot platform with imbot.

## Features

- WebSocket-based real-time chat
- Built-in web interface (no separate frontend needed!)
- Command handling (/start, /ping, /time, /info, /status, /menu)
- Interactive keyboard support
- Auto-reconnection
- Multiple session support

## Running the Example

### Start the Bot

**Option 1: From the imbot directory**
```bash
cd imbot
go run ./examples/webchat/*.go
```

**Option 2: From the webchat example directory**
```bash
cd imbot/examples/webchat
go run .
```

The bot will start on `:8080` by default. You can change this with the `WEBSHOT_ADDR` environment variable:

```bash
WEBSHOT_ADDR=:3000 go run ./examples/webchat/*.go
```

### Open the Web Interface

Simply open your browser and navigate to:

```
http://localhost:8080/
```

That's it! The chat interface is embedded in the server itself.

## WebSocket Message Format

### Sending Messages

```json
{
  "id": "msg_123",
  "senderId": "user_abc",
  "senderName": "John Doe",
  "text": "Hello, bot!",
  "timestamp": 1234567890
}
```

### Receiving Messages

```json
{
  "id": "msg_456",
  "senderId": "bot",
  "senderName": "Bot",
  "text": "📢 You said: Hello, bot!",
  "timestamp": 1234567891,
  "metadata": {}
}
```

## Available Commands

The demo bot supports the following commands:

### Basic Commands
- `/start`, `/help` - Show help message
- `/ping` - Check bot status
- `/about` - About this bot

### Text Commands
- `/echo <text>` - Echo back your message
- `/reverse <text>` - Reverse the text
- `/upper <text>` - Convert to UPPERCASE
- `/lower <text>` - Convert to lowercase

### Info Commands
- `/time` - Show current time
- `/date` - Show today's date
- `/info` - Show your user information

### Fun Commands
- `/roll [max]` - Roll a dice (default: 100)
- `/flip` - Flip a coin
- `/8ball` - Magic 8-ball response
- `/joke` - Tell a random joke
- `/quote` - Show an inspirational quote

You can also just send any text without a command and the bot will echo it back!

## Integration with Other Platforms

The WebChat bot works seamlessly with the imbot Manager. You can add multiple platforms:

```go
manager := imbot.NewManager()

// Add WebChat
manager.AddBot(&imbot.Config{
    Platform: imbot.PlatformWebChat,
    Enabled:  true,
    Options: map[string]interface{}{"addr": ":8080"},
})

// Add Telegram
manager.AddBot(&imbot.Config{
    Platform: imbot.PlatformTelegram,
    Enabled:  true,
    Auth: imbot.AuthConfig{Type: "token", Token: "..."},
})

// Same handler works for all platforms
manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
    // Handle message from any platform
    manager.SendTo(platform, msg.Sender.ID, &imbot.SendMessageOptions{
        Text: "Response",
    })
})
```

## Next Steps

- Add JWT authentication for production use
- Implement message history persistence
- Add file upload support
- Add rate limiting
- Create multi-room support
