# Usage Dashboard

Paths: `/dashboard/:timeRange` (default `/dashboard/today` — the **Dashboard** rail item opens here) · `/dashboard/users` (**Team usage**) · `/dashboard/quota-history` (**Quota history**)

![Usage Dashboard](../images/dashboard.png)

The Dashboard rail item has three pages in its sidebar: **Usage**, **Team usage** and **Quota history**. There is no page title — the sidebar already says where you are — and every page starts with one bordered **toolbar**.

---

## Usage

Statistics and visualisations of AI request activity: call volume, token consumption, cache hit rate, response performance, by agent, provider and model.

### Toolbar

Left: four filters, right: the range and refresh controls.

| Control | What it does |
|---------|--------------|
| **Agent** | `All agents`, or one agent (Claude Code, Codex, a Claude Code profile, a Team, …). An agent page's **Usage** button opens the Dashboard already filtered to that agent (`?scenario=<agent>`) |
| **Provider** | Providers grouped by auth type (OAuth / API Key / Bearer Token / Basic Auth / Virtual Model), each with its logo |
| **Model** | Models that have data in the current range, sorted by token usage |
| **Identity** | `All identities`, `Main account` (the global Model Token), or an individual **Team Key** (grouped under its Team) |
| *clear* | A **Clear all filters** button appears when any filter is set |
| **Range** | A dropdown with a clock icon — **Today**, **Yesterday**, **3D**, **7D**, **30D**, **90D**. Each range keeps its own URL (`/dashboard/7d`, …); the range used to be six rows in the sidebar |
| **Auto** | Auto-refresh toggle (every minute) |
| refresh | Manual refresh |

![Range menu](../images/dashboard-range-menu.png)

### Stat cards

Five cards for the selected range. The two that are a health judgement carry a **status dot** (green / amber / red); volume cards stay neutral.

| Metric | Description |
|--------|-------------|
| **Total Requests** | Number of requests |
| **Total Tokens** | Total tokens (subtitle: `Input: X + Cache: Y` / `Output: Z`) |
| **Cache Hit Rate** ● | Percentage (status dot: green ≥ 50 %, amber ≥ 20 %, red below); subtitle splits cache into **read** vs **written** (e.g. `40M read · 4.8M written`) |
| **Error Rate** ● | Request failure rate, with the error count |
| **Streamed Rate** | Share of streaming responses |

### Tabs: Summary · By Request · Activity

The tabs sit at the top of the panel under the cards:

- **Summary** — **Response Performance** on the left and the **Token Usage Over Time** chart on the right. Today/Yesterday show a **5-minute** curve (Cache Read / Input / Output); longer ranges show daily bars.

  | Column | Description |
  |--------|-------------|
  | TTFT | Time to first token |
  | TPS | Tokens per second |
  | Latency | Total request latency |

  Rows are percentiles — **P99, P95, P90, P50, P10** — each with its sample count (`n=…`); a dash means that percentile has no data (e.g. TPS for a request that errored before streaming).

- **By Request** — only for **Today** / **Yesterday**: `Requests · N total`, one row per request — **Time**, **Model** (with the provider's logo), **Scenario**, **Cache Read**, **Cache Write**, **Input**, **Output**, **Reasoning**, **Latency**, **TTFT**, **TPS** — with an **All / Success / Error** filter and pagination (50 rows per page)

  ![By Request](../images/dashboard-by-request.png)

- **Activity** — a GitHub-style contribution heatmap, below.

### Usage by Model table

At the bottom (paginated), with each provider's logo beside its name:

| Column | Description |
|--------|-------------|
| Provider | Provider name |
| Model | Model name |
| Requests | Request count |
| Cache Read / Cache Write / Cache Hit | Cache tokens and hit rate |
| Input Tokens / Output Tokens | Token counts |
| Reasoning Tokens | Reasoning/thinking tokens (0 or blank when the model doesn't report them) |
| Error Rate | Failure rate (%) |

### Activity heatmap

![Activity Heatmap](../images/dashboard-activity.png)

The **Activity** tab shows a GitHub-style contribution heatmap inside the Dashboard, sharing the Agent / Provider / Model / Identity filters.

- **Fixed window**: always the **last 365 days**, regardless of the range (the range only affects Summary and By Request)
- **Grid**: months across, day of week down; darker cell = more tokens that day
- **Bottom stats**: total tokens, active days / total days, longest streak, maximum single-day usage

---

## Team Usage (`/dashboard/users`)

![Team Usage](../images/dashboard-team-usage.png)

Breaks usage down **per registered account** instead of per model/provider — useful when several people share one instance through the shared Model Token and [Team Keys](./07-team.md). One toolbar, with a **Today / 7D / 30D / 90D** switch and a refresh button.

### Stat cards

The first card follows the view below; the rest are the same everywhere:

| Card | Description |
|------|-------------|
| **Registered users** / **Models used** / **Providers used** | By account / by model / by provider; subtitle: active accounts in the period, or how many providers/models it spans |
| **Total tokens** | Sum; subtitle breaks down Cache / Input / Output |
| **Cache hit rate** ● | With a status dot; subtitle shows cache read vs written |
| **Requests** | Total; subtitle shows the average per active user |
| **Errors** ● | Error count and rate, status dot on health |

### By account / By model / By provider

A three-way toggle above the table switches its grouping (the cards follow it). A search box filters by name.

**By account** (default) — one row per account:

| Column | Description |
|--------|-------------|
| User | Name, with a **Primary** badge for the account tied to the global Model Token or a **Disabled** badge; subtitle: last-used time or `Never used` |
| Requests | Request count |
| Total | Total tokens (default sort) |
| Cache Read / Cache Write / Cache Hit | Cache breakdown per account |
| Input / Output / Reasoning | Token counts |
| Error Rate | Per-account failure rate |

**By model** / **By provider** pivot the table to one row per model or provider. Selecting a row opens a detail panel below listing the accounts using it, subtitled with how many providers/models each touches.

Beside the table, a **Top list** (ranked share bars) shows **Top accounts**, **Top models** or **Top providers** for the active view — name and percentage share, the rest grouped as **Others**.

![Team Usage — By Model](../images/dashboard-team-usage-by-model.png)

---

## Quota History (`/dashboard/quota-history`)

![Quota history](../images/quota-history.png)

How each provider's **quota** has moved over time — the snapshots taken whenever a quota is refreshed (see [Credentials](./08-credentials.md#quota-column)). The same toolbar shape: a **Provider** filter, a range switch (**5H / 1D / 7D / 30D**) and refresh.

One card per provider (logo and name, `N samples`, and a **Details** link to the latest raw quota response), holding one line chart per quota window — e.g. *5-Hour Window* and *7-Day Window* for Claude Code OAuth, with the latest `38% left` beside the title. Balances (money or credits) get their own axis. A provider whose latest refresh failed shows the error instead; with nothing recorded yet the page says *No quota snapshots in this period* (snapshots appear after a supported provider's quota is refreshed). The page shows the latest 1,000 points — pick one provider or a shorter range to see more.

---

## Related Pages

- [System Settings](./17-system-settings.md)
- [Credentials](./08-credentials.md)
- [Team](./07-team.md)
