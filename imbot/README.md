# IMBot - Unified IM Bot Framework for Go

A unified, extensible framework for building IM bots that work across multiple messaging platforms.

## Features

- **Unified API** - Single interface for all platforms
- **Type-Safe** - Full Go type safety with compile-time checks
- **Extensible** - Easy to add new platforms via core interfaces
- **Multi-Platform** - Manage multiple bots from different platforms in one manager
- **Rich Interactions** - Inline keyboards, commands, menus, and custom interactions
- **Markdown Support** - Cross-platform markdown conversion with entity handling
- **Well-Tested** - Comprehensive test coverage including E2E tests
- **Production Ready** - Used in tingly-box AI orchestrator

## Supported Platforms

- ✅ **Telegram** - Full support with inline keyboards, menus, commands, and media
- ✅ **Feishu/Lark** - Full support with quick actions and interactive cards
- ✅ **DingTalk** - Basic support with message handling
- ✅ **Weixin** - Basic support
- ✅ **WeWork** - Basic support (enterprise WeChat)
- 🚧 **Discord** - Planned
- 🚧 **Slack** - Planned
- 🚧 **WhatsApp** - Planned
- 🚧 **Google Chat** - Planned
- 🚧 **Signal** - Planned

## Architecture

```
imbot/
├── core/              # Core abstractions and interfaces
│   ├── bot.go         # Bot interface definition
│   ├── message.go     # Message types and content
│   ├── config.go      # Configuration structures
│   ├── errors.go      # Error handling
│   └── types.go       # Common types and constants
├── platform/          # Platform-specific implementations
│   ├── telegram/      # Telegram bot implementation
│   ├── feishu/        # Feishu/Lark bot implementation
│   ├── dingtalk/      # DingTalk bot implementation
│   ├── discord/       # Discord bot implementation
│   ├── slack/         # Slack bot implementation
│   └── weixin/        # WeixinWork bot implementation
├── interaction/       # Interactive elements (keyboards, buttons)
├── command/           # Command registry and handling
├── menu/              # Menu system (bot commands, quick actions)
├── markdown/          # Markdown parser and converter
├── manager.go         # Multi-bot manager
└── factory.go         # Bot factory for creating platform instances
```

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
)

func main() {
    manager := imbot.NewManager()

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
            Platform: imbot.PlatformFeishu,
            Enabled:  true,
            Auth: imbot.AuthConfig{
                Type:         "oauth",
                ClientID:     os.Getenv("FEISHU_APP_ID"),
                ClientSecret: os.Getenv("FEISHU_APP_SECRET"),
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
        "debug": false,
    },
}
```

### Auth Configuration

The framework supports multiple authentication methods:

```go
// Token authentication (Telegram, Discord)
Auth: imbot.AuthConfig{
    Type:  "token",
    Token: "your-bot-token",
}

// OAuth authentication (Feishu, Slack)
Auth: imbot.AuthConfig{
    Type:         "oauth",
    ClientID:     "client-id",
    ClientSecret: "client-secret",
}
```

### Environment Variables

You can use environment variables in your config:

```go
config := &imbot.Config{
    Auth: imbot.AuthConfig{
        Token: os.Getenv("TELEGRAM_BOT_TOKEN"),
    },
}
```

## Message Handling

### Send Text Message

```go
bot.SendText(ctx, "chat-id", "Hello, World!")
```

### Send Message with Markdown

```go
bot.SendMessage(ctx, "chat-id", &imbot.SendMessageOptions{
    Text:      "*Bold* _italic_ `code`",
    ParseMode: imbot.ParseModeMarkdown,
})
```

### Send Message with Inline Keyboard

```go
keyboard := imbot.NewKeyboardBuilder().
    AddRow(
        imbot.CallbackButton("Option 1", "opt:1"),
        imbot.CallbackButton("Option 2", "opt:2"),
    ).
    Build()

bot.SendMessage(ctx, "chat-id", &imbot.SendMessageOptions{
    Text:     "Choose an option:",
    Keyboard: keyboard,
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
```

## Interactive Elements

### Inline Keyboards

```go
// Create inline keyboard
keyboard := imbot.NewKeyboardBuilder().
    AddRow(
        imbot.CallbackButton("Approve", "action:approve"),
        imbot.CallbackButton("Reject", "action:reject"),
    ).
    AddRow(
        imbot.CallbackButton("Cancel", "action:cancel"),
    ).
    Build()

// Send message with keyboard
bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
    Text:     "Do you approve this action?",
    Keyboard: keyboard,
})

