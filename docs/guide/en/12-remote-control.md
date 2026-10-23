# Remote

Paths: `/bots/*`, `/remote-agent`, `/notify` — the **IM** rail item

The **IM** rail item (it was called **Remote**) is where chat bots live: you connect messaging accounts, decide who in each chat may use Claude Code, and let automations push messages back into chats. It is split into three pages by concern: **Bots** (the accounts), **Remote Control** (who may control Claude Code from each bot, and under which profile) and **IM Notify** (outbound notifications).

**What a chat does.** Every IM chat talks to Claude Code **through [Projects](./22-projects.md)**: a message becomes a Claude Code session in a working folder, the session shows up in the Projects sidebar like any other, and approvals, queueing and replies work the same in the chat and on the web. Remote now only owns *who may come in* — bot connections, pairing, access control and notifications — while Projects owns what happens afterwards. The IM rail item therefore appears with the **Projects** switch (Power-ups → *Projects*): one switch turns on both. With it off the bots keep running and notifying, but a chat that asks Claude Code is told to turn Projects on (only `/bind` and `/join` still work).

> The old **@tb** (SmartGuide) path, `@cc` / `@tb` prefixes, the per-bot SmartGuide model, persistent-session switch, bash allowlist and bot-level verbose setting are gone. A leading `@cc` / `/cc` / `@tb` / `/tb` is simply dropped and the rest is sent as the message.

---

## Bots (`/bots/overview`)

![Bots Overview](../images/bots-overview.png)

The resource layer: connect and maintain the messaging accounts shared by Remote Control and IM Notify. Subtitle: *Connect and maintain the messaging accounts used by Remote Control and IM Notify.* The **Bots** row in the sidebar carries an `active/total` count (`2/3`).

Supported platforms: **Telegram, Feishu, Lark, DingTalk, Weixin (WeChat), WeCom** — filterable with the tile row (**All** plus one tile per platform, each with `active N / N`). QQ is not supported yet (its page, `/bots/qq`, says *coming soon*).

### Connections table

| Column | Description |
|--------|-------------|
| Status | Enable/disable switch + On/Off badge |
| Name | Bot alias |
| Bot UUID | Unique bot identifier (copy button) |
| Platform | Platform key (e.g. `telegram`) |
| Capabilities | **Remote** / **Notify** badges — which of the other two pages this bot takes part in |
| Actions | **Access** (see below), restore, edit, delete; a button shows the chats the bot has seen, so you can copy a Chat ID |

### Connecting a bot

Click **Connect a bot** (top right):

![Connect a Bot Dialog](../images/bots-connect-dialog.png)

