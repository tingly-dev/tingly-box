// Capture the Claude Code *scenario routing graph* against a REAL backend.
//
// Why this exists: the routing graph (RuleCard -> RoutingGraph / SmartRoutingGraph)
// needs real rule + provider data. Mock mode (`dev:mock`) does NOT populate it:
//   - unified mode fetches `built-in-cc`, which the MSW handlers don't return;
//   - switching to separate mode needs a config-apply the mock can't do.
// So we run the actual Go server, seed data via its API, and drive the real UI.
//
// Prereqs (see SKILL.md "Scenario routing graph" for the full walkthrough):
//   1. git submodule update --init --recursive   (libs/* must be checked out)
//   2. go build -o /tmp/tingly-box ./cli/tingly-box
//   3. /tmp/tingly-box --verbose start --debug --port 12580 --browser=false \
//        > /tmp/tingly-server.log 2>&1 &
//   4. cd frontend && npm run dev:real   (proxies /api + /tingly -> :12580;
//        note vite may fall back to :3001 if :3000 is taken)
//   5. Chrome-for-Testing at /tmp/chrome/... (see SKILL.md Setup)
//
// Run from frontend/ so `playwright` resolves:
//   API=http://localhost:12580 FE=http://localhost:3001 \
//   node ../.claude/skills/ui-preview/scenario-routing-graph.mjs
//
// TOKEN is read from $TOKEN, else parsed from /tmp/tingly-server.log
// ("Login Token: tb-user-..."). Outputs /tmp/scenario-routing-{light,dark}.png
// (full page) and /tmp/scenario-routing-smart-{light,dark}.png (smart card).

// Resolve playwright from the cwd (run this from frontend/, where playwright
// is installed) rather than from this file's location under .claude/.
import { createRequire } from 'node:module';
import { join } from 'node:path';
import { readFileSync } from 'fs';
const require = createRequire(join(process.cwd(), 'noop.js'));
const { chromium } = require('playwright');

const API = process.env.API || 'http://localhost:12580';
const FE = process.env.FE || 'http://localhost:3001';
const CHROME_PATH = process.env.CHROME_PATH || '/tmp/chrome/chrome-linux64/chrome';
const SCENARIO = 'claude_code';

function resolveToken() {
    if (process.env.TOKEN) return process.env.TOKEN;
    for (const f of ['/tmp/tingly-server.log']) {
        try {
            const m = readFileSync(f, 'utf8').match(/Login Token:\s*(tb-user-[0-9a-f]+)/);
            if (m) return m[1];
        } catch { /* ignore */ }
    }
    throw new Error('No TOKEN: set $TOKEN or ensure /tmp/tingly-server.log has the Login Token line');
}

const TOKEN = resolveToken();
const auth = { Authorization: `Bearer ${TOKEN}`, 'Content-Type': 'application/json' };

async function api(path, opts = {}) {
    const res = await fetch(`${API}${path}`, { ...opts, headers: { ...auth, ...(opts.headers || {}) } });
    const text = await res.text();
    let json; try { json = JSON.parse(text); } catch { json = text; }
    return { status: res.status, json };
}

async function seed() {
    // Providers (force=true skips the live connectivity check; token is a dummy).
    const mk = async (name, api_base) => {
        const r = await api('/api/v2/providers?force=true', {
            method: 'POST',
            body: JSON.stringify({ name, api_base, api_style: 'openai', token: 'demo-key', enabled: true }),
        });
        return r.json?.data?.uuid;
    };
    const glm = await mk('glm', 'https://open.bigmodel.cn/api/paas/v4');
    const ds = await mk('deepseek', 'https://api.deepseek.com/v1');
    if (!glm || !ds) throw new Error('provider seeding failed: ' + JSON.stringify({ glm, ds }));

    const rule = (body) => api('/api/v1/rule', { method: 'POST', body: JSON.stringify(body) });
    await rule({
        scenario: SCENARIO, request_model: 'claude-opus-4-7', description: 'Opus to GLM', active: true,
        services: [{ provider: glm, model: 'glm-4.6', weight: 1, active: true }],
    });
    await rule({
        scenario: SCENARIO, request_model: 'claude-sonnet-4-6', description: 'Sonnet with smart routing', active: true,
        services: [
            { provider: glm, model: 'glm-4.6', weight: 1, active: true },
            { provider: ds, model: 'deepseek-chat', weight: 1, active: true },
        ],
        smart_enabled: true,
        smart_routing: [{
            description: 'Long context to Deepseek',
            ops: [{ position: 'token', operation: 'gt', value: '8000' }],
            services: [{ provider: ds, model: 'deepseek-chat', weight: 1, active: true }],
        }],
    });
    await rule({
        scenario: SCENARIO, request_model: 'claude-haiku-4-5', description: 'Haiku to Deepseek', active: true,
        services: [{ provider: ds, model: 'deepseek-chat', weight: 1, active: true }],
    });
    console.log('seeded providers + 3 rules (incl. one smart-routing)');
}

async function shoot(browser, mode) {
    const ctx = await browser.newContext({ viewport: { width: 1500, height: 1100 }, deviceScaleFactor: 2 });
    await ctx.addInitScript(([t, m]) => {
        localStorage.setItem('user_auth_token', t);
        localStorage.setItem('tingly-theme-mode', m);
    }, [TOKEN, mode]);
    const page = await ctx.newPage();
    page.on('pageerror', e => console.log('[pageerror]', e.message.slice(0, 140)));
    await page.goto(`${FE}/agent/${SCENARIO}`, { waitUntil: 'networkidle', timeout: 60000 });
    await page.waitForTimeout(2500);

    // Default view is "Unified Model" (a single built-in rule). Switch to
    // "Separate Model" to render the per-rule routing graphs, confirming the dialog.
    try {
        await page.getByText('Separate Model', { exact: true }).first().click({ timeout: 8000 });
        await page.waitForTimeout(600);
        await page.getByRole('button', { name: /^Confirm$/ }).first().click({ timeout: 6000 });
    } catch (e) {
        console.log('[mode switch]', e.message.slice(0, 100));
    }
    await page.waitForTimeout(4000);

    await page.screenshot({ path: `/tmp/scenario-routing-${mode}.png`, fullPage: true });

    // Tight crop of the smart-routing rule card (has both the model + "Add Smart Rule").
    const tagged = await page.evaluate(() => {
        const ok = [...document.querySelectorAll('*')].find(el =>
            el.childElementCount > 0 &&
            /claude-sonnet-4-6/.test(el.textContent) &&
            /Add Smart Rule/.test(el.textContent) &&
            el.getBoundingClientRect().width > 500 && el.getBoundingClientRect().width < 1300 &&
            el.getBoundingClientRect().height > 150 && el.getBoundingClientRect().height < 700);
        if (!ok) return false;
        ok.id = '__smartcard';
        return true;
    });
    if (tagged) {
        await page.locator('#__smartcard').screenshot({ path: `/tmp/scenario-routing-smart-${mode}.png` });
    } else {
        console.log(`[${mode}] smart card not found (is data seeded + separate mode active?)`);
    }
    console.log(`[${mode}] captured`);
    await ctx.close();
}

await seed();
const browser = await chromium.launch({
    executablePath: CHROME_PATH,
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
await shoot(browser, 'light');
await shoot(browser, 'dark');
await browser.close();
console.log('done -> /tmp/scenario-routing-*.png');
