# imbot — unified IM bot framework for Go

A unified, extensible framework for building IM bots that run across many
messaging platforms through one API. Used in production by the
[tingly-box](https://github.com/tingly-dev/tingly-box) AI orchestrator.

> This README is the entry point. Per-package and per-platform detail lives in
> `core/README.md`, `platform/README.md`, and each `platform/<name>/README.md`.

## Layering

```
imbot/                       public API + multi-bot runtime
├── imbot.go                 re-exports core types; Manager entry; cast helpers (AsTelegramBot, AsRestater)
├── manager.go               Manager — owns bot lifecycle, fan-out event handlers, reconnect
├── factory.go / registry.go CreateBot / global registry + RegisterBuiltinPlatforms
├── menu_setup.go            per-platform native command-menu installer dispatch
├── auth.go                  settings-UI form fields + GetAllPlatforms/GetPlatformConfig (intrinsic auth metadata lives in core)
│
├── core/                    platform-neutral vocabulary — knows no concrete SDK
├── markdown/                cross-platform markdown → entity conversion
├── platform/                one package per platform, each implementing core.Bot
└── examples/, tests/        runnable examples and E2E tests
```

The three layers are strictly ordered: **`core`** holds the abstractions;
**`imbot`** (this package) is the flat entry that wires them into a runtime;
**`platform/*`** are the per-platform adapters. Nothing in `core` imports a
platform package. See `.design/imbot-platform-seams.md` for the design
rationale behind actions, payloads, capabilities, and restate.

- `core/` internals → [`core/README.md`](core/README.md)
- platform registry & "add a new platform" → [`platform/README.md`](platform/README.md)
- markdown rendering → [`markdown/README.md`](markdown/README.md)

## Supported platforms

`core.Platform` constants: `telegram`, `discord`, `slack`, `feishu`, `lark`,
`dingtalk`, `weixin`, `wecom`, `whatsapp`, and the internal test platform
`tingly`. The single source of truth for display names, capabilities,
per-platform behavior, **and auth metadata** (credential type, UI category,
and the auth-map → `AuthConfig` wiring) is the `PlatformDescriptor` table in
`core/platforms.go`. `imbot/auth.go` holds only the settings-UI form fields
(labels, placeholders) and derives every intrinsic field from that table.
Per-platform connection detail is in each `platform/<name>/README.md`.

| Platform | Auth | Connection | README |
|---|---|---|---|
| Telegram | token | long-polling | [`telegram/`](platform/telegram/README.md) |
| Discord | token | WebSocket (Gateway) | [`discord/`](platform/discord/README.md) |
| Slack | token (+ optional app token) | RTM / Socket Mode | [`slack/`](platform/slack/README.md) |
| Feishu | oauth | WebSocket (event push) | [`feishu/`](platform/feishu/README.md) |
| Lark | oauth | WebSocket (event push) | [`lark/`](platform/lark/README.md) |
| DingTalk | oauth | Stream SDK | [`dingtalk/`](platform/dingtalk/README.md) |
| Weixin | token (+ account/user ids) | WebSocket | [`weixin/`](platform/weixin/README.md) |
| WeCom | oauth | WebSocket | [`wecom/`](platform/wecom/README.md) |
| WhatsApp | token | REST (Meta Cloud API) | [`whatsapp/`](platform/whatsapp/README.md) |
| Tingly (test) | none | in-process Transport | [`tingly/`](platform/tingly/README.md) |

## Quick start

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

	// OnMessage handlers receive (message, platform, botUUID).
	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform, botUUID string) {
		log.Printf("[%-10s] %s: %s", platform, msg.Sender.DisplayName, msg.GetText())

		bot := manager.GetBotByUUID(botUUID)
		if bot != nil {
			bot.SendText(context.Background(), msg.Recipient.ID, "Echo: "+msg.GetText())
		}
	})

	if err := manager.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	select {}
}
```

`Config`, `AuthConfig`, `Message`, `Platform`, and the content/error helpers are
re-exported by the `imbot` package and defined in `core/`. Add several bots at
once with `manager.AddBots([]*imbot.Config{...})`.

## Core concepts

**Bot lifecycle.** `manager.AddBot(cfg)` constructs the platform `core.Bot`
through the global registry, `Start(ctx)` connects every bot, and event
handlers (`OnMessage`, `OnError`, `OnConnected`, `OnDisconnected`, `OnReady`)
fan out across all of them. Look up a bot later with `GetBotByUUID(uuid)`.

**Inbound messages.** Every platform adapter converts its native events into a
`core.Message` with a typed `Content` (text / media / poll / reaction / system).
`msg.GetText()` returns the text; `msg.IsCallback()` reports whether the message
is a **button press** rather than typed input. Button presses carry their
identity as `msg.Payload` — an ordered list of segments (see `core/payload.go`)
— which replaces the old flat `callback_data` string.

**Outbound interactions.** Send with `bot.SendMessage(ctx, target, opts)`. The
neutral outbound vocabulary is `core.ActionSet` (`core/action.go`): each
platform renders it natively if it can, or falls back. Build buttons with
`imbot.NewKeyboardBuilder()` / `imbot.CallbackButton(...)`. `imbot.ParseCallbackData`
parses the historical flat encoding for producers that have not migrated to
`Payload`.

**Markdown.** Convert standard markdown to UTF-16-correct entities with
`markdown.Convert(...)` and attach them via `SendMessageOptions.Entities`
(precedence over `ParseMode`). See `markdown/README.md`.

**Platform-specific escape hatches.** Cast a `core.Bot` for native features:
`imbot.AsTelegramBot(bot)` (commands, menu button, chat resolution) and
`imbot.AsRestater(bot)` (replace a message's presentation).

## Security: pairing (TOFU)

Bot tokens are bearer credentials. Each bot can require a one-time pairing
handshake (trust-on-first-use) so a leaked token cannot grant command access.
The TOFU machinery itself (`PairingManager`: code minting, constant-time
compare, TTL, lockout, audit) lives in `core/pairing.go`; whether a given bot
*enforces* it is a server-side bot setting (not a field on `core.Config`),
resolving per-platform defaults from `core.PlatformBehavior.RequiresPairingByDefault`.

When a bot has pairing enabled:

1. The server prints a fresh pairing code to stderr on bot start, e.g.
   `[tingly-box] Bot "personal-tg" (telegram) pairing code: K7P2-QX9M`.
2. The operator sends `/bind K7P2-QX9M` from the bot's DM; the chat becomes
   the bot's owner. Codes are in-memory only, time-limited (default 10 min),
   single-use, compared in constant time, and lock the bot after 5 wrong
   attempts. Every outcome is audit-logged as `imbot.pair.*`.
3. Group chats still use the existing `/join <chatID>` whitelist, but the
   operator who whitelists a group must themselves be paired.

```bash
tingly-box bot pair enable|disable|status|revoke <bot-uuid> [<chat-id>]
```

Restart the bot to rotate the code. The CLI commands and tri-state default
resolution (`db.Settings.RequirePairing` → else platform default) are owned by
the server module, not by this package.

## Examples & tests

- [`examples/telegram/`](examples/telegram),
  [`examples/multi_platform/`](examples/multi_platform),
  [`examples/dingtalk/`](examples/dingtalk) — runnable bots.
- [`tests/telegram_e2e_test/`](tests/telegram_e2e_test) — Telegram E2E tests
  (`tests/telegram_e2e_test/TELEGRAM_E2E_TESTS.md`).

This framework is part of tingly-box; build and test with the project's
`task` targets (`task build`, `task go:test`).
