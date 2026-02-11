# Remote Coder Setup

This guide explains how to run Remote Coder, configure the Telegram bot, and use the Remote Coder GUI.

## 1. Run Remote Coder Service

Remote Coder runs as a subcommand of `tingly-box` and starts the Telegram bot automatically if configured.

```bash
./build/tingly-box rc
```

By default it listens on port `18080`. You can override this with flags:

```bash
./build/tingly-box rc --port 18080
```

## 2. Configure Telegram Bot

Remote Coder can proxy chats from Telegram. The bot settings are managed in the Remote Coder GUI.

### Bot Token and Allowlist

1. Open the Remote Coder GUI page: `Remote Coder Chat`.
2. In the **Telegram Bot** panel:
   - Paste your Telegram bot token.
   - Add allowlisted chat IDs (one per line).
   - Click **Save Bot Settings**.

Settings are stored in `tingly-remote-coder.db` in plain text.

### Start the Bot

The bot starts automatically when you run `tingly-box rc`, as long as a Telegram bot token is configured.

## 3. Get Telegram Chat ID

You must allowlist the chat ID before the bot will respond.

Options:

- **Use @RawDataBot**:
  1. In Telegram, search for `@RawDataBot` and start it.
  2. Send any message.
  3. The bot replies with JSON; copy the `chat.id` value.

- **Group chat ID**:
  - Add `@RawDataBot` to the group and send a message there. Use the returned `chat.id`.

## 4. Use Remote Coder GUI

1. Open the Remote Coder GUI page in the sidebar.
2. Set a **Project Path** when prompted (required for Claude Code context).
3. Select a session or start a new one.
4. Send messages and view responses.
5. Manage sessions from the **Manage Sessions** button.

## 5. Quick Checklist

- `tingly-box rc` is running
- Bot token saved in GUI
- Chat ID allowlisted
- `tingly-box rc bot` is running
