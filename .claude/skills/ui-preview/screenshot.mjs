// Headless screenshot template for tingly-box frontend.
// Copy to frontend/screenshot.mjs and run from frontend/ so node resolves
// `playwright` from node_modules. See ../SKILL.md for setup.
//
// Customize:
//   - BASE_PATH:       route to screenshot (e.g. '/agent', '/zen/claude_code')
//   - viewport:        size of the captured viewport
//   - crops:           per-shot clip rects
//   - interactions:    extra clicks/hovers before captures
//
// Outputs go to /tmp/<name>.png. Use SendUserFile to surface them.

import { chromium } from 'playwright';

const CHROME_PATH = '/tmp/chrome/chrome-linux64/chrome';
const BASE = 'http://localhost:3000';
const BASE_PATH = '/agent';

const browser = await chromium.launch({
    executablePath: CHROME_PATH,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
});

// Skip the login gate — ProtectedRoute checks this localStorage key.
await context.addInitScript(() => {
    localStorage.setItem('user_auth_token', 'mock-token');
});

const page = await context.newPage();
page.on('console', msg => {
    if (msg.type() === 'error' || msg.type() === 'warning') {
        console.log(`[${msg.type()}]`, msg.text().slice(0, 240));
    }
});
page.on('pageerror', err => console.log('[pageerror]', err.message.slice(0, 240)));

await page.goto(`${BASE}${BASE_PATH}`, { waitUntil: 'networkidle', timeout: 60000 });
await page.waitForSelector('nav', { timeout: 20000 }).catch(() => console.log('no <nav>'));
await page.waitForTimeout(2500); // let MUI settle

const bodyText = await page.evaluate(() => document.body.innerText.slice(0, 500));
console.log('--- body text ---\n' + bodyText + '\n---');

// Full viewport
await page.screenshot({ path: '/tmp/page-full.png', fullPage: false });

// Cropped: activity bar + secondary sidebar (top-left region)
await page.screenshot({
    path: '/tmp/page-sidebar.png',
    clip: { x: 0, y: 0, width: 420, height: 280 },
});

// Optional: trigger a menu/dropdown and capture it.
// Example: click any "Zen Mode" tooltip button if present.
try {
    const btn = page.getByRole('button', { name: /Zen Mode|禅模式/i }).first();
    await btn.click({ timeout: 5000 });
    await page.waitForTimeout(500);
    await page.screenshot({
        path: '/tmp/page-menu.png',
        clip: { x: 0, y: 0, width: 600, height: 400 },
    });
} catch (e) {
    console.log('[interaction skipped]', e.message.slice(0, 200));
}

await browser.close();
console.log('done');
