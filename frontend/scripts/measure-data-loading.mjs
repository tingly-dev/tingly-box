// Measures what a page load costs: API requests (count, grouped by endpoint),
// JS/CSS bytes fetched, and requests that keep firing while the page sits idle.
//
// Runs against a *mock-mode* production build, so request counts are exactly
// what the frontend issues (the MSW worker answers them; payload sizes are the
// mock's, not the real backend's):
//
//   pnpm exec vite build --mode mock --outDir /path/to/dist-mock
//   pnpm exec vite preview --mode mock --outDir /path/to/dist-mock --port 41731
//   CHROME=~/.cache/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-linux64/chrome-headless-shell \
//     node scripts/measure-data-loading.mjs http://localhost:41731 [idleSeconds] [scenario...]
import { chromium } from 'playwright';

const base = process.argv[2] ?? 'http://localhost:41731';
const idleSeconds = Number(process.argv[3] ?? 0);
const only = process.argv.slice(4);

const API = /\/(api|openai|anthropic|tingly)\//;

const scenarios = {
    // Cold loads: first paint of the page, nothing cached.
    'cold:/dashboard/today': { path: '/dashboard/today' },
    'cold:/dashboard/7d': { path: '/dashboard/7d' },
    'cold:/dashboard/users': { path: '/dashboard/users' },
    'cold:/dashboard/quota-history': { path: '/dashboard/quota-history' },
    'cold:/agent/claude_code': { path: '/agent/claude_code' },
    'cold:/credentials': { path: '/credentials' },
    // The dashboard's three view tabs, clicked in order after the cold load.
    'tabs:/dashboard/today': {
        path: '/dashboard/today',
        steps: async (page) => {
            for (const name of ['By Request', 'Activity', 'Summary', 'By Request']) {
                await page.getByRole('tab', { name }).click();
                await page.waitForTimeout(1500);
            }
        },
    },
};

const browser = await chromium.launch({
    executablePath: process.env.CHROME,
    args: ['--no-sandbox'],
});

const rows = [];
for (const [name, sc] of Object.entries(scenarios)) {
    if (only.length && !only.some((o) => name.includes(o))) continue;
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    const api = [];
    let staticBytes = 0;
    let staticCount = 0;
    let phase = 'load';
    page.on('response', async (res) => {
        const url = new URL(res.url());
        let size = 0;
        try { size = (await res.body()).length; } catch { /* aborted */ }
        if (API.test(url.pathname)) {
            api.push({ phase, key: `${res.request().method()} ${url.pathname}`, size });
        } else if (/\.(js|css)$/.test(url.pathname)) {
            staticBytes += size;
            staticCount += 1;
        }
    });
    await page.addInitScript(() => localStorage.setItem('user_auth_token', 'mock-token'));
    await page.goto(base + sc.path, { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    if (sc.steps) { phase = 'steps'; await sc.steps(page); }
    if (idleSeconds) { phase = 'idle'; await page.waitForTimeout(idleSeconds * 1000); }

    const group = (phaseName) => {
        const g = new Map();
        for (const r of api.filter((r) => r.phase === phaseName)) {
            const e = g.get(r.key) ?? { n: 0, bytes: 0 };
            e.n += 1; e.bytes += r.size; g.set(r.key, e);
        }
        return g;
    };
    const total = (phaseName) => api.filter((r) => r.phase === phaseName).length;
    console.log(`\n=== ${name}  js/css: ${staticCount} files, ${(staticBytes / 1024).toFixed(0)} KB`);
    for (const p of ['load', 'steps', 'idle']) {
        if (!total(p)) continue;
        console.log(`  [${p}] ${total(p)} API requests, ${(api.filter((r) => r.phase === p).reduce((s, r) => s + r.size, 0) / 1024).toFixed(1)} KB`);
        for (const [k, e] of [...group(p)].sort((a, b) => b[1].n - a[1].n)) {
            console.log(`    ${String(e.n).padStart(3)}x ${k}  (${(e.bytes / 1024).toFixed(1)} KB)`);
        }
    }
    rows.push({ name, js: staticBytes, load: total('load'), steps: total('steps'), idle: total('idle') });
    await ctx.close();
}
console.log('\nSUMMARY');
for (const r of rows) console.log(`${r.name.padEnd(36)} jsKB=${(r.js / 1024).toFixed(0).padStart(5)} load=${r.load} steps=${r.steps} idle=${r.idle}`);
await browser.close();
