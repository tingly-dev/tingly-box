// Capture the Claude Code *scenario routing graph* against a REAL backend.
//
// Why this exists: the routing graph (RuleCard -> RoutingGraph / SmartRoutingGraph)
// needs real rule + provider data. Mock mode (`dev:mock`) does NOT populate it:
//   - unified mode fetches `built-in-cc`, which the MSW handlers don't return;
//   - switching to separate mode needs a config-apply the mock can't do.
// So we run the actual Go server, seed data via its API, and drive the real UI.
//
// Prereqs:
//   1. git submodule update --init --recursive   (libs/* must be checked out)
//   2. export TOKEN=$(python3 -c "import json; print(json.load(open('/root/.tingly-box/config.json'))['user_token'])")
//   3. go build -o /tmp/tingly-box ./cli/tingly-box
//      /tmp/tingly-box --verbose start --debug --port 12580 --browser=false \
//        >> /tmp/tingly-server.log 2>&1 &
//      until curl -fs http://localhost:12580/ >/dev/null; do sleep 1; done
//   4. cd frontend && USE_MOCK= npm run dev:real > /tmp/vite-real.log 2>&1 &
//      (vite may fall back to :3001 if :3000 is taken)
//   5. npm i -D playwright   (from frontend/)
//
// Run from frontend/:
//   TOKEN=$TOKEN FE=http://localhost:3000 API=http://localhost:12580 \
//   node ../.claude/skills/ui-preview/scenario-routing-graph.mjs
//
// Outputs:
//   /tmp/scenario-routing-{light,dark}.png       — full page, all rules
//   /tmp/scenario-routing-smart-{light,dark}.png — smart-routing rule card
//
// Seeds two providers (glm, deepseek) and three claude_code rules; re-running
// appends more rules — restart the server for a clean slate.
// Cleanup: pkill -f "tingly-box.*start"; pkill -f "vite"
//
// ── Known issues ──────────────────────────────────────────────────────────
//
// recharts / es-toolkit vite error (require_isUnsafeProperty is not a function):
//   recharts v3.x imports es-toolkit/compat/* CJS shims that Vite 8/rolldown
//   inlines with broken IIFE helpers. Fix once per container before starting vite:
//     for func in get isPlainObject last maxBy minBy omit range sortBy sumBy throttle uniqBy; do
//       echo "export { ${func} as default } from '../dist/compat/index.mjs';" \
//         > frontend/node_modules/es-toolkit/compat/${func}.js
//     done
//   Then: rm -rf frontend/node_modules/.vite
//
// "Separate Model" button not found:
//   The script predates UnifiedRoutingGraph. The old RoutingGraph/SmartRoutingGraph
//   had a "Separate Model" modal; the new inline EntryNode has a Direct/Smart
//   toggle instead. Update the mode-switch section to click the Smart button
//   on the EntryNode directly.


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
    // Server reuses existing token from config — no longer prints to log.
    // Read it from ~/.tingly-box/config.json (field: user_token).
    for (const f of ['/root/.tingly-box/config.json', '/home/user/.tingly-box/config.json']) {
        try {
            const cfg = JSON.parse(readFileSync(f, 'utf8'));
            if (cfg.user_token) return cfg.user_token;
        } catch { /* ignore */ }
    }
    // Fallback: old "Login Token: tb-user-..." line in server log.
    for (const f of ['/tmp/tingly-server.log']) {
        try {
            const m = readFileSync(f, 'utf8').match(/Login Token:\s*(tb-user-[0-9a-f]+)/);
            if (m) return m[1];
        } catch { /* ignore */ }
    }
    throw new Error('No TOKEN: set $TOKEN env var, or ensure ~/.tingly-box/config.json has user_token');
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
    const mk = async (name, body_extra) => {
        const r = await api('/api/v2/providers?force=true', {
            method: 'POST',
            body: JSON.stringify({ name, api_style: 'openai', token: 'demo-key', enabled: true, ...body_extra }),
        });
        return r.json?.data?.uuid;
    };
    const glm = await mk('glm', { api_base: 'https://open.bigmodel.cn/api/paas/v4' });
    const ds  = await mk('deepseek', { api_base: 'https://api.deepseek.com/v1' });
    // fusion: supports both OpenAI and Anthropic API styles (shows dual O+A tags)
    const fusion = await mk('fusion', {
        api_base: 'https://fusion-proxy.example.com/v1',
        api_base_openai: 'https://fusion-proxy.example.com/v1',
        api_base_anthropic: 'https://fusion-proxy.example.com/anthropic',
    });
    if (!glm || !ds) throw new Error('provider seeding failed: ' + JSON.stringify({ glm, ds }));

    const rule = (body) => api('/api/v1/rule', { method: 'POST', body: JSON.stringify(body) });
    await rule({
        scenario: SCENARIO, request_model: 'claude-opus-4-7', description: 'Opus to GLM', active: true,
        services: [{ provider: glm, model: 'glm-4.6', weight: 1, active: true }],
    });
    await rule({
        scenario: SCENARIO, request_model: 'claude-sonnet-4-6', description: 'Sonnet with smart routing', active: true,
        services: [
            { provider: glm,    model: 'glm-4.6',      weight: 1, active: true },
            { provider: ds,     model: 'deepseek-chat', weight: 1, active: true },
        ],
        smart_enabled: true,
        smart_routing: [{
            description: 'Long context to Deepseek',
            ops: [{ position: 'token', operation: 'gt', value: '8000' }],
            services: [{ provider: ds, model: 'deepseek-chat', weight: 1, active: true }],
        }],
    });
    await rule({
        scenario: SCENARIO, request_model: 'claude-haiku-4-5', description: 'Haiku via fusion proxy', active: true,
        services: [{ provider: fusion || ds, model: 'claude-haiku-4-5', weight: 1, active: true }],
    });
    console.log('seeded providers + 3 rules (incl. one smart-routing, one fusion)');
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

    // The UnifiedRoutingGraph shows an EntryNode with inline Direct/Smart toggle.
    // No "Separate Model" dialog needed — rules render directly on the page.
    // Allow extra time for the real API data to load and React to settle.
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