1. Choose the **Platform**
2. Fill in the platform-specific credential (e.g. **Bot Token** for Telegram, from `@BotFather`)
3. Optional: **Alias** and **Proxy URL** (HTTP/HTTPS proxy for the bot's API requests)
4. Click **Connect bot**

Each platform also has a **Setup guide** (collapsed by default) with its steps and examples. Feishu and Lark offer **One-click (scan QR)** app creation (the app, permissions and events are created for you); Weixin uses **QR code** binding instead of a token.

### Access

**Access** on a bot row opens *`<bot>` access* — what the bot may do and for whom:

- **Capabilities**: Remote Control and Notify. Turning off the last one stops the connection without deleting its configuration
- **Direct Chats**: everyone who messaged the bot in a direct chat, with the concrete platform id. Each is **Paired person** / **Unpaired chat** and has a level — **Can control** (Remote Control), **Notify only**, **No access** or **Blocked**. An unpaired chat must pair first; to add someone, they send the pairing code shown to the bot in a direct message
- **Groups**: allow Remote Control per group, then add the people (actors) in it who may use it — group access and actors are separate controls; new groups start with no access

---

## Remote Control (`/remote-agent`)

![Remote Control](../images/remote-control.png)

One page, **one card per bot**. Subtitle: *Who can use Claude Code from each bot's chats, and the profile it runs under.* Platform is just the icon on each card; **Connect a bot** (top right) opens the same dialog as above.

### Per-bot card

- **Header**: platform icon + bot name, a one-line live status, and the **Remote Control switch** (off only unmounts this purpose — the bot and Notify keep running), plus **Edit** (pencil) and a ⋮ menu with **Restart Bot** and **Delete**
- **Status line**: `Remote Control off` (grey) · `Checking access…` · `Couldn't check who can control` / `Nobody can control yet` (warning) · `N chats can control` (green)
- **Route graph** (a card that is on): **Authorized access** (`N direct · N groups`) → the **platform** → **Agent · Claude Code** → **Profile**. The access node opens the Access editor above; the platform node edits the bot; the profile node opens the profile picker
- A card that is **on but unusable** shows what to do next right in the card — the pairing code to send the bot (*Nobody can control this bot yet. Send this to the bot in a direct message:*) or *N direct chat(s) reached this bot but can't control it. Grant it Remote Control if it's yours.* with **Manage access**
- An **off** card is quiet: greyed identity, one line (`Claude Code · Default`) and a **Settings** link instead of the full graph

### Claude Code profile

The **Profile** node (**Claude Code profile** dialog) picks which Claude Code configuration the bot's chats run under: *New sessions from this bot's chats run under this profile: its rules, model mapping and settings overrides.* **Default** is the main `claude_code` scenario; otherwise choose one of your [profiles](./03-scenario-claude-code.md#profiles) (create it on the Claude Code page first). The bot's profile applies to a chat's **first** session; later sessions keep the previous session's profile, model and permission mode, and fall back to the default if the profile was deleted.

> Old per-platform `/remote-agent/:platform` bookmarks and the pre-split `/remote-control/*` redirect here.

---

## Talking to Claude Code in a chat

Just send a message — the first one starts a session in the chat's working folder (the chat's earlier folder, or the bot's default directory). Replies, tool-call approvals and questions come back to the chat; permission approvals use buttons (or `y` / `n` / `always` as text on platforms without buttons), and a question from Claude Code is answered by replying with the option number or your own words. A message sent while a session is busy is queued, as on the web. Commands:

| Command | What it does |
|---------|--------------|
| `/cd <path>` | Work in another folder (the next message starts a new session there); the directory browser buttons do the same |
| `/dir [N]` | List the folders this chat has used, or switch to one |
| `/session [N]` | List the sessions in this place, or switch to `#N` |
| `/new [message]` | Start a new session |
| `/stop` | Stop the running session |
| `/clear` | Stop, and start a new session with the next message |
| `/status` | Where this chat is and how its session runs |
| `/verbose on\|off` | Also see each step, not only the reply |
| `/yolo` | Use every tool without asking, or switch back |
| `@N <message>` | Say one thing to session `#N` |
| `/bind <code>`, `/join` | Pairing and group allow-listing (the access layer; they work even when Projects is off) |
| `/help` | The list above, plus your user id and chat id |

Chats that are bound to a [project](./22-projects.md) rather than a folder talk to the project's coordinator instead — binding a chat to a project is done on the web (Projects → the project's **Bind chat**, or **Projects → IM**), because the project binding has exactly one entry point.

---

## IM Notify (`/notify`)

![IM Notify](../images/im-notify.png)

Same page shape: **one group per bot**. Subtitle: *Authorize a target, send through the production path, and see whether delivery worked.* It lets automations push messages into chats the bot has seen, independent of Remote Control. The **API guide** button (top right) documents the notification API: authenticate with your user token as a Bearer header, `POST /api/v1/bots/{bot}/notify` with the bot UUID (a `200` means delivered), and copy the target UUID from a chat node.

### Per-bot group

Each group has its own **Notify switch** and a **refresh** button, a status line (`N targets can receive` · `No target allowed yet` · `Notify off`) and a graph `POST /notify (API)` → platform → **chat nodes**. Each node pairs the platform id (e.g. `telegram:123456789`, tagged **Direct** or **Group**, and `paired`) with inline actions:

- **Notify** — send a test/manual message
- **Confirm** — shown as **Allow Notify & Test** for a target that hasn't been authorized yet
- **Custom** — send a custom payload (title, level, markdown body)
- copy (the internal target UUID the API needs), **Disable** (silently drop its messages; **Show disabled (N)** / **Hide disabled** reveals them) and delete (direct chats only; the pairing and bindings go, session history stays)

New targets need explicit authorization (**Allow Notify & Test**) before automated notifications reach them — a chat the bot merely happens to see can't silently become a target.

---

## Related Pages

- [Projects](./22-projects.md)
- [Experimental Features § Projects](./19-experimental.md#projects)
- [Claude Code § Profiles](./03-scenario-claude-code.md#profiles)
- [Scenario Overview](./02-scenario-overview.md)
