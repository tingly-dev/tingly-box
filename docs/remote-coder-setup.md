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

### Create a Telegram Bot

1. Open Telegram and start a chat with `@BotFather`.
2. Send `/newbot` and follow the prompts to name your bot.
3. Copy the bot token provided by BotFather.

### Bot Token Settings

1. Open the Credentials page.
2. Select the `bot token` tab.
3. Click **Edit** and configure:
   - **Platform**: `telegram`
   - **Telegram Bot Token**
   - **Proxy URL** 
   - **Chat ID Lock** (optional; only this chat can use the bot)
4. Click **Save Bot Token**.

Settings are stored in `tingly-remote-coder.db` in plain text.

### Start the Bot

The bot starts automatically when you run `tingly-box rc`, as long as a Telegram bot token is configured.

### Test Bot Connectivity (Optional)

If you use a SOCKS5 proxy, verify Telegram API connectivity:

```bash
curl --socks5-hostname 127.0.0.1:7897 -sS "https://api.telegram.org/botBOT_TOKEN/getMe"
```

Replace `BOT_TOKEN` with your bot token.

## 3. Get Telegram Chat ID (Optional)

Only required if you set **Chat ID Lock**.

1. Send a message to your bot.
2. Fetch the latest update from the Bot API and copy `chat.id` from the response.

Example (replace `BOT_TOKEN`):
```bash
curl "https://api.telegram.org/botBOT_TOKEN/getUpdates"
```

Refer to Telegram Bot API docs for `getUpdates` and the `chat` object if you need more details.

## 4. Use Remote Coder GUI (Optional)

You can use either the GUI or the Telegram bot commands. Both control the same remote-coder sessions.

1. Open the Remote Coder GUI page in the sidebar.
2. Set a **Project Path** when prompted (required for Claude Code context).
3. Select a session or start a new one.
4. Send messages and view responses.
5. Manage sessions from the **Manage Sessions** button.

## 5. Telegram Bot Commands (Optional)

You can use either the GUI or the Telegram bot commands. Both control the same remote-coder sessions.

- `/info` - Show current session ID, project path, and last assistant summary.
- `/list` - List all sessions with ID, status, project path, and last assistant summary.
- `/use <session_id>` - Switch this chat to a specific session.
- `/new <project_path>` - Create a new session and set its project path (required).
- `/bash <command>` - Run allowlisted shell commands (`pwd`, `ls`, `cd <path>`) without requiring a session.

## 6. Quick Checklist

- `tingly-box rc` is running
- Bot token saved in GUI
- Chat ID lock set (optional)
