/**
 * Screenshot script for tingly-box docs/images
 * Run from frontend/: node screenshot.mjs
 *
 * Ordering (logical product story):
 *   1-dashboard.png      – Usage dashboard (today, hourly)
 *   2-connect-ai.png     – Connect AI provider dialog
 *   3-model-select.png   – Virtual Models / model selection
 *   4-agents.png         – Agent selection overview
 *   5-claude-code.png    – Claude Code setup + routing rules
 *   6-routing.png        – OpenAI SDK smart routing
 *   7-remote.png         – Telegram remote control bot
 *   8-guardrails.png     – Guardrails policies
 *   9-heatmap.png        – Token heatmap (180d)
 */
import { chromium } from 'playwright';
import path from 'path';

const CHROME = '/tmp/chrome/chrome-linux64/chrome';
const BASE   = 'http://localhost:3000';
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

// ── 2: Connect AI provider dialog ─────────────────────────────────────────
await shoot(browser, '/credentials', '2-connect-ai.png', {
    settle: 2500,
    interact: async (page) => {
        try {
            const btn = page.getByRole('button', { name: /Connect AI/i });
            await btn.waitFor({ timeout: 6000 });
            await btn.click();
            await page.waitForTimeout(1800);
        } catch (e) { console.warn('  ⚠ dialog open failed:', e.message.slice(0, 80)); }
    },
});

// ── 3: Virtual Models / model selection ───────────────────────────────────
await shoot(browser, '/credentials/virtual-models', '3-model-select.png', {
    settle: 2500,
});

// ── 4: Agent selection overview ───────────────────────────────────────────
await shoot(browser, '/agent', '4-agents.png', { settle: 2500 });

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
