// Browser journey for the Managed Agent feature: the real web UI served by a
// real tingly-box, driving a real `claude` CLI. Launched by browser_test.go,
// which provides:
//
//   TB_BASE_URL   the server (serves the built UI and the API)
//   TB_TOKEN      the user token (seeded into localStorage, as the login page does)
//   TB_FOLDER     an absolute folder to pick in the composer
//   TB_OUT        where screenshots go
//   TB_CHROME     optional Chrome/Chromium executable (Playwright's own otherwise)
//
// It exits non-zero on the first failed expectation; the Go test turns that
// into a test failure and keeps the screenshots for inspection.
import { createRequire } from 'module';
import path from 'path';
import fs from 'fs';

const frontendDir = process.env.TB_FRONTEND_DIR;
const require = createRequire(path.join(frontendDir, 'package.json'));
const { chromium } = require('playwright');

const BASE = process.env.TB_BASE_URL;
const TOKEN = process.env.TB_TOKEN;
const FOLDER = process.env.TB_FOLDER;
const OUT = process.env.TB_OUT;
fs.mkdirSync(OUT, { recursive: true });

const launch = { headless: true };
if (process.env.TB_CHROME) launch.executablePath = process.env.TB_CHROME;
const browser = await chromium.launch(launch);
const ctx = await browser.newContext({ viewport: { width: 1280, height: 860 } });
await ctx.addInitScript(([token]) => {
  localStorage.setItem('user_auth_token', token);
  localStorage.setItem('i18nextLng', 'en');
}, [TOKEN]);
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(String(e)));
page.on('console', (m) => { if (m.type() === 'error') errors.push('console: ' + m.text()); });

let step = 0;
const shot = async (name) => { step++; await page.screenshot({ path: path.join(OUT, `${String(step).padStart(2, '0')}-${name}.png`), fullPage: true }); };
const fail = async (msg) => { await shot('FAILED'); console.error('FAIL:', msg); if (errors.length) console.error('page errors:', errors); await browser.close(); process.exit(1); };
const expectText = async (text, timeout = 180_000) => {
  try { await page.getByText(text, { exact: false }).first().waitFor({ timeout }); }
  catch { await fail(`expected to see "${text}"`); }
};

try {
  // 1. Composer: pick the folder through the browse dialog.
  await page.goto(BASE + '/tasks', { waitUntil: 'networkidle' });
  await expectText('What should the agent do?', 30_000);
  await shot('composer');
  await page.getByRole('combobox').first().click();
  await page.getByRole('option', { name: /Browse for a folder/ }).click();
  await page.getByLabel('Folder path').fill(FOLDER);
  await page.getByLabel('Folder path').press('Enter');
  await page.waitForTimeout(500);
  await shot('folder-picker');
  await page.getByRole('button', { name: 'Use this folder' }).click();
  await expectText(path.basename(FOLDER), 10_000);

  // 2. Start the task and land on its detail page.
  await page.getByLabel('What should the agent do?').fill('Say hello from the browser journey');
  await shot('composer-filled');
  await page.getByRole('button', { name: 'Start' }).click();
  await page.waitForURL(/\/tasks\/[0-9a-f-]{36}/, { timeout: 30_000 });
  await expectText('Hello from the browser journey');
  await expectText('Waiting for you');
  await shot('first-turn-idle');

  // 3. Steer: the model asks to run a command; approve it from the UI.
  await page.getByPlaceholder('Send a follow-up…').fill('now create the marker file');
  await page.getByRole('button', { name: 'Send' }).click();
  await expectText('Needs your input');
  await shot('approval-pending');
  await page.getByRole('button', { name: 'Allow' }).click();
  await expectText('ran it (browser)');
  await expectText('browser-marker');
  await shot('after-approval');

  // 4. The folder now shows under "Folders used directly".
  await page.goto(BASE + '/tasks/sources', { waitUntil: 'networkidle' });
  await expectText('Folders used directly', 30_000);
  await expectText(FOLDER, 10_000);
  await shot('sources');
} catch (e) {
  await fail(String(e && e.stack || e));
}

if (errors.length) await fail('page errors: ' + errors.join('\n'));
console.log('browser journey ok');
await browser.close();
