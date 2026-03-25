# Telegram Markdown E2E Test

Simple test to verify Telegram markdown rendering.

## Setup

```bash
export TELEGRAM_BOT_TOKEN="your_bot_token"
export TELEGRAM_TEST_CHAT_ID="your_chat_id"
```

## Run

```bash
go test -tags=e2e -v -run TestE2E_TelegramMarkdown ./imbot/tests/
```

## Verify

Check your Telegram chat - markdown should be **rendered** (not raw text like `**bold**`).

Expected:
- **bold text** (formatted)
- _italic text_ (formatted)
- `code` (monospace)
- Code blocks with syntax highlighting
- Special chars displayed correctly

If you see raw markdown like `**bold**`, the bug still exists.
