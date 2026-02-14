# Adapter Package

The adapter package provides platform-specific event/message adapters that convert raw platform events to unified `core.Message` objects.

## Overview

Each adapter implements the `MessageAdapter` interface, providing:
- `AdaptMessage()` - Convert platform messages to core.Message
- `AdaptCallback()` - Convert platform callbacks (button clicks, etc.)
- `Platform()` - Return the platform identifier

## Usage Example

```go
import (
    "github.com/tingly-dev/tingly-box/imbot/internal/adapter"
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Create adapter
telegramAdapter := adapter.NewTelegramAdapter(config, botAPI)

// Adapt incoming message
var message *tgbotapi.Message
// ... receive from Telegram ...

coreMsg, err := telegramAdapter.AdaptMessage(ctx, message)
if err != nil {
    log.Fatal(err)
}

// Use the unified message
fmt.Printf("Sender: %s\n", coreMsg.Sender.DisplayName)
fmt.Printf("Text: %s\n", coreMsg.GetText())
```

## Registered Adapters

### Telegram
- File: `telegram.go`
- Type: `TelegramAdapter`
- Handles: Messages, Callback queries
- Content types: Text, Photo, Document, Sticker, Video, Audio

### Discord
- File: `discord.go`
- Type: `DiscordAdapter`
- Handles: Messages, Reactions
- Content types: Text, Embeds, Attachments

### Feishu/Lark
- File: `feishu.go`
- Type: `FeishuAdapter`
- Handles: Webhook events
- Content types: Text, Post (rich content), Image, Video, Audio, File

## Integration with Existing Platform Bots

The adapters are designed to be used by existing platform implementations:

```go
// In imbot/internal/platform/telegram/telegram.go

type Bot struct {
    *core.BaseBot
    api     *tgbotapi.BotAPI
    adapter *adapter.TelegramAdapter  // New field
    // ...
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
    // Use adapter instead of manual mapping
    message, err := b.adapter.AdaptMessage(b.ctx, msg)
    if err != nil {
        b.Logger().Error("Failed to adapt message: %v", err)
        return
    }

    b.EmitMessage(*message)
}
```

## Benefits

1. **Code Reduction** - Eliminates 60-100 lines of manual mapping per platform
2. **Consistency** - Uniform message structure across all platforms
3. **Testability** - Adapters can be tested independently
4. **Maintainability** - Changes to message structure affect adapters only
5. **Type Safety** - Compile-time checks for all conversions
