/**
 * Docs screenshot script — captures all docs/images/ product screenshots.
 *
 * Run from frontend/ (so node resolves playwright from node_modules):
 *   node ../.claude/skills/ui-preview/docs-screenshots.mjs
 *
 * Prerequisites (run once per fresh container):
 *   cd frontend && npm i -D playwright
 *   mkdir -p /tmp/chrome && cd /tmp/chrome
 *   curl -fsSL -o chrome.zip \
 *     "https://storage.googleapis.com/chrome-for-testing-public/148.0.7778.96/linux64/chrome-linux64.zip"
 *   unzip -q chrome.zip
 *   cd <repo>/frontend && npm i @emotion/react @emotion/styled
 *
 * Dev server must be running on :3000:
 *   USE_MOCK=true node_modules/.bin/vite --mode mock --port 3000 &
 *
 * Output ordering (logical product story):
 *   1-dashboard.png      – Usage dashboard (today, minute-interval sparklines)
 *   2-agents.png         – Agent selection overview
 *   3-connect-ai.png     – Connect AI provider dialog
 *   4-model-select.png   – Model select dialog (opened from routing graph → New Rule)
 *   5-claude-code.png    – Claude Code setup + routing rules
 *   6-routing.png        – OpenAI SDK smart routing
 *   7-remote.png         – Telegram remote control bot
 *   8-guardrails.png     – Guardrails policies
 *   9-heatmap.png        – Token heatmap (180d)
 *   theme-preview/light-dashboard.png
 *   theme-preview/dark-dashboard.png
 *   theme-preview/claude-dashboard.png
 */
// playwright lives in frontend/node_modules; ESM bare-specifier resolution starts
// from the *file* location, not cwd. createRequire with a cwd-based URL makes it
// resolve from wherever the script is *run* from (i.e. frontend/).
import { createRequire } from 'module';
import path from 'path';
const { chromium } = createRequire('file://' + process.cwd() + '/')('playwright');

const CHROME = '/tmp/chrome/chrome-linux64/chrome';
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

// ── 1: Usage Dashboard (today) ────────────────────────────────────────────
await shoot(browser, '/dashboard/today', '1-dashboard.png', {
    waitFor: '.MuiGrid-root', settle: 3500,
});

// ── 2: Agent selection overview ───────────────────────────────────────────
await shoot(browser, '/agent', '2-agents.png', { settle: 2500 });

// ── 3: Connect AI provider dialog ─────────────────────────────────────────
await shoot(browser, '/credentials', '3-connect-ai.png', {
    settle: 2500,
    interact: async (page) => {
        try {
            const btn = page.getByRole('button', { name: /Connect AI/i });
            await btn.waitFor({ timeout: 6000 });
            await btn.click();
            await page.waitForTimeout(1800);
        } catch (e) { console.warn('  ⚠ connect-ai dialog failed:', e.message.slice(0, 80)); }
    },
});

// ── 4: Model select dialog ────────────────────────────────────────────────
// Open the routing page, click "New Rule" to open the ModelSelectDialog,
// then click the Anthropic provider tab so the right-side models panel loads.
// The dialog takes ~3s to animate in; wait for its title text before clicking.
await shoot(browser, '/agent/openai', '4-model-select.png', {
    settle: 3000,
    interact: async (page) => {
        try {
            const newRuleBtn = page.getByRole('button', { name: /Create new routing rule/i });
            await newRuleBtn.waitFor({ timeout: 6000 });
            await newRuleBtn.click();
            // Wait for the dialog title to appear (dialog animates in after ~2-3s)
            await page.getByText('Select a model for your new rule').waitFor({ timeout: 8000 });
            await page.waitForTimeout(400);
            // Click the first Anthropic tab to trigger model list fetch on the right panel
            await page.getByRole('dialog').getByText('Anthropic').first().click();
            await page.waitForTimeout(2000);
        } catch (e) { console.warn('  ⚠ model-select dialog failed:', e.message.slice(0, 80)); }
    },
});

// ── 5: Claude Code – routing rules loaded ────────────────────────────────
await shoot(browser, '/agent/claude_code', '5-claude-code.png', { settle: 3000 });

// ── 6: OpenAI SDK – smart routing diagram ────────────────────────────────
await shoot(browser, '/agent/openai', '6-routing.png', { settle: 3000 });

// ── 7: Telegram Remote Control ────────────────────────────────────────────
await shoot(browser, '/remote-control/telegram', '7-remote.png', { settle: 4000 });

// ── 8: Guardrails ─────────────────────────────────────────────────────────
await shoot(browser, '/guardrails', '8-guardrails.png', { settle: 3000 });

// ── 9: Token Heatmap (180d) ───────────────────────────────────────────────
await shoot(browser, '/overview/180d', '9-heatmap.png', {
    waitFor: '.MuiGrid-root', settle: 4500,
});

// ── Theme previews ────────────────────────────────────────────────────────
for (const theme of ['light', 'dark', 'claude']) {
    await shoot(browser, '/dashboard/today', `theme-preview/${theme}-dashboard.png`, {
        theme, waitFor: '.MuiGrid-root', settle: 3000,
    });
}

await browser.close();
console.log('\nAll done.');
