/**
 * screenshot-docs.mjs
 *
 * Captures all docs/guide/images screenshots in one pass.
 * Run from repo root:  node .claude/skills/doc-guide/screenshot-docs.mjs
 *
 * Prerequisites:
 *   - Mock dev server running: cd frontend && USE_MOCK=true npm run dev:mock
 *   - Playwright installed in frontend/: cd frontend && npm i -D playwright
 *   - es-toolkit shim patched (see doc-guide SKILL.md) if recharts errors appear
 *
 * Note: mock mode auto-seeds user_auth_token in main.tsx, so no manual
 * localStorage injection is needed for auth. Feature flags and onboarding
 * suppression are still injected via addInitScript below.
 */

// playwright lives in frontend/node_modules; createRequire resolves from cwd
// when called as:  node .claude/skills/doc-guide/screenshot-docs.mjs  (repo root)
import { createRequire } from 'module';
const { chromium } = createRequire(new URL('file:///home/user/tingly-box/frontend/'))('playwright');
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO    = path.resolve(__dirname, '../../../');
const OUT_DIR = path.join(REPO, 'docs/guide/images');
const BASE    = 'http://localhost:3000';

// Auto-detect Chromium: prefer pre-installed container path, fall back to
// a manually downloaded Chrome for Testing at /tmp/chrome.
const CHROME = [
    '/opt/pw-browsers/chromium-1194/chrome-linux/chrome',
    '/opt/pw-browsers/chromium_headless_shell-1194/chrome-headless-shell-linux64/chrome-headless-shell',
    '/tmp/chrome/chrome-linux64/chrome',
].find(p => fs.existsSync(p));
if (!CHROME) {
    console.error('No Chromium found. See SKILL.md for download instructions.');
    process.exit(1);
}
console.log(`Using Chrome: ${CHROME}`);
fs.mkdirSync(OUT_DIR, { recursive: true });

const browser = await chromium.launch({
    executablePath: CHROME,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
    headless: true,
});

// --- Context ---------------------------------------------------------------
// auth token is auto-seeded by MSW mock in main.tsx; we only need feature flags
// and onboarding suppression here.
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.addInitScript(() => {
    localStorage.setItem('feature_guardrails', 'true');
    localStorage.setItem('feature_mcp', 'true');
    // Suppress the onboarding wizard overlay on scenario pages
    localStorage.setItem('onboarding_complete', 'true');
    localStorage.setItem('onboarding_dismissed', 'true');
});

// --- Helpers ---------------------------------------------------------------

async function shot(name, route, { action, waitMs = 3500 } = {}) {
    const page = await ctx.newPage();
    page.on('pageerror', err => {
        if (!err.message.includes('Failed to get version'))
            console.error(`  [err] ${err.message.slice(0, 80)}`);
    });
    await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(waitMs);
    if (action) {
        await action(page);
        await page.waitForTimeout(1500);
    }
    const out = path.join(OUT_DIR, `${name}.png`);
    await page.screenshot({ path: out, fullPage: false });
    const size = fs.statSync(out).size;
    console.log(`  [${size > 10000 ? 'OK  ' : 'BLNK'}] ${name}.png  (${size}b)`);
    await page.close();
}

// Scroll the page's internal overflow container up by `px` pixels so the
// target element has breathing room above it in the viewport.
async function nudgeScrollUp(page, px = 120) {
    await page.evaluate((px) => {
        for (const n of document.querySelectorAll('*')) {
            const s = window.getComputedStyle(n);
            if ((s.overflowY === 'auto' || s.overflowY === 'scroll') && n.scrollHeight > n.clientHeight) {
                n.scrollTop = Math.max(0, n.scrollTop - px);
                break;
            }
        }
    }, px);
    await page.waitForTimeout(400);
}

// --- Batch 1: top-level pages ----------------------------------------------
console.log('\nBatch 1: top-level pages');

await shot('scenario-overview',  '/agent');
await shot('claude-code',        '/agent/claude_code',          { waitMs: 4500 });
await shot('credentials',        '/credentials');
await shot('dashboard',          '/dashboard/7d',               { waitMs: 4000 });
await shot('guardrails',         '/guardrails',                 { waitMs: 4000 });
await shot('guardrails-rules',   '/guardrails/rules',           { waitMs: 4500 });
await shot('guardrails-history', '/guardrails/history',         { waitMs: 4000 });
await shot('mcp',                '/mcp/sources',                { waitMs: 4000 });
await shot('system',             '/system');
await shot('remote-control',     '/remote-control/telegram');
await shot('remote-coder',       '/remote-coder/chat');
await shot('experimental',       '/system/experimental');
await shot('api-tokens',         '/tingly-box-token');
await shot('virtual-models',     '/credentials/virtual-models');
await shot('access-control',     '/access-control');
await shot('prompt-skills',      '/prompt/skill');
await shot('imagegen',           '/agent/imagegen',             { waitMs: 4500 });
await shot('playground',         '/agent/playground',           { waitMs: 4500 });

