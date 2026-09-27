# Remote

Paths: `/bots/*`, `/remote-agent/:platform`, `/notify` (Full Edition)

The **Remote** group lets you control Claude Code (and other agents) from mainstream IM platforms. It's split into three pages by concern: **Bots** (connect the messaging accounts), **Remote Control** (route incoming chat commands to an agent), and **IM Notify** (push outbound notifications back to chats).

> **Note**: Remote is available in **Full Edition** only.

---

## Bots (`/bots/overview`, `/bots/:platform`)

![Bots Overview](../images/bots-overview.png)

The resource layer: connect and maintain the messaging accounts shared by Remote Control and IM Notify. Page subtitle: *"Connect and maintain the messaging accounts used by Remote Control and IM Notify."*

Supported platforms: Telegram, Feishu, Lark, DingTalk, Weixin (WeChat), WeCom, QQ, Discord, Slack — filterable via the platform tab row (**All** plus one tab per platform, each showing an `active N / total N` count).

### Connections Table

| Column | Description |
|--------|-------------|
| Status | Enable/disable toggle + On/Off badge |
| Name | Bot alias |
| Bot UUID | Unique bot identifier (copy button) |
| Platform | Platform key (e.g. `telegram`) |
| Capabilities | **Remote** / **Notify** badges — which of the other two pages this bot participates in |
| Actions | **Access** (chat-ID / group authorization), restore, edit, delete |

### Connecting a Bot

Click **Connect a bot** (top-right):

![Connect a Bot Dialog](../images/bots-connect-dialog.png)

1. Choose the **Platform** from the dropdown
2. Fill in the platform-specific credential (e.g. **Bot Token** for Telegram, obtained from `@BotFather`)
3. Optional: **Alias** (friendly name) and **Proxy URL** (HTTP/HTTPS proxy for the bot's API requests)
4. Click **Connect bot**

Each platform tab also shows a collapsible **Setup Guide** with connection steps, credentials, and examples specific to that platform.

> WeChat (Weixin) bots use **QR code scanning** instead of a token — the dialog shows a QR code to scan after starting the connection.

---

## Remote Control (`/remote-agent/:platform`)

![Remote Control](../images/remote-control.png)

The routing layer: *"Choose who can control each bot and where chat commands route."* For each bot, define the chain: **Access → Bot → Agent**.

- **Access**: the authorization box — direct chat IDs and/or groups allowed to issue commands (`0 direct`, `0 groups` when unrestricted)
- **Bot**: which connected bot (from the Bots page) this route applies to
- **Agent**: which scenario the bot's commands are routed to, and which model/profile handles them

Click any node in the route diagram to edit that part. A **Setup guide** button (top-right) walks through first-time configuration.

> Old `/remote-control/*` bookmarks redirect here automatically.

---

## IM Notify (`/notify`)

![IM Notify](../images/im-notify.png)

The outbound-notification layer: *"Authorize a target, send through the production path, and see whether delivery worked."* Lets scenarios and automations push messages back into chats/groups the bot has seen, without touching Remote Control's inbound routing.

### Delivery Targets

Grouped by bot; each target row shows:
- The chat/group identifier (e.g. `telegram:123456789`) and whether it's a **Direct** chat or a **Group**
- Pairing status (`paired`)
- Actions: **Notify** (send a test/manual message), **Confirm** (for unconfirmed targets, shown as **Allow Notify & Test**), **Custom** (custom payload), copy, revoke, delete

New targets require explicit authorization (**Allow Notify & Test**) before they can receive automated notifications — this prevents any chat the bot happens to observe from silently becoming a notification target.

An **API guide** button (top-right) documents the notification API for scripted/automation use.

---

## Bot Security Settings

### Access Authorization

From the Bots page, click **Access** on a bot row to restrict which chat IDs or groups may issue commands to it — the same authorization the Remote Control route diagram's **Access** node edits.

### Bash Allowlist

Configured per bot-to-agent route: one command pattern per line, limiting which shell commands the bot can trigger. Commands not in the allowlist are rejected. Example:

```
ls
cat *.md
git status
git diff
```

---

## Usage

Once a bot is connected (Bots) and routed to an agent (Remote Control), send messages to it on the IM platform:

- Send a code request → the bot calls the routed agent (e.g. Claude Code) to execute it
- Query status → the bot returns the current run status
- Send a file → the bot processes the file in the working directory

Outbound updates (long-running task completion, alerts, etc.) are delivered via IM Notify to any authorized target.

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [System Settings](./17-system-settings.md)
