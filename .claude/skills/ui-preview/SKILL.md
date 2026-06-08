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

> ⚠️ **This project uses pnpm** (`frontend/pnpm-lock.yaml` is the source of truth).
> Do **NOT** run `npm i` here — npm ignores the pnpm lockfile and re-resolves
> transitive deps, which pulls **broken** versions (e.g. `es-toolkit@1.47.0` vs the
> pinned `1.46.1`) that crash the whole SPA at load. See *Troubleshooting* below.

```bash
# 1. Ensure deps match the lockfile (also fixes a node_modules previously
#    polluted by npm — wipe it first if you suspect that).
cd frontend
pnpm install --frozen-lockfile          # @emotion/react & styled are already deps

# 2. Add Playwright as a dev tool (no browsers downloaded).
pnpm add -D playwright

# 3. Download Chrome for Testing
mkdir -p /tmp/chrome && cd /tmp/chrome
curl -fsSL -o chrome.zip \
  "https://storage.googleapis.com/chrome-for-testing-public/148.0.7778.96/linux64/chrome-linux64.zip"
unzip -q chrome.zip          # → /tmp/chrome/chrome-linux64/chrome
```

`@emotion/react` / `@emotion/styled` are already declared in `package.json`, so a
plain `pnpm install` provides them — no separate install needed.

**Tooling-only** — never commit the playwright dev-dep. Revert with:
```bash
cd <repo> && git checkout -- frontend/package.json frontend/pnpm-lock.yaml
pnpm -C frontend install --frozen-lockfile   # restore the clean tree
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

## Troubleshooting

**Blank white page / `root` is empty, console shows
`TypeError: require_isUnsafeProperty is not a function`**
(stack points at `es-toolkit/dist/compat/object/get.js` ← `recharts`).

Cause: `node_modules` was installed/resolved by **npm** instead of pnpm, so a
transitive dep (`es-toolkit`) drifted to a version whose CJS→ESM interop esbuild
mis-bundles. It breaks the React vendor chunk, so **every** route renders blank —
not just the page you're testing. Tweaking `optimizeDeps` (include/exclude/
`keepNames`) does **not** fix it.

Fix — reinstall the pinned tree with pnpm:
```bash
cd frontend
rm -rf node_modules node_modules/.vite        # drop the npm-polluted tree + dep cache
pnpm install --frozen-lockfile
# verify the pinned (working) version is what's on disk:
ls node_modules/.pnpm | grep es-toolkit       # expect es-toolkit@1.46.1, NOT 1.47.0
pnpm add -D playwright                         # re-add the tool
```
Then restart the dev server. If you edited `vite.config.ts` while chasing this,
revert it — the config is not the problem.

**Dev-server process exits immediately (e.g. exit 144) on `--force`**: usually a
follow-on of the crash above (the unhandled client error). Once the pnpm tree is
correct it starts cleanly; `--force` is only needed once to drop a stale
`node_modules/.vite` optimize cache.
