# IMBot - Unified IM Bot Framework for Go

A unified, extensible framework for building IM bots that work across multiple messaging platforms.

## Features

- **Unified API** - Single interface for all platforms
- **Type-Safe** - Full Go type safety with compile-time checks
- **Extensible** - Easy to add new platforms
- **Well-Tested** - Comprehensive test coverage
- **Production Ready** - Reliable and performant

## Supported Platforms

- ✅ Telegram
- 🚧 Discord (coming soon)
- 🚧 Slack (coming soon)
- 🚧 WhatsApp (coming soon)
- 🚧 Google Chat (coming soon)
- 🚧 Signal (coming soon)
- 🚧 BlueBubbles/iMessage (coming soon)
- 🚧 Feishu/Lark (coming soon)
- 🚧 WebChat (for testing)

## Installation

```bash
go get github.com/tingly-dev/tingly-box/imbot
```

## Quick Start

### Basic Telegram Bot

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/tingly-dev/tingly-box/imbot/pkg"
)

func main() {
    // Create bot manager
    manager := imbot.NewManager()

    // Add Telegram bot
    err := manager.AddBot(&imbot.Config{
        Platform: imbot.PlatformTelegram,
        Enabled:  true,
        Auth: imbot.AuthConfig{
            Type:  "token",
            Token: os.Getenv("TELEGRAM_BOT_TOKEN"),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Set message handler
    manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
        log.Printf("[%-10s] %s: %s", platform, msg.Sender.DisplayName, msg.GetText())

        // Reply
        bot := manager.GetBot(platform)
        if bot != nil {
            bot.SendText(context.Background(), msg.Sender.ID, "Echo: "+msg.GetText())
        }
    })

    // Start manager
    if err := manager.Start(context.Background()); err != nil {
        log.Fatal(err)
    }

    // Wait forever
    select {}
}
```

### Multi-Platform Bot

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/tingly-dev/tingly-box/imbot/pkg"
)

func main() {
    manager := imbot.NewManager(
        imbot.WithAutoReconnect(true),
        imbot.WithMaxReconnectAttempts(5),
    )

    // Add multiple platforms
    configs := []*imbot.Config{
        {
            Platform: imbot.PlatformTelegram,
            Enabled:  true,
            Auth: imbot.AuthConfig{
                Type:  "token",
                Token: os.Getenv("TELEGRAM_TOKEN"),
            },
        },
        {
            Platform: imbot.PlatformDiscord,
            Enabled:  true,
            Auth: imbot.AuthConfig{
                Type:  "token",
                Token: os.Getenv("DISCORD_TOKEN"),
            },
        },
    }

    if err := manager.AddBots(configs); err != nil {
        log.Fatal(err)
    }

    // Unified message handler
    manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
        log.Printf("[%-10s] %s: %s", platform, msg.Sender.DisplayName, msg.GetText())

        bot := manager.GetBot(platform)
        if bot != nil {
            bot.SendText(context.Background(), msg.Sender.ID, "Thanks for your message!")
        }
    })

    manager.Start(context.Background())
    select {}
}
```

## Configuration

### Bot Configuration

```go
config := &imbot.Config{
    Platform: imbot.PlatformTelegram,
    Enabled:  true,
    Auth: imbot.AuthConfig{
        Type:  "token",
        Token: "your-bot-token",
    },
    Options: map[string]interface{}{
        "webhookUrl": "",      // Optional webhook URL
        "useWebhook":  false,  // Use polling instead
    },
    Logging: &imbot.LoggingConfig{
        Level:      "info",
        Timestamps: true,
    },
}
```

### Environment Variables

You can use environment variables in your config:

```go
config := &imbot.Config{
    Auth: imbot.AuthConfig{
        Token: "$TELEGRAM_BOT_TOKEN",  // Will be read from environment
    },
}
```

## Message Handling

### Send Text Message

```go
bot.SendText(ctx, "chat-id", "Hello, World!")
```

### Send Message with Options

```go
bot.SendMessage(ctx, "chat-id", &imbot.SendMessageOptions{
    Text:      "Hello, World!",
    ParseMode: imbot.ParseModeMarkdown,
    Silent:    false,
})
```

### Send Media

```go
bot.SendMedia(ctx, "chat-id", []imbot.MediaAttachment{
    {
        Type: "image",
        URL:  "https://example.com/image.jpg",
    },
})
```

### Reply to Message

```go
bot.SendMessage(ctx, "chat-id", &imbot.SendMessageOptions{
    Text:    "Replying to your message",
    ReplyTo: messageID,
})
```

## Event Handlers

```go
// Message received
manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
    log.Printf("Message from %s: %s", msg.Sender.DisplayName, msg.GetText())
})

// Error occurred
manager.OnError(func(err error, platform imbot.Platform) {
    log.Printf("Error on %s: %v", platform, err)
})

// Bot connected
manager.OnConnected(func(platform imbot.Platform) {
    log.Printf("%s bot connected", platform)
})

// Bot disconnected
manager.OnDisconnected(func(platform imbot.Platform) {
    log.Printf("%s bot disconnected", platform)
})
```

## Platform-Specific Features

### Telegram

```go
config := &imbot.Config{
    Platform: imbot.PlatformTelegram,
    Auth: imbot.AuthConfig{
        Type:  "token",
        Token: "your-telegram-bot-token",
    },
    Options: map[string]interface{}{
        "updateTimeout": 30,  // Polling timeout in seconds
        "debug":        false,
    },
}
```

## Testing

```go
package main

import (
    "context"
    "testing"

    "github.com/tingly-dev/tingly-box/imbot/internal/platform"
    "github.com/tingly-dev/tingly-box/imbot/pkg"
)

func TestMockBot(t *testing.T) {
    // Create mock bot
    bot, err := platform.NewMockBot(&imbot.Config{
        Platform: imbot.PlatformWebChat,
    })
    if err != nil {
        t.Fatal(err)
    }

    // Connect
    ctx := context.Background()
    if err := bot.Connect(ctx); err != nil {
        t.Fatal(err)
    }

    // Send message
    result, err := bot.SendText(ctx, "test-user", "Hello!")
    if err != nil {
        t.Fatal(err)
    }

    if result.MessageID == "" {
        t.Error("Expected message ID")
    }

    // Disconnect
    if err := bot.Disconnect(ctx); err != nil {
        t.Fatal(err)
    }
}
```

## License

Mozilla Public License Version 2.0
