# Multi-Platform Bot Example

This example demonstrates how to use the imbot framework to create a multi-platform bot, achieving "Write Once, Run Everywhere".

## Supported Platforms

| Platform | Environment Variables | Auth Method |
|----------|----------------------|-------------|
| Telegram | `TELEGRAM_BOT_TOKEN` | Token |
| DingTalk | `DINGTALK_APP_KEY`, `DINGTALK_APP_SECRET` | OAuth |
| Feishu | `FEISHU_APP_ID`, `FEISHU_APP_SECRET` | OAuth |
| Discord | `DISCORD_BOT_TOKEN` | Token |

## Quick Start

### 1. Set Environment Variables

```bash
# Telegram
export TELEGRAM_BOT_TOKEN="your-telegram-bot-token"

# DingTalk
export DINGTALK_APP_KEY="your-app-key"
export DINGTALK_APP_SECRET="your-app-secret"

# Feishu
export FEISHU_APP_ID="your-app-id"
export FEISHU_APP_SECRET="your-app-secret"

# Discord
export DISCORD_BOT_TOKEN="your-discord-bot-token"
```

### 2. Run

```bash
cd imbot/examples/multi_platform
go run main.go
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                Business Logic (Shared)                   │
│       Command handling, message processing, whitelist    │
└─────────────────────────────────────────────────────────┘
                          ▲
                          │ core.Message (Unified message type)
                          │
┌──────────┬──────────┬───┴───────┬──────────┬──────────┐
│ Telegram │ Discord  │  Feishu   │ DingTalk │  New     │
│ Adapter  │ Adapter  │  Adapter  │ Adapter  │ Platform │
└──────────┴──────────┴───────────┴──────────┴──────────┘
```

### Core Concepts

1. **Unified Message Type** - `core.Message` encapsulates messages from all platforms
2. **Unified Command System** - Command handlers are identical across all platforms
3. **Platform Adapters** - Each platform has its own adapter for message conversion

### Key Code

```go
// Command definitions - shared across all platforms
var Commands = []Command{
    {Name: "start", Handler: cmdStart, Aliases: []string{"help"}},
    {Name: "ping", Handler: cmdPing},
    {Name: "echo", Handler: cmdEcho},
    // ...
}

// Unified message handling - write once
manager.OnMessage(func(msg imbot.Message, platform core.Platform) {
    handleMessage(ctx, manager, msg, platform)
})
```

## Available Commands

| Command | Description |
|---------|-------------|
| `/start`, `/help` | Show help |
| `/ping` | Check bot status |
| `/echo <message>` | Echo the message |
| `/time` | Show current time |
| `/info` | Show user info |
| `/status` | Show bot status |
| `/platform` | Show current platform |
| `/about` | About the bot |

## Adding a New Platform

To add a new platform, you only need to:

1. Add configuration loading in `loadConfigs()`
2. Ensure the platform is registered in the imbot framework

No changes needed to business logic!

```go
// Add new platform configuration
if token := os.Getenv("NEW_PLATFORM_TOKEN"); token != "" {
    configs = append(configs, &imbot.Config{
        Platform: core.PlatformNewPlatform,
        Enabled:  true,
        Auth: imbot.AuthConfig{
            Type:  "token",
            Token: token,
        },
    })
}
```