// Handle callback
manager.OnMessage(func(msg imbot.Message, platform imbot.Platform) {
    if msg.IsCallbackQuery() {
        parts := imbot.ParseCallbackData(msg.CallbackData)
        action := parts[0]  // "action"
        value := parts[1]   // "approve", "reject", or "cancel"

        // Process the callback
        // ...
    }
})
```

### Commands

```go
// Create command registry
registry := imbot.NewCommandRegistry()

// Register commands
registry.Register(
    imbot.NewCommand("start", "/start", "Start the bot").
        SetHandler(func(ctx *imbot.HandlerContext) error {
            return ctx.Bot.SendText(ctx.Context(), ctx.ChatID, "Welcome!")
        }).
        Build(),
)

// Set command list on Telegram bot
if tgBot, ok := imbot.AsTelegramBot(bot); ok {
    tgBot.SetCommandList(registry.ToTelegramCommands())
}
```

## Markdown Support

The framework includes a powerful markdown converter that handles cross-platform differences:

```go
import "github.com/tingly-dev/tingly-box/imbot/markdown"

// Parse markdown and convert to Telegram entities
text, entities := markdown.ConvertToTelegram("*bold* _italic_ `code`")

// Send with entities
bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
    Text:     text,
    Entities: entities,
})
```

See `markdown/USAGE.md` for detailed usage and supported formats.

## Platform-Specific Features

### Telegram

```go
// Cast to TelegramBot for platform-specific features
if tgBot, ok := imbot.AsTelegramBot(bot); ok {
    // Resolve chat ID from username or invite link
    chatID, err := tgBot.ResolveChatID("@username")

    // Set bot commands
    tgBot.SetCommandList(commands)

    // Edit message with keyboard
    tgBot.EditMessageWithKeyboard(ctx, chatID, messageID, "Updated text", keyboard)
}
```

### Feishu/Lark

```go
// Cast to FeishuBot for platform-specific features
if fsBot, ok := imbot.AsFeishuBot(bot); ok {
    // Set quick actions (shown when typing /)
    fsBot.SetQuickActions(actions)

    // Get current quick actions
    actions, err := fsBot.GetQuickActions()
}
```

## Error Handling

```go
manager.OnError(func(err error, platform imbot.Platform) {
    // Check if it's a bot error
    if imbot.IsBotError(err) {
        botErr := err.(*imbot.BotError)
        code := imbot.GetErrorCode(err)

        switch code {
        case imbot.ErrAuthFailed:
            log.Printf("Authentication failed: %v", botErr)
        case imbot.ErrRateLimited:
            log.Printf("Rate limited: %v", botErr)
        case imbot.ErrConnectionFailed:
            log.Printf("Connection failed: %v", botErr)
        }
    }
})
```

## Testing

The framework includes comprehensive E2E tests for Telegram:

```bash
# Set environment variables
export TELEGRAM_BOT_TOKEN="your-bot-token"
export TELEGRAM_TEST_CHAT_ID="your-chat-id"

# Run tests
make test-telegram-e2e
```

See `tests/TELEGRAM_E2E_TESTS.md` for detailed testing documentation.

## Examples

See the `examples/` directory for complete examples:

- `examples/telegram/` - Telegram bot with commands and keyboards
- `examples/multi_platform/` - Multi-platform bot example
- `examples/dingtalk/` - DingTalk bot example

Run examples:

```bash
cd examples/telegram
go run telegram-bot.go
```

## Development

### Building

```bash
make build
```

### Running Tests

```bash
# Unit tests
make test

# E2E tests (requires environment variables)
make test-telegram-e2e
```

### Project Structure

- **core/** - Core abstractions, independent of platform implementations
- **platform/** - Platform-specific bot implementations
- **interaction/** - Interactive elements (keyboards, buttons, forms)
- **command/** - Command registry and handling system
- **menu/** - Menu system for bot commands and quick actions
- **markdown/** - Markdown parsing and cross-platform conversion
- **tests/** - E2E and integration tests

## Used In

This framework is used in [tingly-box](https://github.com/tingly-dev/tingly-box), an AI orchestrator that provides:
- LLM gateway with multi-provider support
- Remote control via IM platforms
- Smart routing and load balancing
- Context optimization

## License

Mozilla Public License Version 2.0
