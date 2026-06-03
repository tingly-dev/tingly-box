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
 */

import { chromium } from '../../frontend/node_modules/playwright/index.js';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO = path.resolve(__dirname, '../../../');
const OUT_DIR = path.join(REPO, 'docs/guide/images');
const BASE = 'http://localhost:3000';

// Auto-detect available Chromium: prefer the pre-installed opt path, fall
// back to a manually downloaded Chrome for Testing at /tmp/chrome.
const CHROME_CANDIDATES = [
    '/opt/pw-browsers/chromium-1194/chrome-linux/chrome',
    '/opt/pw-browsers/chromium_headless_shell-1194/chrome-headless-shell-linux64/chrome-headless-shell',
    '/tmp/chrome/chrome-linux64/chrome',
];
const CHROME = CHROME_CANDIDATES.find(p => fs.existsSync(p));
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

// --- Helper ------------------------------------------------------------

async function shot(ctx, name, route, { action, waitMs = 3500 } = {}) {
    const page = await ctx.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(waitMs);
    if (action) await action(page);
    if (action) await page.waitForTimeout(1500);
    const out = path.join(OUT_DIR, `${name}.png`);
    await page.screenshot({ path: out, fullPage: false });
    const size = fs.statSync(out).size;
    console.log(`  [${size > 10000 ? 'OK  ' : 'BLNK'}] ${name}.png  (${size}b)`);
    await page.close();
}

// --- Contexts ----------------------------------------------------------

// Standard context: auth + all feature flags
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.addInitScript(() => {
    localStorage.setItem('user_auth_token', 'mock-token');
    localStorage.setItem('feature_guardrails', 'true');
    localStorage.setItem('feature_mcp', 'true');
});

// Auth-only context (no feature flags) for feature-gated pages
const ctxClean = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctxClean.addInitScript(() => {
    localStorage.setItem('user_auth_token', 'mock-token');
});

// --- Batch 1: top-level pages ------------------------------------------
console.log('\nBatch 1: top-level pages');

await shot(ctx, 'scenario-overview',  '/agent');
await shot(ctx, 'claude-code',        '/agent/claude_code',               { waitMs: 4500 });
await shot(ctx, 'credentials',        '/credentials');
await shot(ctx, 'dashboard',          '/dashboard/7d',                    { waitMs: 4000 });
await shot(ctx, 'guardrails',         '/guardrails',                      { waitMs: 4000 });
await shot(ctx, 'guardrails-rules',   '/guardrails/rules',                { waitMs: 4500 });
await shot(ctx, 'guardrails-history', '/guardrails/history',              { waitMs: 4000 });
await shot(ctx, 'mcp',                '/mcp/sources',                     { waitMs: 4000 });
await shot(ctx, 'system',             '/system');
await shot(ctx, 'remote-control',     '/remote-control/telegram');
await shot(ctx, 'remote-coder',       '/remote-coder/chat');
await shot(ctx, 'experimental',       '/system/experimental');
await shot(ctx, 'api-tokens',         '/tingly-box-token');
await shot(ctx, 'virtual-models',     '/credentials/virtual-models');
await shot(ctx, 'access-control',     '/access-control');
await shot(ctx, 'prompt-skills',      '/prompt/skill');
await shot(ctx, 'imagegen',           '/agent/imagegen',                  { waitMs: 4500 });
await shot(ctx, 'playground',         '/agent/playground',                { waitMs: 4500 });

// --- Batch 2: detail / interaction shots ------------------------------
console.log('\nBatch 2: detail & interaction shots');

await shot(ctx, 'zen-claude-code',    '/zen/claude_code',                 { waitMs: 4500 });
await shot(ctx, 'zen-openai',         '/zen/openai',                      { waitMs: 4000 });
await shot(ctx, 'onboarding',         '/onboarding');
await shot(ctx, 'heatmap',            '/overview/90d',                    { waitMs: 4000 });
await shot(ctx, 'dashboard-today',    '/dashboard/today',                 { waitMs: 4000 });
await shot(ctx, 'remote-coder-sessions', '/remote-coder/sessions');
await shot(ctx, 'guardrails-groups',  '/guardrails/groups',               { waitMs: 4000 });
await shot(ctx, 'mcp-local-mode',     '/mcp/local-mode');
await shot(ctx, 'logs',               '/system/logs');
await shot(ctx, 'servertool',         '/tools/servertool');

// Config modal: click the Config / Auto Config button on the Claude Code page
await shot(ctx, 'claude-code-config-modal', '/agent/claude_code', {
    waitMs: 3000,
    action: async (page) => {
        const btns = await page.$$('button');
        for (const btn of btns) {
            const txt = await btn.textContent();
            if (txt && (txt.trim() === 'Config' || txt.trim() === 'Auto Config')) {
                await btn.click();
                await page.waitForTimeout(2000);
                break;
            }
        }
    },
});

// --- Batch 3: routing graph -------------------------------------------
console.log('\nBatch 3: routing graph');

await shot(ctx, 'routing-graph-direct', '/agent/claude_code',  { waitMs: 5000 });
await shot(ctx, 'routing-graph-sdk',    '/agent/sdk-proxy',    { waitMs: 5000 });

// --- Done -------------------------------------------------------------
await browser.close();
console.log(`\nAll screenshots written to ${OUT_DIR}`);
