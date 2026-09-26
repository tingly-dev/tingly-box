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
 * Output ordering follows the sidebar (ActivityBar) layout top-to-bottom, so
 * the numbered files and output.gif walk the product the way the nav does:
 *
 *   Agent
 *     1-agents.png        – Agent selection overview
 *     2-claude-code.png   – Claude Code setup + routing rules
 *     3-routing.png       – OpenAI SDK smart routing
 *     4-model-select.png  – Model select dialog (routing graph → New Rule)
 *   Team
 *     5-team.png          – Team scenario: shared endpoint + model rules
 *   Image
 *     6-image-api.png     – Image API endpoint + image model rules
 *   Dashboard
 *     7-dashboard.png     – Usage dashboard (today)
 *     8-team-usage.png    – Per-user team usage
 *     9-heatmap.png       – Token heatmap (Activity view, 90d)
 *   Remote
 *     10-remote.png       – Telegram remote control routes
 *   Credential
 *     11-connect-ai.png   – Connect AI provider dialog
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
await shoot(browser, '/agent', '1-agents.png', { settle: 2500 });
await shoot(browser, '/agent/claude_code', '2-claude-code.png', { settle: 3000 });
await shoot(browser, '/agent/openai', '3-routing.png', { settle: 3000 });

// Model select dialog: open the routing page, click "New Rule" to open the
// ModelSelectDialog, then click the Anthropic provider tab so the right-side
// models panel loads. The dialog animates in; wait for its title first.
await shoot(browser, '/agent/openai', '4-model-select.png', {
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
await shoot(browser, '/agent/team', '5-team.png', { settle: 3000 });

// ── Image ─────────────────────────────────────────────────────────────────
await shoot(browser, '/image/api', '6-image-api.png', { settle: 3000 });

// ── Dashboard ─────────────────────────────────────────────────────────────
await shoot(browser, '/dashboard/today', '7-dashboard.png', {
    waitFor: '.MuiGrid-root', settle: 3500,
});
await shoot(browser, '/dashboard/users', '8-team-usage.png', { settle: 3500 });

// The heatmap lives inside the Dashboard page as a view-mode toggle
// ("Activity"), not its own route.
await shoot(browser, '/dashboard/90d', '9-heatmap.png', {
    waitFor: '.MuiGrid-root', settle: 3500,
    interact: async (page) => {
        try {
            const activityBtn = page.getByRole('button', { name: 'Activity' });
            await activityBtn.waitFor({ timeout: 6000 });
            await activityBtn.click();
            await page.waitForTimeout(1500);
        } catch (e) { console.warn('  ⚠ activity heatmap toggle failed:', e.message.slice(0, 80)); }
    },
});

// ── Remote ────────────────────────────────────────────────────────────────
await shoot(browser, '/remote-agent/telegram', '10-remote.png', { settle: 4000 });

// ── Credential ────────────────────────────────────────────────────────────
await shoot(browser, '/credentials', '11-connect-ai.png', {
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
