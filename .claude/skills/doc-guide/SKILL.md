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
- TOC sections in both README files: I Getting Started · II Agent Scenarios · III Team ·
  IV Image · V Projects & Remote · VI Configuration Chain · VII Other Main Entry Points ·
  VIII System Settings · IX Experimental Features · X Advanced Topics.
- Numbering has gaps on purpose (`10` was the retired API Tokens page; `04` has three
  files; `07` has two). Never renumber — add the next free number (latest: `23`).
- Page-level layout facts live in ONE place: every agent page is the same shape and is
  documented in `03-scenario-claude-code.md`; the other agent chapters only say what differs.

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

**1b. Fresh container: link the workspace packages and the Projects deps**

`frontend/package.json` depends on `@tingly/{ui,projects}` (workspace packages under
`frontend/packages/`). A container that only has `node_modules` from an old install lacks
the links, and vite fails with *"could not be resolved: @tingly/ui/…"*. Fix it without
touching tracked files (`node_modules` is untracked):

```bash
cd frontend
ln -s ../../packages/ui       node_modules/@tingly/ui
ln -s ../../packages/projects node_modules/@tingly/projects
# @tingly/projects needs react-markdown + remark-gfm (+ their deps), absent from the old install:
mkdir -p /tmp/pp && cd /tmp/pp && npm init -y && npm i react-markdown@10.1.0 remark-gfm@4.0.1
for d in $(ls -A node_modules | grep -v '^[@.]'); do
  [ -e /path/to/frontend/node_modules/$d ] || ln -s /tmp/pp/node_modules/$d /path/to/frontend/node_modules/$d
done
rm -rf /path/to/frontend/node_modules/.vite
```
(or just `pnpm install` when the network allows it).

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

The script outputs all PNGs directly to `docs/guide/images/` (~95 shots, ~10 minutes).
Pass a regex to capture only matching shot names:

```bash
node .claude/skills/doc-guide/screenshot-docs.mjs "projects|image-"   # just those
```

| Batch | What |
|-------|------|
| 1 | Top-level pages (full viewport, mock data) |
| 2 | Interactions and dialogs (Auto Config, slots, Power-ups menu, Connect AI, Projects thread…) |
| 3 | Routing graph, rule plugins popover/catalog, Model Select, routing guide |

Each shot opens a **fresh page**; a failing selector is logged (`[action failed for …]`)
and the page is still captured, so scan the log for those and the `Needs attention:` line.
Look at every PNG afterwards (Read tool) — a shot can be "OK" and still show the wrong state.

### Mock-mode tricks (all inside the script — never edit `frontend/src/mocks`)

- **Fetch patch** (`initScript`): the mock names Claude Code's rules `claude-sonnet-5` and
  its profile rules `mock-rule-cc-…`; the real app names them `cc` / `builtin:claude_code:<id>:cc`.
  The init script wraps `window.fetch` and rewrites only the `/api/v1/rules?scenario=claude_code…`
  response so the pages (and the Slots row) read like a real install.
- **Coach marks**: one-time tips store `'1'` under `layout.powerUpsTip.dismissed` /
  `layout.agentVisibilityTip.dismissed`; `SUPPRESS` sets them. The routing guide is
  deliberately left unsuppressed for one shot.
- **Fresh install**: `?mockData=newcomer` (e.g. `/agent/claude_code?mockData=newcomer`) shows
  Quick Start in progress; the default populated mock has it finished/hidden.
- **Persisted UI state** can be seeded per context: `agent-access-expanded-<scenario>`,
  `team-keys-overview-expanded`.
- **Feature flags** (projects, guardrails, mcp, bench, skills): one `ctx.route` answers every
  `/api/v1/scenario/*/flag/*` with `true`.
- **Hub panel** (`/hub`) is shot in a 320×560 viewport (the tray panel's real size).
- Image profiles/library are browser IndexedDB data seeded only in mock mode
  (`/image/profile/linxia-film`, `tuanzi-book`, `lowpoly-poster`).
- Projects mock data: project `p-latency` (thread "Pick a retry backoff" has a question card),
  folder sessions under `/projects/folder`.

### Auth token injection

`src/contexts/AuthContext` redirects every route to the login screen when
`localStorage.user_auth_token` is missing. The script seeds it via
`context.addInitScript()` before any navigation, so all pages load authenticated.

### Feature flags

Guardrails, MCP, Bench, Projects and the Prompt pages only appear when their
server-side feature flags are on; the script forces them on (see above).

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

## Source-of-truth checklist when refreshing the guide

Do not trust the existing docs. For each page: read the page component, the locale files
(`frontend/src/i18n/locales/<area>/` — split by feature area; Projects strings live in
`frontend/packages/projects/src/i18n/en.ts` and `transcript/locales`), the nav in
`layout/useActivityItems.tsx`, and the design notes in `.design/`. Cross-check labels against
the **rendered screenshot**. Backend facts (flag lists, CLI help) come from
`internal/typ/flag_registry.go` and `internal/command/*`.

Final checks: no broken relative `.md` links / `#anchors` (GitHub slug rules: lowercase,
punctuation dropped, spaces→`-`; Chinese headings keep their characters), every image
referenced exists and every image is referenced, identical en/zh file sets.

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

Flags live in `internal/typ/flag_registry.go` (backend source of truth, served to the UI and
rendered by `FlagCatalogDialog.tsx`). Catalog sidebar order: **App**, **Request**, **Request (O)**,
**Request (A)**, **Response**, **Reasoning**, **Vision**, **Observe**, **Routing**.

- **App**: `cursor_compat`, `cursor_compat_auto`, `claude_code_compat`, `clean_header`
- **Request (protocol-agnostic)**: `extra_headers`
- **Request (OpenAI)**: `custom_user_agent`, `openai_endpoint_override`, `use_max_completion_tokens`,
  `use_max_tokens`, `block_tools`
- **Request (Anthropic)**: `anthropic_endpoint_override`, `claude_org_id`, `claude_code_version`, `context_1m`
- **Response**: `skip_usage`
- **Reasoning**: `thinking_effort` (By Client / off / low ~4K / medium ~10K / high ~20K / max ~32K)
- **Vision**: `vision_proxy_service` (service_ref — model picker)
- **Observe**: `recording` (multi-select: client request / upstream request)
- **Routing**: `session_affinity` (TTL in seconds; 0 = disabled)

In the UI a rule's header shows chips (`1M`, `Endpoint`, Codex-only `Compact`, `Plugins N`);
the Plugins chip opens a popover whose **Manage plugins** button opens the catalog.

SmartOp condition catalog (Smart routing sub-rules): `agent.claude_code`, `context_system`,
`latest_user`, `time`, `thinking`, `token`, `service_ttft`, `service_capacity`, `service_quota`
— authoritative operator list in `components/rule-card/SmartRuleCatalogDialog.tsx`.

## Committing docs

All content under `docs/` is gitignored. Always use `git add -f`:

```bash
git add -f docs/guide/zh/<file> docs/guide/en/<file>
git add -f docs/guide/images/<file>.png
# Modified TOC files are already tracked, no -f needed:
git add docs/guide/zh/README.md docs/guide/en/README.md
```
