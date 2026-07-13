// Regression test for the provider "Add API Key" flow on /credentials.
//
// Guards against the bugs fixed in PR #996:
//   1. Free-typed provider must produce a well-formed POST payload WITHOUT
//      first clicking "Test Connection" (name + api_base + token populated).
//      Regression: a stale-state race sent name:"" and the backend rejected it.
//   2. Notifications must render via the unified top-right stack (top:24,
//      right:24), not a page-local bottom-right Snackbar.
//   3. The submit button must show a spinner while the request is in flight.
//
// Setup (from frontend/):
//   npm i -D playwright && npm i @emotion/react @emotion/styled
//   mkdir -p /tmp/chrome && curl -fsSL -o /tmp/chrome/chrome.zip \
//     "https://storage.googleapis.com/chrome-for-testing-public/148.0.7778.96/linux64/chrome-linux64.zip" \
//     && unzip -q /tmp/chrome/chrome.zip -d /tmp/chrome
//   USE_MOCK=true node_modules/.bin/vite --mode mock --port 3000 &
// Run from frontend/:
//   node ../.claude/skills/ui-preview/regression-credentials.mjs
//
// Exits 0 on success, 1 on any failed assertion. Screenshot: /tmp/regression-credentials.png
//
// Note: in mock mode there is no POST /api/v2/providers handler, so the request
// 404s — that is fine. We assert on the OUTGOING payload (captured in-page) and
// on the resulting top-right error toast, not on a successful save.

// Resolve playwright from the cwd (run this from frontend/, where playwright
// is installed) rather than from this script's folder under .claude/skills/.
import { createRequire } from 'node:module';
import { join } from 'node:path';
const require = createRequire(join(process.cwd(), 'noop.js'));
const { chromium } = require('playwright');

const CHROME_PATH = process.env.CHROME_PATH || '/tmp/chrome/chrome-linux64/chrome';
const BASE = process.env.BASE_URL || 'http://localhost:3000';
const VIEWPORT_W = 1440;

const failures = [];
const assert = (cond, msg) => { if (!cond) failures.push(msg); console.log(`${cond ? 'PASS' : 'FAIL'}: ${msg}`); };

const browser = await chromium.launch({
    executablePath: CHROME_PATH,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const context = await browser.newContext({ viewport: { width: VIEWPORT_W, height: 1000 }, deviceScaleFactor: 1 });

await context.addInitScript(() => {
    // Skip the login gate.
    localStorage.setItem('user_auth_token', 'mock-token');
    // Record outgoing POSTs to the providers endpoint, and delay them so the
    // in-flight submit spinner is observable.
    window.__posts = [];
    const orig = window.fetch;
    window.fetch = async (input, init) => {
        const url = input instanceof Request ? input.url : String(input);
        const method = (input instanceof Request ? input.method : (init?.method || 'GET')).toUpperCase();
        if (url.includes('/api/v2/providers') && method === 'POST') {
            let bodyText;
            if (input instanceof Request) { try { bodyText = await input.clone().text(); } catch {} }
            else if (init?.body != null) { bodyText = typeof init.body === 'string' ? init.body : JSON.stringify(init.body); }
            let body = bodyText; try { body = JSON.parse(bodyText); } catch {}
            window.__posts.push({ url, body });
            await new Promise(r => setTimeout(r, 1500));
        }
        return orig(input, init);
    };
});

const page = await context.newPage();
page.on('pageerror', err => console.log('[pageerror]', err.message.slice(0, 160)));

await page.goto(`${BASE}/credentials`, { waitUntil: 'networkidle', timeout: 60000 });
await page.waitForTimeout(2000);

// Open the Add API Key dialog and fill a free-typed provider — deliberately
// WITHOUT clicking "Test Connection".
await page.getByRole('button', { name: 'Add API Key' }).first().click();
const dialog = page.getByRole('dialog');
await dialog.waitFor({ timeout: 10000 });
await page.waitForTimeout(400);
await dialog.getByPlaceholder('Select a provider or enter custom base URL').fill('https://custom.example.com/v1');
await dialog.getByText('OpenAI Compatible', { exact: false }).click();
await dialog.getByPlaceholder('Your API key').fill('sk-regression-123');
await dialog.getByRole('button', { name: 'Add API Key' }).click();

// Mid-flight (request delayed 1.5s): the submit button should show a spinner.
await page.waitForTimeout(500);
const spinnerVisible = await page.evaluate(() => {
    // Multiple [role=dialog] nodes can be mounted (e.g. a placeholder plus the
    // real one), so scan submit buttons across all of them.
    const submit = Array.from(document.querySelectorAll('[role="dialog"] button[type="submit"]')).pop();
    if (!submit) return false;
    const hasProgress = !!submit.querySelector('.MuiCircularProgress-root, [role="progressbar"]');
    // While submitting, the label is replaced by the spinner, so the button is
    // disabled with no visible text.
    const disabledNoText = submit.disabled && submit.innerText.trim() === '';
    return hasProgress || disabledNoText;
});
await page.screenshot({ path: '/tmp/regression-credentials.png' });
assert(spinnerVisible, 'submit button shows a spinner while the request is in flight');

// Let the request settle (404 in mock) and the error toast appear.
await page.waitForTimeout(2000);

// Assertion 1: outgoing payload is well-formed.
const posts = await page.evaluate(() => window.__posts || []);
const body = posts.length ? (posts[posts.length - 1].body || {}) : {};
assert(posts.length > 0, 'a POST /api/v2/providers request was sent');
assert(!!body.name, `payload.name is populated (got ${JSON.stringify(body.name)})`);
assert(!!body.api_base, `payload.api_base is populated (got ${JSON.stringify(body.api_base)})`);
assert(!!body.token, 'payload.token is populated');

// Assertion 2: notifications render via the unified top-right stack.
const toasts = await page.evaluate(() => Array.from(document.querySelectorAll('.MuiAlert-root')).map(a => {
    const r = a.getBoundingClientRect();
    return { top: Math.round(r.top), right: Math.round(window.innerWidth - r.right) };
}));
const topRight = toasts.some(t => t.top <= 60 && t.right <= 60);
assert(toasts.length > 0, 'a notification toast is shown');
assert(topRight, `at least one toast is anchored top-right (got ${JSON.stringify(toasts)})`);

await browser.close();

console.log(`\n=== ${failures.length === 0 ? 'ALL CHECKS PASSED' : `${failures.length} CHECK(S) FAILED`} ===`);
process.exit(failures.length === 0 ? 0 : 1);
