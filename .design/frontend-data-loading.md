# Frontend data loading: what a page costs, and what was trimmed

How much the web UI downloads (JS) and asks the backend (API requests) when a
page opens, the numbers before and after the first pass of trimming, and the
rules that keep it from creeping back. No API shape was changed; the backend
dashboard-aggregation work is independent of this.

## How it is measured

`frontend/scripts/measure-data-loading.mjs` drives headless Chrome through a
**mock-mode production build** (MSW answers), so request *counts* are exactly
what the frontend sends; payload *sizes* are the mock's (the real backend's are
larger — see "Payloads"). The header of the script has the commands. Bundle
sizes come from `pnpm build` and the `<link rel="modulepreload">` list in
`dist/index.html` (that list is what ships on every first paint).

## Result (first pass)

Eager JS + CSS (everything `index.html` loads before any page code), minified:

| | files | raw | gzip |
|---|---|---|---|
| before | 36 | 2,187 KB | 636 KB |
| after | 36 | 1,330 KB | 411 KB |

(−39% raw, −35% gzip.) What moved out of the eager set: the `openai` SDK
(`api` chunk 388 KB → 25 KB), the zh/ru locale bundles (`index` chunk 874 KB →
483 KB), the syntax highlighter behind the quota "Details" dialog (85 KB).

API requests on a cold load (`load` phase of the script):

| page | before | after |
|---|---|---|
| `/dashboard/today` | 23 | 22 |
| `/dashboard/users` | 23 | 21 (+1 when switching axis) |
| `/dashboard/quota-history` | 19 | 18 |
| `/agent/claude_code` | 43 | 30 |
| `/credentials` | 25 | 19 |

Dashboard, "By Request" tab, two visits: 4 requests / 169 KB → 4 / 115 KB on
the mock (120 records); a real range with >500 records used to send a
500-row sample *and* then page one again, now one 50-row page.

Idle (95 s, tab visible): unchanged — `info/health` every 30 s, `projects`
every 30 s (the rail's waiting dot), plus page-specific 15–30 s pollers that
were already visibility-gated.

## What changed and why

- **`openai` SDK loaded on first use** (`services/modelApi.ts`). It is only
  needed to build an image/prompt client, but `api.ts` imports `modelApi` for
  the model-list calls, so the whole SDK rode in the eager `api` chunk.
- **Non-English locales load on demand** (`i18n/index.ts`). `en` stays bundled
  as the fallback; `zh`/`ru` come through an inline i18next backend module.
  `main.tsx` waits for `i18nReady` so the first paint is translated, and
  `changeLanguage()` resolves after the load, so there is no flash of keys.
  Only the `translation` namespace is fetched; other namespaces (Projects adds
  its own with `addResourceBundle`) are untouched.
- **Quota raw-response dialog is lazy** (`LazyQuotaRawResponseDialog`). It is
  reached from quota cells in the always-loaded layout but pulls the syntax
  highlighter; it now loads on first open and stays mounted afterwards.
- **Concurrent identical GETs share one request** (`services/coalescedFetch.ts`,
  wired into the generated client and `fetchUIAPI`). Several components each
  fetch the same list on mount (`/config` was sent 7× on `/credentials`).
  Entries live only while the request is in flight; any mutation clears them;
  callers get their own `Response` clone; a caller's abort does not cancel the
  shared request.
- **A few repeated reads are reused for 5 s** (`services/readCache.ts`:
  providers, the model token, a scenario's config and string flags). In-flight
  sharing cannot catch asks that are a moment apart (the dashboard filter bar
  and its tables both read providers). Entries are dropped when any mutation
  goes through the API client, callers get a `structuredClone`, and
  `getProviders({ fresh: true })` bypasses it (the dashboard's refresh button).
  Only a change made *elsewhere* (another tab, the CLI) can show up ≤5 s late.
- **"By Request" asks the server for the page on screen**
  (`RequestsView`, `useDashboardData`). It used to fetch a 500-row sample and
  page it locally; with >500 records it then fetched page one again, so the
  sample was pure waste. Filtering by status is now a request instead of a
  local filter.
- **Team usage loads only the visible axis's detail**
  (`useRosterAxis({ enabled })`). Three detail queries ran on load for one
  visible table. A detail already loaded for the same row/range is kept when
  an axis is shown again; a freshly shown axis reports "loading" from its
  first render.
- **Health check skips hidden tabs** and re-checks the moment the tab is
  visible again (`HealthContext`).

## Rules going forward

- Call the API through `api.*` / `controlApi` / `fetchUIAPI` — that is what
  makes duplicate GETs fold together. A raw `fetch` to `/api/v1` does not
  (and does not invalidate the read cache after a write).
- Do **not** add a static `import` of `openai`, `recharts`, `three`, a locale
  bundle or other large libraries to code reachable from `Layout`/`App`;
  check `dist/index.html`'s modulepreload list after `pnpm build`.
- Reads worth putting behind `readCached` are cheap-to-be-stale, widely
  repeated, and invalidated by a mutation that goes through the client. Do not
  use it for anything polled faster than 5 s or for something another writer
  (server side effect, OAuth callback) changes behind the UI's back.
- A new poller: gate on `document.visibilityState`, pick ≥15 s unless a user is
  actively waiting.

## Not done (candidates, need an API or product decision)

- Rail "waiting" dot reads the whole project list (`GET /api/v1/projects`,
  3–10 KB) every 30 s; a count-only endpoint would make it ~100 B.
- Five separate `GET /scenario/_global/flag/<name>` calls at startup; a batch
  read would make it one request. Same for `info/version/check`, `mcp/config`,
  `teams`, `scenario-descriptors` which every page load sends once.
- `/api/v2/providers` returns full provider records (with model lists) to
  callers that need `uuid/name/auth_type`; a slim variant would shrink the
  largest repeated payload (15.6 KB in the mock, more in real configs).
- Dashboard "Activity" tab: the 12-month heatmap pulls a per-day timeseries
  with every token field (76 KB in the mock) for a grid that needs only totals.
- `BrandIcons` (138 KB) is eager because the rail/provider UI uses it; it could
  be split per brand.
- The Projects chunk (`ProjectsRoute`, 425 KB) and the image playground
  (`three`, 692 KB) are already route-lazy; no action.
