import { chromium } from 'playwright';
import path from 'path';
import fs from 'fs';

const BASE = 'http://localhost:3000';
const CHROME = '/tmp/chrome/chrome-linux64/chrome';
const OUT_DIR = '/tmp/screenshots2';
fs.mkdirSync(OUT_DIR, { recursive: true });

const browser = await chromium.launch({
  executablePath: CHROME,
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
  headless: true,
});

async function shot(context, name, route, { waitFor, action, waitMs = 3500 } = {}) {
  const page = await context.newPage();
  page.on('pageerror', err => console.error('[err]', err.message.slice(0, 80)));
  await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await page.waitForTimeout(waitMs);
  if (action) await action(page);
  if (waitFor) await page.waitForSelector(waitFor, { timeout: 8000 }).catch(() => {});
  await page.waitForTimeout(1500);
  const out = path.join(OUT_DIR, `${name}.png`);
  await page.screenshot({ path: out, fullPage: false });
  const size = fs.statSync(out).size;
  console.log(`[${size > 10000 ? 'OK' : 'BLANK'}] ${name} (${size}b) <- ${route}`);
  await page.close();
}

// Base context with auth + feature flags
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.addInitScript(() => {
  localStorage.setItem('user_auth_token', 'mock-token');
  localStorage.setItem('feature_guardrails', 'true');
  localStorage.setItem('feature_mcp', 'true');
});

// 1. Zen mode - Claude Code
await shot(ctx, 'zen-claude-code', '/zen/claude_code', { waitMs: 4000 });

// 2. Onboarding page
await shot(ctx, 'onboarding', '/onboarding', { waitMs: 3500 });

// 3. Heatmap overview
await shot(ctx, 'heatmap', '/overview/90d', { waitMs: 4000 });

// 4. Claude Code profile - navigate with a profile (mock data)
await shot(ctx, 'claude-code-profile', '/agent/claude_code', { waitMs: 4500 });

// 5. Claude Code - try to open the Config modal
await shot(ctx, 'claude-code-config-modal', '/agent/claude_code', {
  waitMs: 3000,
  action: async (page) => {
    // Look for Config button or Auto Config button
    const btns = await page.$$('button');
    for (const btn of btns) {
      const txt = await btn.textContent();
      if (txt && (txt.trim() === 'Config' || txt.trim() === 'Auto Config')) {
        await btn.click();
        await page.waitForTimeout(2000);
        break;
      }
    }
  }
});

// 6. Dashboard - today view (hourly chart)
await shot(ctx, 'dashboard-today', '/dashboard/today', { waitMs: 4000 });

// 7. Remote coder sessions page
await shot(ctx, 'remote-coder-sessions', '/remote-coder/sessions', { waitMs: 3500 });

// 8. Guardrails groups
await shot(ctx, 'guardrails-groups', '/guardrails/groups', { waitMs: 4000 });

// 9. MCP local mode
await shot(ctx, 'mcp-local-mode', '/mcp/local-mode', { waitMs: 3500 });

// 10. Provider list
await shot(ctx, 'provider-list', '/onboarding', { waitMs: 3500 });

// 11. Access control (already have, but take fresh full-detail shot)
await shot(ctx, 'access-control-full', '/access-control', { waitMs: 3500 });

// 12. Logs page
await shot(ctx, 'logs', '/system/logs', { waitMs: 3500 });

// 13. Servertool
await shot(ctx, 'servertool', '/tools/servertool', { waitMs: 3500 });

// Zen wider viewport for zen page
const zenCtx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await zenCtx.addInitScript(() => { localStorage.setItem('user_auth_token', 'mock-token'); });
await shot(zenCtx, 'zen-claude-code-wide', '/zen/claude_code', { waitMs: 5000 });
await shot(zenCtx, 'zen-openai', '/zen/openai', { waitMs: 4000 });

await browser.close();
console.log('Done.');
