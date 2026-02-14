# IMBot - Unified IM Bot Framework for Go

A unified, extensible framework for building IM bots that work across multiple messaging platforms.

## Features

- **Unified API** - Single interface for all platforms
- **Type-Safe** - Full Go type safety with compile-time checks
- **Extensible** - Easy to add new platforms
- **Auto-Reconnect** - Built-in reconnection with configurable attempts and delay
- **Multi-Platform** - Manage multiple bots from different platforms in one manager
- **Well-Tested** - Comprehensive test coverage with mock platform
- **Production Ready** - Reliable and performant

## Supported Platforms

- ✅ **Telegram** - Full support with inline keyboards, polls, and media
- ✅ **Discord** - Basic support (in development)
- ✅ **Slack** - Basic support (in development)
- ✅ **Feishu/Lark** - Basic support (in development)
- ✅ **WhatsApp** - Basic support (in development)
- ✅ **WebChat** - Mock platform for testing
- 🚧 Google Chat (coming soon)
- 🚧 Signal (coming soon)
- 🚧 BlueBubbles/iMessage (coming soon)

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

    "github.com/tingly-dev/tingly-box/imbot"
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

    "github.com/tingly-dev/tingly-box/imbot"
    "github.com/tingly-dev/tingly-box/imbot/internal/core"
)

