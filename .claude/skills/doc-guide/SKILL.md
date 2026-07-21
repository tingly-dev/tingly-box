---
name: doc-guide
description: Generate or update the Tingly-Box Web UI guide at docs/guide/. Use when the user asks to "update the docs", "write documentation", "add a guide page", "retake screenshots", or when UI pages have changed and the guide needs to reflect them. Produces bilingual (zh + en) markdown under docs/guide/zh/ and docs/guide/en/, with screenshots in docs/guide/images/.
---

# Tingly-Box Guide Documentation

The guide lives at `docs/guide/` (force-added because `docs/` is in `.gitignore`):

```
docs/guide/
├── README.md           ← language selector (2 lines)
├── zh/
│   ├── README.md       ← Chinese TOC
│   └── NN-<topic>.md   ← numbered content files
├── en/
│   ├── README.md       ← English TOC
│   └── NN-<topic>.md   ← English mirrors of zh files
└── images/
    └── *.png           ← screenshots referenced by both zh/ and en/
```

## Structure conventions

- Files are numbered `01`–`NN` in reading order.
- Each topic is its own file (zh + en pair). The image path in both is `../images/<name>.png`.
- TOC sections in both README files:
  1. Getting Started
  2. Agent Scenarios
  3. Configuration Chain
  4. Other Main Entry Points
  5. System Settings
  6. Experimental Features
  7. Advanced Topics (routing rules, etc.)

## Writing a new doc page

1. Write the Chinese version under `docs/guide/zh/`.
2. Translate it to English under `docs/guide/en/`.
3. Add an entry to both `docs/guide/zh/README.md` and `docs/guide/en/README.md`.
4. Stage with `git add -f` (the `docs/` tree is gitignored):

```bash
git add -f docs/guide/zh/<file>.md docs/guide/en/<file>.md
git add -f docs/guide/zh/README.md docs/guide/en/README.md
# images (if new):
git add -f docs/guide/images/<name>.png
```

## Taking screenshots

All screenshots are captured by a single script:

```
.claude/skills/doc-guide/screenshot-docs.mjs
```

### Prerequisites (once per fresh container)

**1. Start the mock dev server**

```bash
cd frontend
USE_MOCK=true npm run dev:mock   # serves on :3000
```

`USE_MOCK=true` must be set as a shell env var — vite reads it from
`process.env` before `.env.mock` is applied. Without it you get 502s.

Wait until ready:
```bash
until curl -fs http://localhost:3000 >/dev/null; do sleep 1; done
```

**2. Install Playwright (not in package.json)**

```bash
cd frontend && npm i -D playwright
```

Playwright is tooling-only; do NOT commit `package.json` changes.
Revert if accidentally staged: `git checkout -- frontend/package.json`.

**3. Chromium auto-detection**

The script tries these paths in order:
- `/opt/pw-browsers/chromium-1194/chrome-linux/chrome` ← pre-installed in this container
- `/opt/pw-browsers/chromium_headless_shell-1194/…/chrome-headless-shell`
- `/tmp/chrome/chrome-linux64/chrome` ← manually downloaded fallback

If the pre-installed path is missing (different container build), download Chrome for Testing:

```bash
mkdir -p /tmp/chrome && cd /tmp/chrome
curl -fsSL -o chrome.zip \
  "https://storage.googleapis.com/chrome-for-testing-public/148.0.7778.96/linux64/chrome-linux64.zip"
unzip -q chrome.zip
```

`cdn.playwright.dev` is blocked by the container network policy; use the
`storage.googleapis.com/chrome-for-testing-public/` URL instead.

**4. Fix recharts / es-toolkit Vite error** (if you see `require_isUnsafeProperty is not a function`)

Rolldown inlines `es-toolkit/compat/*.js` CJS shims with broken IIFE-helper naming.
Patch them once per container:

```bash
for func in get isPlainObject last maxBy minBy omit range sortBy sumBy throttle uniqBy; do
  echo "export { ${func} as default } from '../dist/compat/index.mjs';" \
    > frontend/node_modules/es-toolkit/compat/${func}.js
done
rm -rf frontend/node_modules/.vite
```

### Run

From repo root:

```bash
node .claude/skills/doc-guide/screenshot-docs.mjs
```

The script outputs all PNGs directly to `docs/guide/images/`. It runs three batches:

