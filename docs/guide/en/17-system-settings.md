# System Settings

Paths: `/system`, `/system/logs`

![System Settings](../images/system.png)

The System Settings page provides global preference configuration, server status monitoring, proxy settings, language/theme switching, and log viewing.

---

## System Settings Main Page (`/system`)

The General tab is organized into four cards:

### Server Status Card

| Field | Description |
|-------|-------------|
| Server | Running / Stopped / Unavailable, plus a Connected/Disconnected indicator |
| Uptime | How long the server has been running |
| Version | Current version number, with a copy button — moved here from the About card |

**Actions** (top-right icons): **Force Logout** (force-exit the current web session — clears token, returns to login page) and **Refresh Status**.

When a newer version is available, clicking the version notice opens the **Update** dialog: a channel toggle (`npx` / `npm` / `bundle` / `docker`) switches between install methods, each showing its command with a one-click copy —

| Channel | Command | Use case |
|---------|---------|----------|
| `npx` (default) | `npx tingly-box@<version>` | Quick update — downloads and restarts in one command |
| `npm` | `npm install -g tingly-box@<version>` | Updates a globally-installed CLI; restart the server afterward to apply |
| `bundle` | `npx -y tingly-box-bundle@<version>` | Offline bundle with the binary embedded, for flaky networks |
| `docker` | — | Pull the new image from GitHub Container Registry |

---

### Appearance & Language Card

- **Language**: `English` / `中文` / `Русский`. Falls back to the browser's language on first load when no explicit choice has been saved yet.
- **Theme**: `Light` / `Dark` / `Sunlit` / `Claude` / `System` (follows OS setting)

---

### Proxy Settings Card

Combines both proxy controls in one card:

- **Respect Environment Proxy**: when enabled, providers without an explicit proxy configured fall back to the system's environment proxy settings (`HTTP_PROXY`, `HTTPS_PROXY`, macOS system proxy, Clash, etc.)
- **Quick Proxy**: a reusable HTTP/HTTPS proxy preset that providers and OAuth can pick up with one click — enter the address (e.g. `http://127.0.0.1:7890`) and click **Save**; a per-provider proxy still takes priority if set

> To configure a proxy for a specific provider only, use the Proxy URL field in the provider edit form in [Credentials](./08-credentials.md).

---

### About Card

- **License**: MPL-2.0 + Commercial
- **GitHub**: Project repository link

> Version info and its update notice now live on the Server Status card, not here.

---

## Logs Page (`/system/logs`)

Path: `/system/logs`

![Logs Page](../images/logs.png)

View real-time Tingly-Box server logs.

### Features

**Debug Mode toggle** (top-right):
- On: Log level switches to `debug` — more detailed output
- Off: Log level is `info` (default)

**LogExplorer area:**
- Real-time streaming server logs
- Scrollable history
- Each log entry includes: timestamp, level, source module, message

**Per-request journey**: expanding a request's log entry shows its full journey as one time-ordered list — trace spans and plain log lines are no longer two separate views. Every row follows the same grammar (`[kind] name · detail → result · duration`), whether it's a measured span or a log line; a **kind badge** (`stage` vs `log`) is the only thing distinguishing them, and clicking any row opens a key/value detail panel.

---

## Related Pages

- [Access Control](./18-access-control.md)
- [Experimental Features](./19-experimental.md)
- [Credentials](./08-credentials.md)