func main() {
    manager := imbot.NewManager(
        imbot.WithAutoReconnect(true),
        imbot.WithMaxReconnectAttempts(5),
        imbot.WithReconnectDelay(3000),
    )

    // Add multiple platforms
    configs := []*imbot.Config{
        {
            Platform: core.PlatformTelegram,
            Enabled:  true,
            Auth: imbot.AuthConfig{
                Type:  "token",
                Token: os.Getenv("TELEGRAM_TOKEN"),
            },
        },
        {
            Platform: core.PlatformDiscord,
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
    manager.OnMessage(func(msg imbot.Message, platform core.Platform) {
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
        "webhookUrl":   "",     // Optional webhook URL
        "useWebhook":   false,  // Use polling instead
        "updateTimeout": 30,    // Polling timeout in seconds
        "debug":        false,
    },
    Logging: &imbot.LoggingConfig{
        Level:      "info",
        Timestamps: true,
    },
}
```

### Auth Configuration

The framework supports multiple authentication methods:

```go
// Token authentication (most platforms)
Auth: imbot.AuthConfig{
    Type:  "token",
    Token: "your-bot-token",
}

// Basic authentication
Auth: imbot.AuthConfig{
    Type:     "basic",
    Username: "username",
    Password: "password",
}

// OAuth authentication
Auth: imbot.AuthConfig{
    Type:         "oauth",
    ClientID:     "client-id",
    ClientSecret: "client-secret",
    RedirectURI:  "redirect-uri",
}

// Service Account (for Google Chat)
Auth: imbot.AuthConfig{
    Type:              "serviceAccount",
    ServiceAccountJSON: "path/to/service-account.json",
}

// QR Code authentication (for WhatsApp)
Auth: imbot.AuthConfig{
    Type:      "qr",
    AuthDir:   "./auth",
    AccountID: "account-id",
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

Or use the `$ENV_VAR` syntax directly in config files:

```yaml
auth:
  type: token
  token: $TELEGRAM_BOT_TOKEN
```

### Manager Options

```go
manager := imbot.NewManager(
    imbot.WithAutoReconnect(true),           // Enable auto-reconnect
    imbot.WithMaxReconnectAttempts(10),      // Max reconnect attempts
    imbot.WithReconnectDelay(5000),          // Delay in milliseconds
)
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

### Send to Multiple Platforms

```go
targets := []imbot.Target{
    imbot.NewTarget("telegram", "chat-id-1"),
    imbot.NewTarget("discord", "channel-id-1"),
}

results := manager.Broadcast(targets, &imbot.SendMessageOptions{
    Text: "Hello to all platforms!",
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

// Bot ready
manager.OnReady(func(platform imbot.Platform) {
    log.Printf("%s bot is ready", platform)
})
```

## Content Types

### Working with Message Content

```go
// Check content type
if msg.IsTextContent() {
    text := msg.GetText()
}

if msg.IsMediaContent() {
    media := msg.GetMedia()
}

if msg.IsPollContent() {
    poll := msg.GetPoll()
}

if msg.IsReactionContent() {
    reaction := msg.GetReaction()
}

if msg.IsSystemContent() {
    // Handle system events
}
```

### Creating Content

```go
// Text content with entities
textContent := imbot.NewTextContent("Hello @user!", []imbot.Entity{
    {Type: "mention", Offset: 6, Length: 5},
})

// Media content
mediaContent := imbot.NewMediaContent([]imbot.MediaAttachment{
    {Type: "image", URL: "https://example.com/image.jpg"},
}, "Check this out!")

// Poll content
pollContent := imbot.NewPollContent(imbot.Poll{
    Question: "What's your favorite color?",
    Options: []imbot.PollOption{
        {ID: "1", Text: "Red"},
        {ID: "2", Text: "Blue"},
        {ID: "3", Text: "Green"},
    },
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

// Use platform-specific features via Metadata
bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
    Text: "Choose an option:",
    Metadata: map[string]interface{}{
        "replyMarkup": tgbotapi.NewInlineKeyboardMarkup(
            tgbotapi.NewInlineKeyboardRow(
                tgbotapi.NewInlineKeyboardButtonData("Option 1", "opt1"),
                tgbotapi.NewInlineKeyboardButtonData("Option 2", "opt2"),
            ),
        ),
    },
})
```

### Discord

```go
config := &imbot.Config{
    Platform: imbot.PlatformDiscord,
    Auth: imbot.AuthConfig{
        Type:  "token",
        Token: "your-discord-bot-token",
    },
    Options: map[string]interface{}{
        "intents": []string{"Guilds", "GuildMessages", "MessageContent"},
    },
}
```

### Slack

```go
config := &imbot.Config{
    Platform: imbot.PlatformSlack,
    Auth: imbot.AuthConfig{
        Type:         "oauth",
        ClientID:     "your-client-id",
        ClientSecret: "your-client-secret",
    },
}
```

### WhatsApp

```go
config := &imbot.Config{
    Platform: imbot.PlatformWhatsApp,
    Auth: imbot.AuthConfig{
        Type:    "qr",
        AuthDir: "./auth",
    },
}
```

### Feishu/Lark

```go
config := &imbot.Config{
    Platform: imbot.PlatformFeishu,
    Auth: imbot.AuthConfig{
        Type:         "oauth",
        ClientID:     "your-app-id",
        ClientSecret: "your-app-secret",
    },
}
```

## Error Handling

```go
manager.OnError(func(err error, platform imbot.Platform) {
    // Check error type
    if imbot.IsBotError(err) {
        botErr := err.(*imbot.BotError)
        code := imbot.GetErrorCode(err)

        switch code {
        case imbot.ErrAuthFailed:
            log.Printf("Authentication failed: %v", botErr)
        case imbot.ErrRateLimited:
            log.Printf("Rate limited, retry after: %v", botErr)
        case imbot.ErrConnectionFailed:
            if imbot.IsRecoverable(botErr) {
                log.Printf("Connection failed but recoverable")
            }
        }
    }
})
```

## Platform Capabilities

```go
// Check if platform supports a feature
caps := imbot.GetPlatformCapabilities("telegram")

if caps.SupportsFeature("polls") {
    // Create poll
}

if caps.SupportsMediaType("image") {
    // Send image
}

// Get text limit
if len(text) > caps.TextLimit {
    // Truncate message
}
```

## Bot Status

```go
// Get status of all bots
statuses := manager.GetStatus()
for key, status := range statuses {
    if status.IsHealthy() {
        log.Printf("%s: Healthy", key)
    } else {
        log.Printf("%s: %v", key, status)
    }
}
```

## Testing

```go
package main

import (
    "context"
    "testing"

    "github.com/tingly-dev/tingly-box/imbot"
    "github.com/tingly-dev/tingly-box/imbot/internal/platform"
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

## Examples

See the `examples/` directory for complete examples:

- `examples/basic/` - Basic Telegram bot with commands
- `examples/multi_platform/` - Multi-platform bot example

Run examples:

```bash
cd examples/basic
go run telegram-bot.go
```

## License

Mozilla Public License Version 2.0
