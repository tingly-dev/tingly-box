# System Settings

Paths: **General** `/system`, **Access Control** `/access-control`, **Experimental** `/system/experimental`, **Develop** `/system/develop`, **Logs** `/system/logs`

![System Settings](../images/system.png)

System has **no icon of its own in the rail**. Open it from the **gear at the foot of the rail**: *Preferences* → **All settings** (the gear lights up while you are on a System page). Its sidebar lists **General**, **Access Control** ([own chapter](./18-access-control.md)), **Experimental** ([own chapter](./19-experimental.md)), **Develop** and **Logs**.

The gear's **Preferences** popover itself carries the quick controls — it is the same language and theme switch as the General page, plus **Power-ups** and **Feedback**:

![Preferences menu](../images/preferences-menu.png)

- header: `Tingly-Box <version>` and the tagline
- **Language** — `English` · `中文` · `Русский`
- **Theme** — Light · Dark · Sunlit · Claude · DeepSeek · System
- **Power-ups** (hover or click) — the [Power-ups submenu](./02-scenario-overview.md#power-ups-the-menu-behind-the-gear)
- **All settings** — opens System
- **Feedback** — opens the GitHub issue chooser

---

## General (`/system`)

Four cards:

### Server Status

| Field | Description |
|-------|-------------|
| Server | Running / Stopped / Unavailable, plus a Connected/Disconnected indicator |
| Uptime | How long the server has been running |
| Version | Current version, with a copy button; an `available → <version>` note appears when an update exists |

Copy buttons throughout the UI also work when Tingly-Box is opened over plain HTTP (they fall back to a clipboard shim, even inside dialogs).

**Actions** (right-hand icons): **Force logout** (clears the token and returns to the login page) and **Refresh status**.

Clicking the version — here, or the **version row at the foot of every sidebar** (the version alone, with a small status badge: green check = up to date, orange arrow = update available) — opens the **Update** dialog. A channel toggle (`npm` / `npx` / `docker`) switches install methods, each with a one-click copy. It opens pre-selected on the channel the running instance was launched from when that can be detected (npm/npx), otherwise `npm`:

| Channel | Command(s) | Use case |
|---------|---------|----------|
| `npm` (default, recommended) | `npm install -g tingly-box@<version>` then `tb restart` | Updates the globally installed CLI and restarts it with the short `tb` alias |
| `npx` | `npx tingly-box@<version>` | One-off update — downloads and restarts in one command |
| `docker` | `docker pull ghcr.io/tingly-dev/tingly-box:v<version>` | Pull the new image from GitHub Container Registry |

### Appearance & Language

- **Language**: `English` / `中文` / `Русский` (Russian was added). On the first visit with no saved choice, the UI follows your browser's language.
- **Theme**: `Light` / `Dark` / `Sunlit` / `Claude` / `DeepSeek` / `System` (follows the OS)
- **Palette preview**: swatches of the current theme's colours (hover for names and values)

### Proxy Settings

- **Respect Environment Proxy**: providers without an explicit proxy fall back to the system's environment proxy (`HTTP_PROXY`, `HTTPS_PROXY`, macOS system proxy, Clash, …)
- **Quick Proxy**: a reusable HTTP/HTTPS proxy preset (e.g. `http://127.0.0.1:7890`) that providers and OAuth can pick with one click; a per-provider proxy still takes priority. Enter it and click **Save**

> A proxy for one provider only is set in that provider's edit form — see [Credentials](./08-credentials.md).

### About

- **License**: MPL-2.0 + Commercial
- **GitHub**: the project repository

> The desktop app shows exactly the same pages — the former *lite* edition, which hid some of them, no longer exists.

---

## Develop (`/system/develop`)

![Develop](../images/develop.png)

Developer utilities for verifying frontend behaviour by hand — currently a **Notification playground**: *Success / Info / Warning* toasts auto-dismiss, while *Error* toasts stay on screen until closed and carry a copy button for bug reports (even when a duration is passed).

---

## Logs (`/system/logs`)

![Logs Page](../images/logs.png)

Server logs in two tabs, with a **Debug Mode** switch at the top right (on = log level `debug`, off = `info`, the default).

**Requests** — one row per AI request: **Time**, **Scenario**, **Model** (`requested → routed`), **Provider**, **Status** (coloured) and **Latency**; failed requests carry a red marker. Controls: **Auto-refresh**, a refresh button, `N shown · M recent requests`, **Search loaded requests**, a **With errors** chip and **Load** (200 / 500 / 1000). An agent page's **Log** button opens this same viewer in a dialog, filtered to that agent (a `filter: <scenario>` chip you can clear). Expand a row for its full **journey** as one time-ordered list: routing → conversion → upstream, each row `[kind] name · detail → result · duration` with a `stage` / `log` badge — click a row for a key/value panel; the request and trace ids sit at the bottom for bug reports.

**System** — the server's own log: **Time**, **Level**, **Status**, **Message**, with level chips (**All**, then per level), search, **Load**, auto-refresh and sortable columns.

---

## Related Pages

- [Access Control](./18-access-control.md)
- [Experimental Features](./19-experimental.md)
- [Credentials](./08-credentials.md)
