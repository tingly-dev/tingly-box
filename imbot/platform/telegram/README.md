# platform/telegram

Telegram bot implementation using the [go-telegram/bot](https://github.com/go-telegram/bot) SDK.

## Authentication

**Type:** `token`

Obtain a bot token from [@BotFather](https://t.me/botfather) on Telegram.

```go
Auth: core.AuthConfig{
    Type:  "token",
    Token: "123456:ABC-DEF...",
}
```

The token value may be a literal string or an environment-variable reference
prefixed with `$` (e.g. `"$TELEGRAM_BOT_TOKEN"`).

## Configuration

```go
&core.Config{
    Platform: core.PlatformTelegram,
    Auth: core.AuthConfig{
        Type:  "token",
        Token: os.Getenv("TELEGRAM_BOT_TOKEN"),
    },
    Options: map[string]interface{}{
        // Optional: route all API calls through a proxy.
        // Supported schemes: socks5, socks5h, http, https.
        "proxy": "socks5://user:pass@proxy-host:1080",

        // Optional: enable verbose SDK-level debug logging.
        "debug": false,
    },
}
```

## Connection model

The bot uses **long-polling** (`bot.Start(ctx)`) to receive updates from the
Telegram API. No inbound port or public URL is required. The polling loop runs
in a background goroutine and is stopped when `Disconnect` cancels the context.

Two handler types are registered on connect:

| Handler type | Purpose |
|---|---|
| `HandlerTypeMessageText` | All incoming text messages |
| `HandlerTypeCallbackQueryData` | Inline keyboard button presses |

Callback queries are automatically acknowledged
(`AnswerCallbackQuery`) after the adapted message is emitted, which removes
the loading spinner on the user's button.

## Message ID format

`messageID` is a plain decimal integer string (e.g. `"42"`).

The bot maintains an internal `messageIDMap` (`chatID → last message ID`) so
that `React` can resolve the target chat from a bare message ID.

## Capabilities

| Feature | Supported |
|---|---|
| Send text | ✅ |
| Send media (photo, video, audio, document, sticker, GIF) | ✅ |
| Inline keyboards (callback buttons, URL buttons) | ✅ |
| Reply keyboards | ✅ |
| Chat menu button | ✅ |
| Native commands (`/cmd`) | ✅ |
| Reactions | ✅ |
| Edit message | ✅ |
| Delete message | ✅ |
| Threads (forum topics) | ✅ |
| Polls | ✅ |
| Proxy support (SOCKS5 / HTTP) | ✅ |
| Text limit | 4 096 chars |

## Platform-specific API

Cast the `core.Bot` to `*telegram.Bot` for Telegram-only features:

```go
import "github.com/tingly-dev/tingly-box/imbot/platform/telegram"

if tg, ok := bot.(*telegram.Bot); ok {
    // Set the bot command list shown in the Telegram menu.
    tg.SetCommandList(registry.BuildTelegramMenuCommands())

    // Change the menu button shown in the chat header.
    tg.SetMenuButton(telegram.MenuButtonConfig{
        Type: telegram.MenuButtonTypeCommands,
    })

    // Or link a Web App.
    tg.SetMenuButton(telegram.MenuButtonConfig{
        Type: telegram.MenuButtonTypeWebApp,
        Text: "Open App",
        URL:  "https://example.com/app",
    })
}
```

`imbot.AsTelegramBot(bot)` is a convenience wrapper that performs the same
type assertion.

## Files

| File | Purpose |
|---|---|
| `telegram.go` | `Bot` struct, lifecycle (`Connect`/`Disconnect`), send/react/edit/delete |
| `adapter.go` | Converts `models.Message` / `CallbackQuery` → `core.Message` |
| `interaction.go` | Builds Telegram inline keyboards from `interaction.Interaction` |
| `menu.go` | Converts `menu.Menu` → Telegram reply/inline keyboards |
| `menu_setup.go` | Sets bot commands and menu button via API |
