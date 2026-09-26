/**
 * Docs screenshot script — captures all docs/images/ product screenshots.
 *
 * Run from frontend/ (so node resolves playwright from node_modules):
 *   node ../.claude/skills/ui-preview/docs-screenshots.mjs
 *
 * Prerequisites: `pnpm install --frozen-lockfile` in frontend/ and a Chromium
 * (see SKILL.md setup).
 *
 * Dev server must be running on :3000:
 *   USE_MOCK=true node_modules/.bin/vite --mode mock --port 3000 &
 *
 * Output ordering follows the sidebar (ActivityBar) layout top-to-bottom.
 * Files are named `<group>-<index>-<name>.png`: the group number is the
 * sidebar entry, the index is the shot's position inside it — so a new shot
 * slots into its group without renumbering the others.
 *
 *   1 Agent
 *     1-1-agents.png            – Agent selection overview
 *     1-2-claude-code.png       – Claude Code setup + routing rules
 *     1-3-model-select.png      – Model select dialog (routing graph → New Rule)
 *   2 Team
 *     2-1-team.png              – Team scenario: shared endpoint + model rules
 *   3 Image
 *     3-1-image-playground.png  – Image Playground with a finished generation
 *     3-2-image-api.png         – Image API endpoint + image model rules
 *   4 Dashboard
 *     4-1-team-usage.png        – Per-user team usage
 *     4-2-dashboard.png         – Usage dashboard (today)
 *   5 Remote
 *     5-1-remote.png            – Telegram remote control routes
 *   6 Credential
 *     6-1-connect-ai.png        – Connect AI provider dialog
 *
 * Not every shot goes into output.gif — the frames are the explicit list in
 * docs/images/gif-frames.txt (read by create_gif.sh). Add a shot here first,
 * then list it there only if it belongs in the README demo.
 *
 *   theme-preview/{light,dark,claude}-dashboard.png
 *
 * Experimental features (guardrails, mcp, bench, desk, prompt skills) are
 * mocked always-on in mocks/handlers.ts; this script forces their flags off
 * so they neither get their own shot nor clutter the sidebar in the others.
 * The GitHub star banner is dismissed for the same reason.
 */
// playwright lives in frontend/node_modules; ESM bare-specifier resolution starts
// from the *file* location, not cwd. createRequire with a cwd-based URL makes it
// resolve from wherever the script is *run* from (i.e. frontend/).
import { createRequire } from 'module';
import fs from 'fs';
import path from 'path';
const { chromium } = createRequire('file://' + process.cwd() + '/')('playwright');

// Prefer the container's pre-installed Chromium; fall back to Chrome for Testing.
const CHROME = ['/opt/pw-browsers/chromium', '/tmp/chrome/chrome-linux64/chrome'].find(p => fs.existsSync(p));
const BASE   = 'http://localhost:3000';
// Script lives in .claude/skills/ui-preview/ but is run from frontend/,
// so ../docs/images resolves to the repo docs/images/ directory.
const OUTDIR = path.resolve('../docs/images');
const VP     = { width: 1440, height: 900 };

async function makePage(browser, theme = 'light') {
    const page = await browser.newPage();
    await page.setViewportSize(VP);
    page.on('pageerror', e => {
        if (!e.message.includes('Failed to get version')) console.error('[err]', e.message.slice(0, 120));
    });
    await page.addInitScript((t) => {
        localStorage.setItem('user_auth_token', 'mock-token-for-screenshots');
        localStorage.setItem('tingly-theme-mode', t);
        // Suppress first-run education overlays that would block product content
        localStorage.setItem('tb.routingGuideAutoShown', '1');
        sessionStorage.setItem('layout.githubStarBanner.dismissed', '1');
        // Force experimental feature flags off (mock mode turns them all on).
        const origFetch = window.fetch.bind(window);
        window.fetch = (input, init) => {
            const url = typeof input === 'string' ? input : input.url;
            const m = url.match(/\/api\/v1\/scenario\/_global\/flag\/(\w+)/);
            if (m && (!init || !init.method || init.method === 'GET') && (typeof input === 'string' || input.method === 'GET')) {
                return Promise.resolve(new Response(JSON.stringify({
                    success: true, data: { scenario: '_global', flag: m[1], value: false },
                }), { headers: { 'Content-Type': 'application/json' } }));
            }
            return origFetch(input, init);
        };
        // Collapse agent setup cards so routing rules are front-and-center
        for (const key of ['claude_code', 'codex', 'vscode', 'openai', 'xcode']) {
            localStorage.setItem(`setup-card-collapsed-${key}`, 'true');
        }
    }, theme);
    return page;
}

