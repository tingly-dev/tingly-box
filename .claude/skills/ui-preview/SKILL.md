---
name: ui-preview
description: Capture headless-Chrome screenshots of the tingly-box frontend (running locally in mock mode) so frontend changes can be visually verified in environments without a real browser. Use when the user asks to "preview", "screenshot", "see the page", "show me the UI", "verify visually", or when frontend layout / component / styling changes need a sanity-check before review. Works in restricted/cloud sandboxes where Playwright's normal Chromium install is blocked.
---

# Headless UI preview (Playwright + Chrome for Testing)

Use this when you need a screenshot of the running frontend. Designed for the
remote-execution container where:
- MCP servers cannot be registered mid-session (so `playwright-mcp` / `chrome-mcp` won't work — use Playwright directly).
- `cdn.playwright.dev` and `dl.google.com` are blocked by network policy.
- `storage.googleapis.com` (Chrome for Testing) is reachable.
- Ubuntu's `chromium-browser` is a snap stub and won't run in the container.

## Setup (run once per fresh container)

Run these from `frontend/`:

```bash
# 1. Install Playwright (no browsers yet)
npm i -D playwright

# 2. Download Chrome for Testing directly — bypasses playwright.dev CDN
#    Match the version Playwright wants if possible (run `npx playwright install chromium`
#    once first to see the version in the error message), otherwise any recent
#    chrome-for-testing build works fine.
mkdir -p /tmp/chrome && cd /tmp/chrome
curl -fsSL -o chrome.zip \
  "https://storage.googleapis.com/chrome-for-testing-public/148.0.7778.96/linux64/chrome-linux64.zip"
unzip -q chrome.zip
# Resulting binary: /tmp/chrome/chrome-linux64/chrome

# 3. Install MUI's emotion peer deps if missing (they're not in package.json
#    but vite/MUI need them at runtime)
cd <repo>/frontend
npm i @emotion/react @emotion/styled
```

These packages are **tooling-only** — do NOT commit `package.json`,
`package-lock.json`, or the screenshot script. Revert them with
`git checkout -- frontend/package.json && rm -f frontend/package-lock.json
frontend/screenshot.mjs` before ending the session.

## Run the dev server

```bash
cd frontend
USE_MOCK=true npm run dev:mock   # vite picks .env.mock; runs on :3000
```

Two gotchas:
- `npm run dev` listens on `:9245`, but `dev:mock` defaults to `:3000`.
- `USE_MOCK=true` must be set as a **shell env var** (vite.config.ts reads
  `process.env.USE_MOCK` before `.env.mock` is applied). Without it you'll
  get 502s and `use mock false` in the dev-server logs.

Wait until the page is reachable:

```bash
until curl -fs http://localhost:3000 >/dev/null; do sleep 1; done
```

## Take the screenshot

Drop `screenshot.mjs` (template provided alongside this SKILL.md) into
`frontend/` and run from `frontend/` so node resolves `playwright` from
`node_modules`:

```bash
node screenshot.mjs
```

The template:
- Launches the downloaded Chrome with `--no-sandbox --disable-dev-shm-usage`.
- Seeds `localStorage.user_auth_token` via `addInitScript` so the app
  skips the login screen (without this, every route lands on the login page).
- Logs page errors / warnings to stdout for debugging blank-page issues.
- Waits for `<nav>` + a 2.5s settle to let MUI finish painting.
- Captures `/tmp/agent-full.png` (full viewport) and `/tmp/agent-sidebar.png`
  (cropped to the activity-bar + secondary-sidebar region).
- Tries to open any "Zen Mode" tooltip button and captures the open menu.

Adjust the `BASE` URL path, the crop rects, and the post-action interactions
for the specific change you're verifying.

## After capturing

1. `SendUserFile` the PNGs so the user sees them.
2. Clean up local-only tooling (see "Setup" above) — the stop hook will
   complain otherwise.
3. Optionally `pkill -f "vite --mode mock"` to free the port.

## Regression tests (committed)

`regression-credentials.mjs` (next to this file) is a committed, assertion-based
regression for the provider "Add API Key" flow on `/credentials`. It guards the
fixes from PR #996:

- a free-typed provider produces a well-formed POST payload (name + api_base +
  token) **without** clicking "Test Connection" first;
- notifications render via the unified top-right stack (not a bottom-right
  page-local Snackbar);
- the submit button shows a spinner while the request is in flight.

Run it after the Setup + dev-server steps above (it resolves `playwright` from
the cwd, so run from `frontend/`):

```bash
node ../.claude/skills/ui-preview/regression-credentials.mjs   # exits 0 / 1
```

It drives a real headless browser and asserts on the captured outgoing request
and DOM. In mock mode there is no `POST /api/v2/providers` handler, so the
request 404s and you'll see a `[pageerror] ... reading 'success'` line — that's
expected; the test asserts on the payload + the resulting top-right error toast,
not on a successful save. Unlike `screenshot.mjs`, this file IS committed — it's
the regression asset, not throwaway tooling.

## Scenario routing graph (needs a REAL backend)

The Claude Code **scenario routing graph** (`RuleCard` → `UnifiedRoutingGraph`
on `/agent/claude_code`) is a frequently-requested screenshot, but **mock mode
cannot render it**: unified mode fetches a `built-in-cc` rule the MSW handlers
don't return. You must run the real Go server and seed data through its API.

`scenario-routing-graph.mjs` (committed, next to this file) automates the
seed + capture. Full procedure:

```bash
# 1. Submodules must be checked out, or the Go build fails on libs/*.
git submodule update --init --recursive

# 2. Build + start the real server.
#    The server reuses its existing token from ~/.tingly-box/config.json
#    (field: user_token). It no longer prints a fresh token to the log.
#    Read the token before starting:
export TOKEN=$(python3 -c "import json; print(json.load(open('/root/.tingly-box/config.json'))['user_token'])")
go build -o /tmp/tingly-box ./cli/tingly-box
/tmp/tingly-box --verbose start --debug --port 12580 --browser=false \
  >> /tmp/tingly-server.log 2>&1 &
until curl -fs http://localhost:12580/ >/dev/null; do sleep 1; done

# 3. Frontend in REAL mode — proxies /api + /tingly to :12580.
#    NOTE: vite defaults to :3000 in real mode; check /tmp/vite-real.log.
cd frontend && USE_MOCK= npm run dev:real > /tmp/vite-real.log 2>&1 &
until curl -fs http://localhost:3000/ >/dev/null 2>&1 || \
      curl -fs http://localhost:3001/ >/dev/null 2>&1; do sleep 1; done

# 4. playwright must be installed in frontend/ (it's not in package.json).
npm i -D playwright   # run from frontend/

# 5. Seed providers + rules and capture (run from frontend/).
TOKEN=$TOKEN FE=http://localhost:3000 API=http://localhost:12580 \
  node ../.claude/skills/ui-preview/scenario-routing-graph.mjs
```

Outputs:
- `/tmp/scenario-routing-{light,dark}.png` — full page, all rules
- `/tmp/scenario-routing-smart-{light,dark}.png` — the smart-routing rule card

The script seeds two providers (`glm`, `deepseek`, with dummy keys via
`?force=true`) and three `claude_code` rules (one with smart routing).
Re-running appends more rules — restart the server (fresh config) for a clean
slate. Clean up afterwards:
`pkill -f "tingly-box.*start"; pkill -f "vite"`.

### Known issues with this script

**recharts / es-toolkit vite error** (`require_isUnsafeProperty is not a function`):
recharts v3.x imports `es-toolkit/compat/*` sub-paths. These resolve to CJS
wrappers that Vite 8's rolldown inlines with broken IIFE-helper naming. The
`es-toolkit/compat/*.js` shims don't have an `"import"` condition in
`package.json`, so rolldown uses the CJS path even for ESM bundles.

**Working fix**: patch the affected shim files in `node_modules` before
starting the dev server. These files are not tracked by git so the patch is
not committed. Recharts imports 11 functions; run this once per container:

```bash
for func in get isPlainObject last maxBy minBy omit range sortBy sumBy throttle uniqBy; do
  echo "export { ${func} as default } from '../dist/compat/index.mjs';" \
    > frontend/node_modules/es-toolkit/compat/${func}.js
done
```

Then clear the vite dep cache and restart: `rm -rf frontend/node_modules/.vite`.

**Why other approaches fail**:
- `resolve.alias` with string → appends sub-path to the alias path (broken)
- Plugin `resolveId` hook (normal or `enforce:'pre'`) → NOT called by rolldown's
  optimizeDeps pre-bundler; only works for the dev server's module-graph serving
- `optimizeDeps.exclude: ['recharts']` → recharts' deps (e.g. `use-sync-external-store`)
  also lack ESM shims and fail the same way

**"Separate Model" button not found** (mode-switch timeout):
The script was originally written for the old `RoutingGraph`/`SmartRoutingGraph`
architecture which had a "Separate Model" dialog. The new `UnifiedRoutingGraph`
has an inline `EntryNode` Direct/Smart toggle — no modal confirm step. Update
the script's mode-switch section to click the `EntryNode` Smart button directly
instead of looking for "Separate Model".

**playwright not in package.json**:
`playwright` is a dev dependency used only for screenshots — it's not in
`package.json` to avoid bloating production installs. Run `npm i -D playwright`
from `frontend/` each fresh container session before running the script.

## Why not Playwright MCP / Chrome MCP?

MCP servers are configured at the harness level (`settings.json`) and cannot
be installed or registered from inside an active session. We use Playwright's
Node API directly, which gives the same screenshot capability with no MCP
registration step.

## Why not `npx playwright install chromium`?

`cdn.playwright.dev` is not in the allowlist for this container. The
Chrome-for-Testing zip at `storage.googleapis.com/chrome-for-testing-public/...`
is reachable, so we download it directly and point Playwright at it via
`executablePath`.

## Why seed the auth token?

`src/contexts/AuthContext` and `ProtectedRoute` redirect every route to
the login screen when `localStorage.user_auth_token` is missing. MSW mocks
don't auto-populate it. Seeding it via `addInitScript` makes
`isAuthenticated` true on first paint.
