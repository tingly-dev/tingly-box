---
name: ui-preview
description: Capture headless-Chrome screenshots of the tingly-box frontend (running locally in mock mode) so frontend changes can be visually verified in environments without a real browser. Use when the user asks to "preview", "screenshot", "see the page", "show me the UI", "verify visually", or when frontend layout / component / styling changes need a sanity-check before review. Works in restricted/cloud sandboxes where Playwright's normal Chromium install is blocked.
---

# Headless UI preview (Playwright + Chrome for Testing)

Designed for the remote-execution container where:
- `cdn.playwright.dev` / `dl.google.com` are blocked — use Chrome for Testing via `storage.googleapis.com`.
- Ubuntu's `chromium-browser` is a snap stub and won't run.
- MCP servers can't be registered mid-session — use Playwright's Node API directly.

## Setup (run once per fresh container)

```bash
# 1. Install Playwright (no browsers)
cd frontend && npm i -D playwright

# 2. Download Chrome for Testing
mkdir -p /tmp/chrome && cd /tmp/chrome
curl -fsSL -o chrome.zip \
  "https://storage.googleapis.com/chrome-for-testing-public/148.0.7778.96/linux64/chrome-linux64.zip"
unzip -q chrome.zip          # → /tmp/chrome/chrome-linux64/chrome

# 3. MUI emotion peer deps (not in package.json but required at runtime)
cd <repo>/frontend && npm i @emotion/react @emotion/styled
```

**Tooling-only** — never commit `package.json` / `package-lock.json`. Revert with:
```bash
git checkout -- frontend/package.json && rm -f frontend/package-lock.json
```

## Dev server

```bash
cd frontend
USE_MOCK=true node_modules/.bin/vite --mode mock --port 3000 &
until curl -fs http://localhost:3000 >/dev/null; do sleep 1; done
```

`USE_MOCK=true` must be a shell env var (read by `vite.config.ts` before `.env.mock`).

## Auth seeding

Every script must seed `localStorage.user_auth_token` via `addInitScript` or the app
redirects to the login screen:

```js
await page.addInitScript(() => {
    localStorage.setItem('user_auth_token', 'mock-token-for-screenshots');
});
```

## Scripts

All scripts live here and are run from `frontend/`. They use `createRequire(cwd)` to
resolve `playwright` from `frontend/node_modules` regardless of the script's location:

```js
import { createRequire } from 'module';
const { chromium } = createRequire('file://' + process.cwd() + '/')('playwright');
```

| Script | Mode | Purpose |
|--------|------|---------|
| [`screenshot.mjs`](./screenshot.mjs) | mock | **Ad-hoc template** — copy here and customise; do NOT commit |
| [`docs-screenshots.mjs`](./docs-screenshots.mjs) | mock | All 9 `docs/images/` product screenshots + theme previews |
| [`regression-credentials.mjs`](./regression-credentials.mjs) | mock | Assertion-based regression for `/credentials` Add API Key flow |
| [`scenario-routing-graph.mjs`](./scenario-routing-graph.mjs) | real backend | Claude Code routing graph screenshots (requires running Go server) |

Each script is self-documenting — see its file header for usage, outputs, and known issues.

## After capturing

1. `SendUserFile` the PNGs so the user can review them.
2. For ad-hoc `screenshot.mjs`: delete it before committing — the stop hook will flag it.
3. `docs/` is gitignored; force-add images: `git add -f docs/images/`.
4. Free the port: `fuser -k 3000/tcp` or `pkill -f "vite --mode mock"`.
