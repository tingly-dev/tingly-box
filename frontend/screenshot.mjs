import { chromium } from 'playwright';
import path from 'path';
import fs from 'fs';

const BASE = 'http://localhost:3000';
const CHROME = '/tmp/chrome/chrome-linux64/chrome';
const AUTH_TOKEN = 'mock-token';

const PAGES = [
  { name: 'scenario-overview', path: '/agent' },
  { name: 'claude-code', path: '/agent/claude_code' },
  { name: 'credentials', path: '/credentials' },
  { name: 'dashboard', path: '/dashboard/7d' },
  { name: 'guardrails', path: '/guardrails' },
  { name: 'mcp', path: '/mcp/sources' },
  { name: 'system', path: '/system' },
  { name: 'remote-control', path: '/remote-control/telegram' },
  { name: 'remote-coder', path: '/remote-coder/chat' },
  { name: 'experimental', path: '/system/experimental' },
  { name: 'api-tokens', path: '/tingly-box-token' },
  { name: 'virtual-models', path: '/credentials/virtual-models' },
  { name: 'access-control', path: '/access-control' },
  { name: 'prompt-skills', path: '/prompt/skill' },
  { name: 'guardrails-rules', path: '/guardrails/rules' },
  { name: 'guardrails-history', path: '/guardrails/history' },
  { name: 'imagegen', path: '/agent/imagegen' },
  { name: 'playground', path: '/agent/playground' },
];

const OUT_DIR = '/tmp/screenshots';
fs.mkdirSync(OUT_DIR, { recursive: true });

const browser = await chromium.launch({
  executablePath: CHROME,
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
  headless: true,
});

const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
});

// Inject auth token before page load so ProtectedRoute passes
await context.addInitScript(() => {
  localStorage.setItem('user_auth_token', 'mock-token');
});

const page = await context.newPage();
page.on('pageerror', (err) => console.error('[pageerror]', err.message.slice(0, 120)));
page.on('console', msg => {
  if (msg.type() === 'error') console.log('[console.error]', msg.text().slice(0, 100));
});

for (const { name, path: route } of PAGES) {
  console.log(`Capturing ${name} (${route})...`);
  try {
    await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    // Wait for any MUI content to appear
    await page.waitForTimeout(4000);
    const outPath = path.join(OUT_DIR, `${name}.png`);
    await page.screenshot({ path: outPath, fullPage: false });
    const stat = fs.statSync(outPath);
    console.log(`  -> saved ${outPath} (${stat.size} bytes)`);
  } catch (e) {
    console.error(`  ERROR capturing ${name}:`, e.message.slice(0, 100));
  }
}

await browser.close();
console.log('Done.');