// --- Batch 2: detail / interaction shots -----------------------------------
console.log('\nBatch 2: detail & interaction shots');

await shot('zen-claude-code',       '/zen/claude_code',             { waitMs: 4500 });
await shot('zen-openai',            '/zen/openai',                  { waitMs: 4000 });
await shot('heatmap',               '/overview/180d',               { waitMs: 4500 });
await shot('dashboard-today',       '/dashboard/today',             { waitMs: 4500 });
await shot('remote-coder-sessions', '/remote-coder/sessions');
await shot('guardrails-groups',     '/guardrails/groups',           { waitMs: 4000 });
await shot('mcp-local-mode',        '/mcp/local-mode');
await shot('logs',                  '/system/logs');
await shot('servertool',            '/tools/servertool');

// Onboarding — shown without the suppression flags so the wizard appears.
{
    const page = await browser.newPage();
    page.on('pageerror', err => {
        if (!err.message.includes('Failed to get version'))
            console.error(`  [err] ${err.message.slice(0, 80)}`);
    });
    await page.addInitScript(() => {
        localStorage.setItem('feature_guardrails', 'true');
        localStorage.setItem('feature_mcp', 'true');
        // intentionally NOT setting onboarding_complete so the wizard renders
    });
    await page.goto(`${BASE}/onboarding`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(3500);
    const out = path.join(OUT_DIR, 'onboarding.png');
    await page.screenshot({ path: out, fullPage: false });
    console.log(`  [${fs.statSync(out).size > 10000 ? 'OK  ' : 'BLNK'}] onboarding.png  (${fs.statSync(out).size}b)`);
    await page.close();
}

// Config modal on the Claude Code page
await shot('claude-code-config-modal', '/agent/claude_code', {
    waitMs: 3000,
    action: async (page) => {
        const btns = await page.locator('button').all();
        for (const btn of btns) {
            const txt = (await btn.textContent() || '').trim();
            if (txt === 'Config' || txt === 'Auto Config') {
                await btn.click();
                await page.waitForTimeout(2000);
                break;
            }
        }
    },
});

// Connect AI dialog on the credentials page
await shot('connect-ai', '/credentials', {
    waitMs: 2500,
    action: async (page) => {
        try {
            const btn = page.getByRole('button', { name: /Connect AI/i });
            await btn.waitFor({ timeout: 6000 });
            await btn.click();
            await page.waitForTimeout(2000);
        } catch { /* ok if button not present */ }
    },
});

// --- Batch 3: routing graph & extensions catalog ---------------------------
console.log('\nBatch 3: routing graph & extensions');

// Routing graph — Direct mode: scroll to T0 tier label, nudge up for headroom.
{
    const page = await ctx.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.goto(`${BASE}/agent/claude_code`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(5000);
    await page.locator('text="T0"').first().scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(800);
    await nudgeScrollUp(page, 120);
    const out = path.join(OUT_DIR, 'routing-graph-direct.png');
    await page.screenshot({ path: out, fullPage: false });
    console.log(`  [${fs.statSync(out).size > 10000 ? 'OK  ' : 'BLNK'}] routing-graph-direct.png  (${fs.statSync(out).size}b)`);
    await page.close();
}

// Routing graph — Smart mode: same scroll, then click the "Smart" ToggleButton.
{
    const page = await ctx.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.goto(`${BASE}/agent/claude_code`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(5000);
    await page.locator('text="T0"').first().scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(600);
    await nudgeScrollUp(page, 120);
    await page.locator('button:has-text("Smart")').first().click().catch(() => {});
    await page.waitForTimeout(2000);
    const out = path.join(OUT_DIR, 'routing-graph-smart.png');
    await page.screenshot({ path: out, fullPage: false });
    console.log(`  [${fs.statSync(out).size > 10000 ? 'OK  ' : 'BLNK'}] routing-graph-smart.png  (${fs.statSync(out).size}b)`);
    await page.close();
}

// Rule Extensions catalog: scroll to Extensions card, click, capture dialog.
{
    const page = await ctx.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.goto(`${BASE}/agent/claude_code`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(5000);
    const extHeader = page.locator('text=/^Extensions/').first();
    await extHeader.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(600);
    await nudgeScrollUp(page, 80);
    await extHeader.click();
    await page.waitForSelector('[role="dialog"]', { timeout: 8000 }).catch(() => {});
    await page.waitForTimeout(1500);
    const out = path.join(OUT_DIR, 'rule-extensions.png');
    await page.screenshot({ path: out, fullPage: false });
    console.log(`  [${fs.statSync(out).size > 10000 ? 'OK  ' : 'BLNK'}] rule-extensions.png  (${fs.statSync(out).size}b)`);
    await page.close();
}

// Model Select dialog: switch to Direct mode, click the claude-sonnet ServiceNode
// (cursor:pointer card, w<300, h≈72) to open the "Choose Model" dialog.
{
    const page = await ctx.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.goto(`${BASE}/agent/claude_code`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(5000);
    // Ensure Direct routing mode so ServiceNodes are rendered as cards
    await page.locator('button[aria-label="Direct routing mode"]').click().catch(() => {});
    await page.waitForTimeout(2000);
    // Find the ServiceNode card by cursor:pointer + contains "claude-sonnet" + Anthropic + no "routing" text
    const svcNode = await page.evaluate(() => {
        for (const el of document.querySelectorAll('div')) {
            if (window.getComputedStyle(el).cursor !== 'pointer') continue;
            const text = el.innerText?.trim() || '';
            if (text.startsWith('claude-sonnet') && text.includes('Anthropic') && !text.includes('routing')) {
                const r = el.getBoundingClientRect();
                if (r.height > 40 && r.height < 120 && r.width < 400) return { x: r.x, y: r.y, w: r.width, h: r.height };
            }
        }
        return null;
    });
    if (svcNode) {
        await page.mouse.click(svcNode.x + svcNode.w / 2, svcNode.y + svcNode.h / 2);
        await page.waitForSelector('[role="dialog"]', { timeout: 6000 }).catch(() => {});
        await page.waitForTimeout(1500);
    }
    const out = path.join(OUT_DIR, 'model-select.png');
    await page.screenshot({ path: out, fullPage: false });
    console.log(`  [${fs.statSync(out).size > 10000 ? 'OK  ' : 'BLNK'}] model-select.png  (${fs.statSync(out).size}b)`);
    await page.close();
}

// Codex scenario page
await shot('codex', '/agent/codex', { waitMs: 3500 });

// Connect AI form (step 2): open connect-ai picker, click a non-OAuth provider card
{
    const page = await ctx.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.goto(`${BASE}/credentials`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(2500);
    const btn = page.getByRole('button', { name: /Connect AI/i });
    await btn.waitFor({ timeout: 6000 });
    await btn.click();
    await page.waitForTimeout(2000);
    // Click a non-OAuth provider to open the config form (e.g. "Custom endpoint")
    // Target the card's subtitle text rather than the title to avoid hitting header text
    const customCard = page.locator('text="Not listed? Bring your own URL"').first();
    await customCard.click().catch(async () => {
        // fallback: click "OpenAI" API key provider card
        await page.locator('text=OpenAI').first().click().catch(() => {});
    });
    await page.waitForTimeout(1500);
    const out = path.join(OUT_DIR, 'connect-ai-form.png');
    await page.screenshot({ path: out, fullPage: false });
    console.log(`  [${fs.statSync(out).size > 10000 ? 'OK  ' : 'BLNK'}] connect-ai-form.png  (${fs.statSync(out).size}b)`);
    await page.close();
}

// Routing guide — open once-per-user routing guide by NOT suppressing it
{
    const page = await browser.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.addInitScript(() => {
        localStorage.setItem('feature_guardrails', 'true');
        localStorage.setItem('feature_mcp', 'true');
        localStorage.setItem('onboarding_complete', 'true');
        localStorage.setItem('onboarding_dismissed', 'true');
        // intentionally NOT setting tb.routingGuideAutoShown so the routing guide auto-opens
    });
    await page.goto(`${BASE}/agent/claude_code`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(5000);
    const out = path.join(OUT_DIR, 'routing-guide.png');
    await page.screenshot({ path: out, fullPage: false });
    console.log(`  [${fs.statSync(out).size > 10000 ? 'OK  ' : 'BLNK'}] routing-guide.png  (${fs.statSync(out).size}b)`);
    await page.close();
}

// --- Done ------------------------------------------------------------------
await browser.close();
console.log(`\nAll screenshots written to ${OUT_DIR}`);
