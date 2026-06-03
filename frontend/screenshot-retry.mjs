import { chromium } from 'playwright';
import path from 'path';
import fs from 'fs';

const BASE = 'http://localhost:3000';
const CHROME = '/tmp/chrome/chrome-linux64/chrome';

const PAGES = [
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

const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });

// Enable experimental flags + auth
await context.addInitScript(() => {
  localStorage.setItem('user_auth_token', 'mock-token');
  // Enable guardrails flag in local storage for mock
  localStorage.setItem('feature_guardrails', 'true');
  localStorage.setItem('feature_mcp', 'true');
});

const page = await context.newPage();
page.on('pageerror', (err) => console.error('[pageerror]', err.message.slice(0, 100)));

for (const { name, path: route } of PAGES) {
  console.log(`Capturing ${name} (${route})...`);
  try {
    await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(5000);
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