async function shoot(browser, route, filename, opts = {}) {
    const { theme = 'light', waitFor = 'nav', settle = 2500, interact } = opts;
    const page = await makePage(browser, theme);
    await page.goto(`${BASE}${route}`, { waitUntil: 'networkidle' });
    try { await page.waitForSelector(waitFor, { timeout: 8000 }); } catch { /* ok */ }
    await page.waitForTimeout(settle);
    if (interact) await interact(page);
    await page.screenshot({ path: path.join(OUTDIR, filename), fullPage: false });
    console.log(`  ✓ [${VP.width}×${VP.height}] ${route} → ${filename}`);
    await page.close();
}

const browser = await chromium.launch({
    executablePath: CHROME,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

// ── Agent ─────────────────────────────────────────────────────────────────
await shoot(browser, '/agent', '1-1-agents.png', { settle: 2500 });
await shoot(browser, '/agent/claude_code', '1-2-claude-code.png', { settle: 3000 });

// Model select dialog: open the routing page, click "New Rule" to open the
// ModelSelectDialog, then click the Anthropic provider tab so the right-side
// models panel loads. The dialog animates in; wait for its title first.
await shoot(browser, '/agent/openai', '1-3-model-select.png', {
    settle: 3000,
    interact: async (page) => {
        try {
            // Use JS click — getByRole/waitFor fails for this button despite it being visible
            await page.evaluate(() => {
                document.querySelector('button[aria-label="Create new routing rule"]')?.click();
            });
            const dialog = page.locator('[role="dialog"]').filter({ hasText: 'Select a model for your new rule' });
            await dialog.waitFor({ timeout: 8000 });
            await page.waitForTimeout(400);
            await dialog.getByText('Anthropic').first().click();
            await page.waitForTimeout(2000);
        } catch (e) { console.warn('  ⚠ model-select dialog failed:', e.message.slice(0, 80)); }
    },
});

// ── Team ──────────────────────────────────────────────────────────────────
await shoot(browser, '/agent/team', '2-1-team.png', { settle: 3000 });

// ── Image ─────────────────────────────────────────────────────────────────
// Playground: fill a prompt, ask for 4 images and generate, so the session
// panel shows a finished generation instead of the empty state. The mock
// /images/generations handler answers with placeholder SVGs.
await shoot(browser, '/image/playground', '3-1-image-playground.png', {
    settle: 2500,
    interact: async (page) => {
        try {
            await page.getByRole('textbox').first().fill(
                'A cozy isometric workshop where little robots assemble glowing AI chips, soft pastel palette');
            await page.getByRole('spinbutton', { name: 'N' }).fill('4');
            await page.getByRole('button', { name: /^Generate/ }).click();
            await page.waitForTimeout(2500);
            // Park the mouse so the Generate button's shortcut tooltip closes.
            await page.mouse.move(VP.width - 10, 10);
            await page.waitForTimeout(600);
        } catch (e) { console.warn('  ⚠ image playground generate failed:', e.message.slice(0, 80)); }
    },
});
await shoot(browser, '/image/api', '3-2-image-api.png', { settle: 3000 });

// ── Dashboard ─────────────────────────────────────────────────────────────
await shoot(browser, '/dashboard/users', '4-1-team-usage.png', { settle: 3500 });
await shoot(browser, '/dashboard/today', '4-2-dashboard.png', {
    waitFor: '.MuiGrid-root', settle: 3500,
});

// ── Remote ────────────────────────────────────────────────────────────────
await shoot(browser, '/remote-agent/telegram', '5-1-remote.png', { settle: 4000 });

// ── Credential ────────────────────────────────────────────────────────────
await shoot(browser, '/credentials', '6-1-connect-ai.png', {
    settle: 2500,
    interact: async (page) => {
        try {
            // Two matches when the empty-state CTA is also present; the toolbar one is first.
            const btn = page.getByRole('button', { name: /Connect AI/i }).first();
            await btn.waitFor({ timeout: 6000 });
            await btn.click();
            await page.waitForTimeout(1800);
        } catch (e) { console.warn('  ⚠ connect-ai dialog failed:', e.message.slice(0, 80)); }
    },
});

// ── Theme previews ────────────────────────────────────────────────────────
for (const theme of ['light', 'dark', 'claude']) {
    await shoot(browser, '/dashboard/today', `theme-preview/${theme}-dashboard.png`, {
        theme, waitFor: '.MuiGrid-root', settle: 3000,
    });
}

await browser.close();
console.log('\nAll done.');