| Batch | What |
|-------|------|
| 1 | 18 top-level pages (full viewport, mock data) |
| 2 | Detail / interaction shots (config modal, routing-related pages) |
| 3 | Routing graph (claude-code and sdk-proxy scenario pages) |

### Auth token injection

`src/contexts/AuthContext` redirects every route to the login screen when
`localStorage.user_auth_token` is missing. The script seeds it via
`context.addInitScript()` before any navigation, so all pages load authenticated.

### Feature flags

Guardrails and MCP pages only appear when their feature flags are enabled.
The script sets `feature_guardrails` and `feature_mcp` in localStorage so
those pages render correctly.

### Compressing screenshots (recommended)

The capture script writes plain `page.screenshot()` PNGs, which run
60–160 KB each — the doc set adds up fast (30+ images). Compress
after capturing, before committing, with `pngquant` (lossy, ~60–70%
smaller, no visible quality loss on UI screenshots):

```bash
apt-get install -y pngquant   # once per fresh container
cd docs/guide/images
for f in *.png; do
  pngquant --quality=75-92 --strip --force --skip-if-larger --output "/tmp/pq_$f" "$f"
  [ -f "/tmp/pq_$f" ] && mv "/tmp/pq_$f" "$f"
done
```

`oxipng`, `sharp`, or `Pillow` also work if `pngquant` isn't available.
Spot-check a few compressed images (Read tool) before committing —
text and fine UI lines should still be crisp.

## Routing graph screenshots

The routing graph (Direct/Tier mode, circuit breaker, Smart routing with
SmartOp conditions) is embedded in each scenario's rule card. **Mock mode
renders it** — the mock API returns a default rule with tier config.

For the most representative shot, use `/agent/claude_code` (Direct/Tier) and
`/agent/sdk-proxy` (often shows a simpler single-tier layout).

To capture Smart routing mode, add a click action in the script that targets
the Smart toggle button in the `EntryNode` at the top of the rule card:

```js
action: async (page) => {
    // Click the Smart toggle (AutoAwesome icon button in the EntryNode)
    const smartBtn = await page.$('[data-mode="smart"], button[aria-label*="Smart"]');
    if (smartBtn) { await smartBtn.click(); await page.waitForTimeout(1500); }
}
```

## Routing system reference

### Direct routing (Tier mode)

Services are arranged in priority tiers (T0 = highest). Within a tier: round-robin
load sharing. Across tiers: failover when all services in the current tier have
open circuits.

Circuit breaker per service: **Closed** → (3 failures) → **Open** → (30s cooldown)
→ **HalfOpen** (probe) → Closed or back to Open.

Mid-request failover via `firstChunkGate` buffer: if upstream fails before the
first response chunk arrives, the request transparently retries on another service.

### Smart routing

`smartEnabled: true` activates a chain of SmartOp sub-rules. First-match wins.
Each sub-rule uses AND logic across its conditions. The last rule must be
unconditional (ops=[]) as a catch-all.

SmartOp condition keys: `agent.claude_code` (main/subagent/compact), `token`
(ge/le N), `thinking` (on/off), `service_ttft` (fastest/fast/slow/slowest),
`service_capacity` (available/degraded/unavailable), `context_system`
(exists/missing), `latest_user` (text/image/file/rich).

### Rule extension flags

Flags live in `internal/typ/flag_registry.go` (backend source of truth) and
are rendered in `FlagCatalogDialog.tsx`. Categories:

- **App**: `cursor_compat`, `cursor_compat_auto`, `claude_code_compat`
- **Request (OpenAI)**: `custom_user_agent`, `openai_endpoint_override`,
  `use_max_completion_tokens`, `use_max_tokens`, `block_tools`
- **Response**: `skip_usage`
- **Reasoning**: `thinking_effort` (off / low ~1K / medium ~5K / high ~20K / max ~32K)
- **Vision**: `vision_proxy_service` (service_ref — model picker)
- **Routing**: `session_affinity` (TTL in seconds; 0 = disabled)

## Committing docs

All content under `docs/` is gitignored. Always use `git add -f`:

```bash
git add -f docs/guide/zh/<file> docs/guide/en/<file>
git add -f docs/guide/images/<file>.png
# Modified TOC files are already tracked, no -f needed:
git add docs/guide/zh/README.md docs/guide/en/README.md
```
