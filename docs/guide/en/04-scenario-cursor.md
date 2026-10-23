# Cursor Scenario

Path: `/agent/cursor`

Cursor is the one agent hidden from the sidebar by default (unhide it in the sidebar's [edit mode](./02-scenario-overview.md#managing-visibility-sidebar-edit-mode) if you use it). The scenario proxies Cursor's OpenAI-compatible API requests to your configured providers, with **Cursor compatibility handling** (`cursor_compat`) enabled by default on its built-in rule.

---

![Cursor Scenario](../images/cursor-scenario.png)

## Why Cursor needs its own page

Unlike every other coding-agent scenario, Cursor calls its configured "Override OpenAI Base URL" from **Cursor's own cloud backend** (`api2.cursor.sh`) — never from the Cursor app running on your machine. That means:

- A `localhost` or private-network Base URL (`10.x`, `192.168.x`, `172.16–31.x`) **will not work** — Cursor's cloud servers can't reach it
- The Base URL must be a **publicly reachable HTTPS** address — expose this server with a Cloudflare Tunnel, ngrok, or a public deployment first

## Page Structure

The standard agent page — see [Claude Code § Page Structure](./03-scenario-claude-code.md#page-structure) for the status line, header buttons, connection card, Plugins card and Model Rules. Cursor's differences:

- The connection card has **Base URL** and **API Key** rows (copy / reveal).
- The header's key action is **Setup Guide** (filled, rightmost), not Auto Config: Cursor has no local settings file Tingly-Box can write to, so setup is a manual copy-paste into Cursor's own settings UI. There is no Quick Start card.
- Model Rules are free-form (**New Model**, **Test All**, **Connect AI**), and the built-in rule carries `cursor_compat` (below).

### Setup Guide (Configure Cursor)

![Configure Cursor Modal](../images/cursor-config-modal.png)

- An **alert** (warning if the Base URL looks unreachable from Cursor's cloud — local/private host or non-HTTPS; info otherwise) reiterates the cloud-reachability requirement
- Numbered steps: open **Cursor → Settings → Models**, enable **Override OpenAI Base URL** under **OpenAI API Key**, enter the Base URL (`<Base URL>/tingly/cursor`) and API Key shown, then click **Verify** in Cursor to save
- **Copy URL** / **Copy API Key** buttons

---

## The `cursor_compat` Flag

The built-in Cursor rule ships with the `cursor_compat` flag **on by default**, which normalizes rich content, gates tool definitions, and strips stream usage blocks for Cursor's client quirks. A separate `cursor_compat_auto` flag (off by default) can instead auto-detect Cursor via request headers and apply the same handling to rules used by other clients too — see [Routing Rules & Plugins § App](./20-routing-rules.md#app).

---

## Configuration Flow

1. Make sure this Tingly-Box instance is reachable over the public internet via HTTPS (tunnel or public deployment)
2. Add at least one provider in [Credentials](./08-credentials.md)
3. Unhide Cursor in the sidebar's edit mode if needed, then open `/agent/cursor`
4. Click **Setup Guide** and follow the dialog's steps inside Cursor's own Settings → Models panel
5. Click **Verify** in Cursor to confirm the connection

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Claude Code Scenario](./03-scenario-claude-code.md)
- [Routing Rules & Plugins](./20-routing-rules.md)
- [Credentials](./08-credentials.md)
