/**
 * screenshot-docs.mjs
 *
 * Captures every docs/guide/images screenshot in one pass.
 * Run from repo root:
 *     node .claude/skills/doc-guide/screenshot-docs.mjs            # everything
 *     node .claude/skills/doc-guide/screenshot-docs.mjs projects   # only shots whose name matches /projects/
 *
 * Prerequisites:
 *   - Mock dev server running: cd frontend && USE_MOCK=true npm run dev:mock
 *   - Playwright installed in frontend/: cd frontend && npm i -D playwright
 *   - workspace packages linked (frontend/node_modules/@tingly/{ui,projects}) and
 *     react-markdown / remark-gfm resolvable — see SKILL.md "Fresh container".
 *
 * Mock mode auto-seeds the auth token (main.tsx). Everything the mock lacks
 * is worked around HERE (route interception / fetch patching) — never by
 * editing frontend/src/mocks.
 */

// playwright lives in frontend/node_modules; createRequire resolves from there.
import { createRequire } from 'module';
const { chromium } = createRequire(new URL('file:///home/user/tingly-box/frontend/'))('playwright');
import path from 'path';
import fs from 'fs';
import zlib from 'zlib';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO    = path.resolve(__dirname, '../../../');
const OUT_DIR = process.env.DOC_OUT || path.join(REPO, 'docs/guide/images');
const BASE    = process.env.DOC_BASE || 'http://localhost:3000';
const ONLY    = process.argv[2] ? new RegExp(process.argv[2]) : null;

const CHROME = [
    '/opt/pw-browsers/chromium-1243/chrome-linux64/chrome',
    '/opt/pw-browsers/chromium_headless_shell-1243/chrome-headless-shell-linux64/chrome-headless-shell',
    '/opt/pw-browsers/chromium-1194/chrome-linux/chrome',
    '/opt/pw-browsers/chromium_headless_shell-1194/chrome-headless-shell-linux64/chrome-headless-shell',
    '/tmp/chrome/chrome-linux64/chrome',
].find(p => fs.existsSync(p));
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

// --- Context ---------------------------------------------------------------
// Suppress every first-run overlay (one-time coach marks store '1').
const SUPPRESS = {
    onboarding_complete: 'true',
    onboarding_dismissed: 'true',
    'tb.routingGuideAutoShown': 'true',
    'layout.powerUpsTip.dismissed': '1',
    'layout.agentVisibilityTip.dismissed': '1',
};


// The mock's MCP handlers only know the config of the old workspace (no
// `enabled`, no routing snapshot, no Server Tools), so MCP Tools / profiles /
// Server Tools would sit on "Checking…" with an error banner. Answer
// GET /api/v1/mcp/{config,routing} here, shaped like the real backend
// (internal/mcp/runtime/routing.go).
const MCP_MOCK = (() => {
    const tool = (src, name, description, props, required = [], extra = {}) => ({
        source_id: src, name, normalized_name: `tingly_box_mcp__${src}__${name}`, description,
        input_schema: { type: 'object', properties: props, required },
        enabled: true, usage: { client: true, gateway: false }, implementation: 'mcp', ...extra,
    });
    const str = (d) => ({ type: 'string', description: d });
    const fsTools = [
        tool('filesystem', 'read_file', 'Read the complete contents of a file as text.', { path: str('File to read') }, ['path']),
        tool('filesystem', 'write_file', 'Create a new file or overwrite an existing one.', { path: str('File to write'), content: str('New contents') }, ['path', 'content']),
        tool('filesystem', 'list_directory', 'List the files and directories inside a path.', { path: str('Directory to list') }, ['path']),
        tool('filesystem', 'search_files', 'Recursively search for files matching a pattern.', { path: str('Where to start'), pattern: str('Glob pattern') }, ['path', 'pattern']),
    ];
    const ghTools = [
        tool('github', 'search_repositories', 'Search for GitHub repositories.', { query: str('Search query') }, ['query']),
        tool('github', 'get_pull_request', 'Get the details of a pull request.', { owner: str('Repository owner'), repo: str('Repository name'), pullNumber: { type: 'number', description: 'Pull request number' } }, ['owner', 'repo', 'pullNumber']),
        tool('github', 'list_issues', 'List the issues of a repository.', { owner: str('Repository owner'), repo: str('Repository name') }, ['owner', 'repo']),
        tool('github', 'create_issue', 'Open a new issue.', { owner: str('Repository owner'), repo: str('Repository name'), title: str('Issue title') }, ['owner', 'repo', 'title']),
    ];
    const webTools = [
        tool('server-webtools', 'web_search', 'Search the web (Serper).', { q: str('Query') }, ['q'], { usage: { client: false, gateway: true }, enabled: false }),
        tool('server-webtools', 'web_fetch', 'Fetch a URL and return it as Markdown (Jina Reader).', { url: str('URL to fetch') }, ['url'], { usage: { client: false, gateway: true } }),
    ];
    const advisorTools = [
        tool('advisor', 'advisor', 'Consult a second model for advice on the current task.', { question: str('What to ask') }, ['question'], { usage: { client: false, gateway: true }, implementation: 'virtual' }),
    ];
    const sources = [
        { id: 'filesystem', name: 'filesystem', origin: 'external', transport: 'stdio', address: 'npx -y @modelcontextprotocol/server-filesystem /home/user/projects', state: 'connected', processing: 'standard', tools: fsTools },
        { id: 'github', name: 'GitHub', origin: 'external', transport: 'http', address: 'https://api.githubcopilot.com/mcp/', state: 'connected', processing: 'standard', tools: ghTools },
        { id: 'postgres', name: 'postgres', origin: 'external', transport: 'stdio', address: 'npx -y @modelcontextprotocol/server-postgres', state: 'disconnected', processing: 'standard', tools: [] },
        { id: 'server-webtools', name: 'Web tools', origin: 'builtin', transport: 'stdio', address: 'Serper / Jina Reader', state: 'connected', processing: 'standard', tools: webTools },
        { id: 'advisor', name: 'Advisor', origin: 'builtin', transport: 'advisor', address: '', state: 'connected', processing: 'advisor', tools: advisorTools },
    ];
    const config = {
        sources: [
            { id: 'filesystem', name: 'filesystem', origin: 'external', enabled: true, transport: 'stdio', command: 'npx', args: ['-y', '@modelcontextprotocol/server-filesystem', '/home/user/projects'], tools: ['*'] },
            { id: 'github', name: 'GitHub', origin: 'external', enabled: true, transport: 'http', endpoint: 'https://api.githubcopilot.com/mcp/', preset: 'github', headers: { Authorization: 'Bearer ghp_xxx' }, tools: ['*'] },
            { id: 'postgres', name: 'postgres', origin: 'external', enabled: false, transport: 'stdio', command: 'npx', args: ['-y', '@modelcontextprotocol/server-postgres'], env: { DATABASE_URL: '' }, tools: ['*'] },
            { id: 'server-webtools', name: 'Web tools', origin: 'builtin', enabled: true, transport: 'stdio', command: 'tingly-box', args: ['mcp-builtin'], tools: ['*'], tool_policies: { web_search: { enabled: false } } },
            { id: 'advisor', name: 'Advisor', origin: 'builtin', enabled: true, transport: 'advisor', tools: ['*'] },
        ],
        profiles: [],
        request_timeout: 30,
    };
    const profileSources = sources.filter(s => s.id === 'filesystem' || s.id === 'github');
    const routing = {
        sources: sources.filter(s => s.id !== 'server-webtools' && s.id !== 'advisor'),
        profiles: [{ id: 'default', name: 'Default', endpoint: '/api/v1/mcp/default', enabled: true, default: true, sources: profileSources }],
        server_tools: [sources[3], sources[4]],
    };
    // The page lists Server Tools from routing.sources too, so keep them there.
    routing.sources = sources;
    return { config: { success: true, enabled: true, config }, routing: { success: true, enabled: true, routing } };
})();

function initScript(suppress) {
    return `(() => {
        try {
            for (const [k, v] of Object.entries(${JSON.stringify(suppress)})) localStorage.setItem(k, v);
        } catch {}
        // The mock's Claude Code rules are named after the old tiers and the
        // profile rules carry mock uuids; the real app names the main rule "cc"
        // (builtin:claude_code:cc, builtin:claude_code:<id>:cc for a profile).
        // Patch only the rules response so the pages read like a real install.
        const realFetch = window.fetch.bind(window);
        const MCP = ${JSON.stringify(MCP_MOCK)};
        window.fetch = async (...args) => {
            try {
                const u = String(args[0] && args[0].url ? args[0].url : args[0]);
                const method = String((args[1] && args[1].method) || (args[0] && args[0].method) || 'GET').toUpperCase();
                if (method === 'GET' && /\\/api\\/v1\\/mcp\\/config(\\?|$)/.test(u)) return new Response(JSON.stringify(MCP.config), { status: 200, headers: { 'Content-Type': 'application/json' } });
                if (method === 'GET' && /\\/api\\/v1\\/mcp\\/routing(\\?|$)/.test(u)) return new Response(JSON.stringify(MCP.routing), { status: 200, headers: { 'Content-Type': 'application/json' } });
            } catch {}
            const res = await realFetch(...args);
            try {
                const url = String(args[0] && args[0].url ? args[0].url : args[0]);
                const m = url.match(/\\/api\\/v1\\/rules\\?(?:.*&)?scenario=(claude_code(?:(?::|%3A)[^&]+)?)/);
                if (m) {
                    const body = await res.clone().json();
                    if (body && Array.isArray(body.data)) {
                        const scenario = decodeURIComponent(m[1]);
                        body.data = body.data.map((r, i) => {
                            if (scenario === 'claude_code' && r.uuid === 'builtin:claude_code:cc') return { ...r, request_model: 'cc' };
                            if (scenario.startsWith('claude_code:') && String(r.uuid).startsWith('mock-rule-cc-')) {
                                return { ...r, uuid: 'builtin:' + scenario + ':cc', request_model: 'cc' };
                            }
                            return r;
                        });
                        return new Response(JSON.stringify(body), { status: res.status, headers: res.headers });
                    }
                }
            } catch {}
            return res;
        };
    })();`;
}

// Server-side feature flags have no mock handler (it 404s, flags default off).
// Force them all on here instead of touching mocks/handlers.ts.
async function newContext({ suppress = SUPPRESS, flags = true, viewport = { width: 1440, height: 900 } } = {}) {
    const ctx = await browser.newContext({ viewport });
    await ctx.addInitScript(initScript(suppress));
    if (flags) {
        await ctx.route('**/api/v1/scenario/*/flag/*', route =>
            route.fulfill({ json: { success: true, data: { value: true } } }));
    }
    return ctx;
}
const ctx = await newContext();

// --- Helpers ---------------------------------------------------------------

let captured = 0;
const failed = [];

async function save(page, name) {
    // The model picker has a DEV-only "Test New" button (import.meta.env.DEV);
    // it doesn't exist in a production build, so keep it out of the guide.
    await page.evaluate(() => {
        for (const b of document.querySelectorAll('button')) if ((b.textContent || '').trim() === 'Test New') b.remove();
    }).catch(() => {});
    const out = path.join(OUT_DIR, `${name}.png`);
    await page.screenshot({ path: out, fullPage: false });
    const size = fs.statSync(out).size;
    captured++;
    if (size <= 10000) failed.push(name);
    console.log(`  [${size > 10000 ? 'OK  ' : 'BLNK'}] ${name}.png  (${size}b)`);
}

/**
 * One screenshot: open `route` in a fresh page, run `action(page)` if given,
 * save. A flaky selector is logged and the page is still captured, so one
 * moved button never takes down the whole run.
 */
async function shot(name, route, { action, waitMs = 3500, context = ctx, park = true } = {}) {
    if (ONLY && !ONLY.test(name)) return;
    const page = await context.newPage();
    page.on('pageerror', err => {
        if (!err.message.includes('Failed to get version'))
            console.error(`  [err] ${err.message.slice(0, 80)}`);
    });
    try {
        await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 20000 });
        await page.waitForTimeout(waitMs);
        if (action) {
            try {
                await action(page);
                // Park the pointer on empty canvas so no hover tooltip sticks.
                // (not for hover-driven shots — leaving the menu would close it)
                if (park) await page.mouse.move(1430, 890);
                await page.waitForTimeout(1200);
            } catch (err) {
                console.error(`  [action failed for ${name}] ${String(err?.message || err).slice(0, 140)}`);
                failed.push(`${name} (action)`);
            }
        }
        await save(page, name);
    } finally {
        await page.close();
    }
}

// Scroll the page's internal overflow container by `px` (negative = up).
async function scrollMain(page, px) {
    await page.evaluate((px) => {
        for (const n of document.querySelectorAll('*')) {
            const s = window.getComputedStyle(n);
            if ((s.overflowY === 'auto' || s.overflowY === 'scroll') && n.scrollHeight > n.clientHeight + 50 && n.clientWidth > 600) {
                n.scrollTop = Math.max(0, n.scrollTop + px);
                break;
            }
        }
    }, px);
    await page.waitForTimeout(500);
}

const click = (page, locator, opts = {}) => locator.first().click({ timeout: 6000, ...opts });

// A tiny solid-colour PNG for reference-image / import uploads (a real file —
// Playwright's setInputFiles needs bytes on disk).
function makeSolidPng(w, h, [r, g, b]) {
    const crcTable = Array.from({ length: 256 }, (_, n) => {
        let c = n;
        for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
        return c >>> 0;
    });
    const crc32 = (buf) => {
        let c = 0xffffffff;
        for (const byte of buf) c = crcTable[(c ^ byte) & 0xff] ^ (c >>> 8);
        return (c ^ 0xffffffff) >>> 0;
    };
    const chunk = (type, data) => {
        const typeBuf = Buffer.from(type, 'ascii');
        const lenBuf = Buffer.alloc(4);
        lenBuf.writeUInt32BE(data.length, 0);
        const crcBuf = Buffer.alloc(4);
        crcBuf.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])), 0);
        return Buffer.concat([lenBuf, typeBuf, data, crcBuf]);
    };
    const ihdr = Buffer.alloc(13);
    ihdr.writeUInt32BE(w, 0);
    ihdr.writeUInt32BE(h, 4);
    ihdr[8] = 8; ihdr[9] = 2;
    const raw = Buffer.alloc(h * (1 + w * 3));
    for (let y = 0; y < h; y++) {
        const rowStart = y * (1 + w * 3);
        for (let x = 0; x < w; x++) {
            const o = rowStart + 1 + x * 3;
            raw[o] = r; raw[o + 1] = g; raw[o + 2] = b;
        }
    }
    return Buffer.concat([
        Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
        chunk('IHDR', ihdr), chunk('IDAT', zlib.deflateSync(raw)), chunk('IEND', Buffer.alloc(0)),
    ]);
}
const FIXTURE_PNG = path.join('/tmp', 'doc-guide-fixture-upload.png');
fs.writeFileSync(FIXTURE_PNG, makeSolidPng(512, 512, [64, 150, 160]));

// ===========================================================================
// Batch 1: top-level pages
// ===========================================================================
console.log('\nBatch 1: top-level pages');

// The /agent card grid is gone (AgentLanding redirects to the last agent).
// Hiding/showing agents is the Agent sidebar's own edit mode (eye icon).
// Taller viewport so the dimmed, hidden rows (Cursor) below the divider are in frame.
await shot('scenario-overview', '/agent/claude_code', {
    waitMs: 4500,
    context: await newContext({ viewport: { width: 1440, height: 1380 } }),
    action: async (page) => { await click(page, page.locator('button[aria-label="Show or hide agents"]')); await page.waitForTimeout(800); },
});
await shot('claude-code',        '/agent/claude_code', { waitMs: 4500 });
await shot('claude-code-profile', '/agent/claude_code/profile/p1', { waitMs: 4500 });
await shot('codex',              '/agent/codex',       { waitMs: 4000 });
await shot('cursor-scenario',    '/agent/cursor',      { waitMs: 4000 });
await shot('custom-scenario',    '/agent/custom',      { waitMs: 3500 });
await shot('dsh-scenario',       '/agent/dsh',         { waitMs: 3500 });
await shot('pi-scenario',        '/agent/pi',          { waitMs: 3500 });
await shot('workbuddy-scenario', '/agent/workbuddy',   { waitMs: 3500 });
await shot('decisions-scenario', '/agent/decisions',   { waitMs: 3500 });
await shot('credentials',        '/credentials',       { waitMs: 4000 });
await shot('virtual-models',     '/credentials/virtual-models');
await shot('dashboard',          '/dashboard/today',   { waitMs: 4500 });
await shot('dashboard-team-usage', '/dashboard/users', { waitMs: 4000 });
await shot('quota-history',      '/dashboard/quota-history', { waitMs: 4000 });
await shot('image-playground',   '/image/playground',  { waitMs: 4500 });
await shot('image-api',          '/image/api',         { waitMs: 4000 });
await shot('image-library',      '/image/library',     { waitMs: 4000 });
// The mock seeds three profiles (browser IndexedDB, mock mode only).
await shot('image-profile',      '/image/profile/linxia-film', { waitMs: 4500 });
await shot('image-library-snippets', '/image/library', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByRole('button', { name: /^Snippets/ })); await page.waitForTimeout(800); },
});
await shot('team-workspace',     '/agent/team',        { waitMs: 4000 });
await shot('team-workspace-keys', '/agent/team', {
    waitMs: 4000,
    context: await newContext({ suppress: { ...SUPPRESS, 'team-keys-overview-expanded': 'true' } }),
});
await shot('team-keys',          '/agent/team/keys',   { waitMs: 4000 });
await shot('projects',           '/projects',          { waitMs: 4500 });
await shot('projects-im',        '/projects/im',       { waitMs: 4500 });
await shot('bots-overview',      '/bots/overview',     { waitMs: 4000 });
await shot('remote-control',     '/remote-agent',      { waitMs: 4000 });
await shot('im-notify',          '/notify',            { waitMs: 4000 });
await shot('guardrails',         '/guardrails',        { waitMs: 4000 });
await shot('guardrails-groups',  '/guardrails/groups', { waitMs: 4000 });
await shot('guardrails-rules',   '/guardrails/rules',  { waitMs: 4500 });
await shot('guardrails-credential', '/guardrails/credentials', { waitMs: 4000 });
await shot('guardrails-history', '/guardrails/history', { waitMs: 4000 });
// MCP & Tools: MCP Tools (connections + tools), a profile, Server Tools.
await shot('mcp-tools',          '/mcp/tools',         { waitMs: 4500 });
await shot('mcp-profile',        '/mcp/profile/default', { waitMs: 4500 });
await shot('mcp-server-tools',   '/mcp/server-tools',  { waitMs: 4000 });
await shot('prompt-skills',      '/prompt/skill');
await shot('prompt-user',        '/prompt/user');
await shot('bench',              '/bench',             { waitMs: 4000 });
await shot('system',             '/system');
await shot('access-control',     '/access-control');
await shot('experimental',       '/system/experimental');
await shot('develop',            '/system/develop');
await shot('logs',               '/system/logs');
await shot('help-page',          '/help',              { waitMs: 3000 });
// "Onboarding" is the Provider Catalog card on /help (every card starts collapsed).
await shot('onboarding',         '/help',              {
    waitMs: 3500,
    action: async (page) => { await click(page, page.getByText('Provider Catalog', { exact: true })); await page.waitForTimeout(1500); },
});
// The tray / menu-bar hub panel (/hub): compact quota list in its own window.
await shot('hub',                '/hub',               { waitMs: 4000, context: await newContext({ viewport: { width: 320, height: 560 } }) });

// ===========================================================================
// Batch 2: interactions and dialogs
// ===========================================================================
console.log('\nBatch 2: detail & interaction shots');

// Power-ups: rail gear -> Preferences popover -> hover the Power-ups row.
await shot('power-ups-menu', '/agent/claude_code', {
    waitMs: 3500,
    park: false,
    action: async (page) => {
        await page.getByRole('button', { name: 'Preferences' }).locator('visible=true').first().click({ timeout: 5000 });
        await page.waitForTimeout(600);
        await page.getByRole('menuitem', { name: /Power-ups/i }).hover({ timeout: 5000 });
        await page.waitForTimeout(1200);
    },
});
await shot('preferences-menu', '/agent/claude_code', {
    waitMs: 3500,
    action: async (page) => {
        await page.getByRole('button', { name: 'Preferences' }).locator('visible=true').first().click({ timeout: 5000 });
        await page.waitForTimeout(800);
    },
});

// Claude Code: Auto Config dialog.
await shot('claude-code-config-modal', '/agent/claude_code', {
    waitMs: 3500,
    action: async (page) => { await click(page, page.getByRole('button', { name: 'Auto Config' })); await page.waitForTimeout(2000); },
});
// ...its Manual tab.
await shot('claude-code-config-manual', '/agent/claude_code', {
    waitMs: 3500,
    action: async (page) => {
        await click(page, page.getByRole('button', { name: 'Auto Config' }));
        await page.waitForTimeout(1500);
        await click(page, page.getByRole('tab', { name: 'Manual' }));
    },
});
// Slots: split every slot off the default; each gets its own rule card.
await shot('claude-code-slots', '/agent/claude_code', {
    waitMs: 4000,
    action: async (page) => {
        await click(page, page.getByRole('button', { name: 'Split all' }));
        await page.waitForTimeout(3000);
        await scrollMain(page, 140);
    },
});
// A fresh install (?mockData=newcomer): nothing connected yet, so Quick Start is open at step 1.
// The newcomer mock also pops the once-per-user routing guide: close it first.
await shot('claude-code-quickstart', '/agent/claude_code?mockData=newcomer', {
    waitMs: 5500,
    action: async (page) => {
        await page.keyboard.press('Escape');
        await page.waitForTimeout(800);
        // The card starts folded ("0/4 · Connect an AI provider to get started"): open it.
        await page.getByText('Connect an AI provider to get started').first().click({ timeout: 4000 }).catch(() => {});
        await page.waitForTimeout(800);
    },
});
// Folded connection card (the persisted choice, as after the agent is connected).
await shot('agent-connection-folded', '/agent/codex', {
    waitMs: 6500,
    context: await newContext({ suppress: { ...SUPPRESS, 'agent-access-expanded-codex': 'false' } }),
});
// Config out-of-date chip + hover details.
// Profile page: Setup Guide dialog; override picker.
await shot('claude-code-profile-setup', '/agent/claude_code/profile/p1', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByRole('button', { name: 'Setup Guide' })); await page.waitForTimeout(1500); },
});
await shot('claude-code-profile-overrides', '/agent/claude_code/profile/p1', {
    waitMs: 4000,
    action: async (page) => {
        await click(page, page.getByRole('button', { name: /^Add$/ }));
        await page.waitForTimeout(1200);
        await scrollMain(page, 80);
    },
});
// Rule header chips: 1M menu, Endpoint menu.
await shot('rule-endpoint-menu', '/agent/claude_code', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByRole('button', { name: /Upstream endpoint/ })); await page.waitForTimeout(800); },
});
// Codex per-model auto-compact chip.
await shot('codex-compact', '/agent/codex', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByRole('button', { name: /Auto-compact for this model/ })); await page.waitForTimeout(1000); },
});
await shot('codex-config-modal', '/agent/codex', {
    waitMs: 3500,
    action: async (page) => { await click(page, page.getByRole('button', { name: 'Auto Config' })); await page.waitForTimeout(2000); },
});
// New Model: pick several models at once, one rule each.
await shot('new-model-multi', '/agent/codex', {
    waitMs: 5500,
    action: async (page) => {
        await click(page, page.getByText('New Model', { exact: true }));
        await page.waitForTimeout(1500);
        await page.getByText('Select multiple').first().click({ timeout: 4000 }).catch(() => {});
        await page.waitForTimeout(500);
        const dlg = page.locator('[role="dialog"]');
        await dlg.getByText('Anthropic', { exact: true }).first().click({ timeout: 4000 }).catch(() => {});
        await page.waitForTimeout(2500);
        // Pick two models: each gets a check, the footer counts them.
        for (const name of ['claude-sonnet-5', 'claude-opus-4-8']) {
            await dlg.getByText(name, { exact: true }).first().click({ timeout: 4000 }).catch(() => {});
            await page.waitForTimeout(400);
        }
    },
});
// WorkBuddy and Decisions guides.
await shot('workbuddy-config-modal', '/agent/workbuddy', {
    waitMs: 3500,
    action: async (page) => { await click(page, page.getByRole('button', { name: /Auto Config|Setup Guide/ })); await page.waitForTimeout(2000); },
});
await shot('decisions-guide', '/agent/decisions', {
    waitMs: 3500,
    action: async (page) => { await click(page, page.getByRole('button', { name: /Setup Guide/ })); await page.waitForTimeout(2000); },
});
await shot('cursor-config-modal', '/agent/cursor', {
    waitMs: 3000,
    action: async (page) => { await click(page, page.getByRole('button', { name: /Config|Setup Guide/ })); await page.waitForTimeout(1200); },
});

// Connect AI picker + the config form behind it.
await shot('connect-ai', '/credentials', {
    waitMs: 2500,
    action: async (page) => {
        const btn = page.getByRole('button', { name: /Connect AI/i }).first();
        await btn.waitFor({ timeout: 6000 });
        await btn.click();
        await page.waitForTimeout(2000);
    },
});
await shot('connect-ai-form', '/credentials', {
    waitMs: 2500,
    action: async (page) => {
        const btn = page.getByRole('button', { name: /Connect AI/i }).first();
        await btn.waitFor({ timeout: 6000 });
        await btn.click();
        await page.waitForTimeout(2000);
        await page.locator('text="Not listed? Bring your own URL"').first().click().catch(async () => {
            await page.locator('text=OpenAI').first().click().catch(() => {});
        });
        await page.waitForTimeout(1500);
    },
});
await shot('bots-connect-dialog', '/bots/overview', {
    waitMs: 3000,
    action: async (page) => {
        await page.getByRole('button', { name: /Connect a bot/i }).first().click().catch(() => {});
        await page.waitForSelector('[role="dialog"]', { timeout: 8000 }).catch(() => {});
        await page.waitForTimeout(1000);
    },
});

// Dashboard variants.
await shot('dashboard-by-request', '/dashboard/today', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByRole('tab', { name: 'By Request' })); await page.waitForTimeout(1500); },
});
await shot('dashboard-activity', '/dashboard/7d', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByRole('tab', { name: 'Activity' })); await page.waitForTimeout(1500); },
});
await shot('dashboard-range-menu', '/dashboard/today', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByText('Today', { exact: true })); await page.waitForTimeout(800); },
});
await shot('dashboard-team-usage-by-model', '/dashboard/users', {
    waitMs: 4000,
    action: async (page) => { await page.getByRole('button', { name: /By model/i }).first().click().catch(() => {}); await page.waitForTimeout(1200); },
});

// Image: mask editor, overview, library, profile.
await shot('image-mask-editor', '/image/playground', {
    waitMs: 4000,
    action: async (page) => {
        await page.locator('input[type="file"][accept="image/png,image/jpeg,image/webp"]').first().setInputFiles(FIXTURE_PNG);
        await page.waitForTimeout(1000);
        await page.getByLabel(/Mask an area to change/i).first().click();
        await page.waitForSelector('[role="dialog"]', { timeout: 8000 }).catch(() => {});
        await page.waitForTimeout(1000);
        const box = await page.locator('[role="dialog"] canvas').last().boundingBox();
        if (box) {
            await page.mouse.move(box.x + box.width * 0.3, box.y + box.height * 0.35);
            await page.mouse.down();
            await page.mouse.move(box.x + box.width * 0.6, box.y + box.height * 0.5, { steps: 8 });
            await page.mouse.move(box.x + box.width * 0.45, box.y + box.height * 0.65, { steps: 8 });
            await page.mouse.up();
        }
        await page.waitForTimeout(600);
    },
});
await shot('image-gallery', '/image/playground', {
    waitMs: 4000,
    action: async (page) => {
        await page.locator('[data-testid="imagegen-preview-panel"] input[type="file"]').first().setInputFiles(FIXTURE_PNG);
        await page.waitForTimeout(1000);
        await page.getByRole('button', { name: /^Overview$/ }).first().click().catch(() => {});
        await page.waitForTimeout(1200);
    },
});

// Projects: a project, a thread waiting on a question, new-project dialog, folder page + session.
await shot('projects-project', '/projects/p-latency', { waitMs: 4500 });
await shot('projects-thread', '/projects/p-latency', {
    waitMs: 4500,
    action: async (page) => { await click(page, page.getByText('Pick a retry backoff')); await page.waitForTimeout(2500); },
});
await shot('projects-thread-info', '/projects/p-latency', {
    waitMs: 4500,
    action: async (page) => {
        await click(page, page.getByText('Pick a retry backoff'));
        await page.waitForTimeout(2500);
        await click(page, page.getByRole('button', { name: /Show details/ }));
        await page.waitForTimeout(800);
    },
});
await shot('projects-settings', '/projects/p-latency', {
    waitMs: 4500,
    action: async (page) => { await click(page, page.getByRole('button', { name: 'Project settings' })); await page.waitForTimeout(1500); },
});
await shot('projects-layout-menu', '/projects/p-latency', {
    waitMs: 4500,
    action: async (page) => { await click(page, page.getByRole('button', { name: /^Layout/ })); await page.waitForTimeout(1000); },
});
await shot('projects-new', '/projects', {
    waitMs: 4000,
    action: async (page) => { await click(page, page.getByRole('button', { name: /New project/ })); await page.waitForTimeout(1200); },
});
await shot('projects-folder', '/projects/folder', { waitMs: 4500 });
await shot('projects-folder-session', '/projects/folder', {
    waitMs: 4500,
    action: async (page) => { await click(page, page.getByText('Add a dark mode toggle')); await page.waitForTimeout(3000); },
});

// MCP Tools: a tool row expanded to its copyable JSON schema.
await shot('mcp-tool-schema', '/mcp/tools', {
    waitMs: 4500,
    action: async (page) => { await click(page, page.getByRole('button', { name: /search_repositories/ })); await page.waitForTimeout(1000); },
});
// MCP Tools: the Connect tools dialog.
await shot('mcp-connect-tools', '/mcp/tools', {
    waitMs: 4500,
    action: async (page) => { await click(page, page.getByRole('button', { name: /Connect tools/ })); await page.waitForTimeout(1500); },
});

// Redesigned Probe panel: "Test All", then the status pill opens the dialog.
if (!ONLY || ONLY.test('probe-panel')) {
    const page = await ctx.newPage();
    page.on('pageerror', err => console.error(`  [err] ${err.message.slice(0, 80)}`));
    await page.goto(`${BASE}/agent/claude_code`, { waitUntil: 'domcontentloaded', timeout: 20000 });
    await page.waitForTimeout(5000);
    await page.locator('button:has-text("Test All")').first().click({ timeout: 10000 }).catch(() => {});
    await page.waitForTimeout(6000);
    const pill = page.locator('text=/^(\\d+ms|\\d+\\.\\d+s|Success|Failed)$/').first();
    await pill.waitFor({ timeout: 8000 }).catch(() => {});
    await pill.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(500);
    await pill.click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(1500);
    await page.locator('text="Request Journey"').first().click().catch(() => {});
    await page.waitForTimeout(500);
    await page.locator('text="Response"').first().click().catch(() => {});
    await page.waitForTimeout(600);
    await save(page, 'probe-panel');
    await page.close();
}

// ===========================================================================
// Batch 3: routing graph & plugin catalogs
// ===========================================================================
console.log('\nBatch 3: routing graph & plugins');

// There is no Direct/Smart switch any more: a rule is its Default row plus,
// optionally, conditions above it. Codex's rules have none (Default alone; hover
// reveals the dashed "+ Add condition" inside it); Claude Code's `cc` rule has four.
await shot('routing-graph-direct', '/agent/codex', {
    waitMs: 5000,
    park: false,
    action: async (page) => {
        await page.mouse.move(459, 384);
        await page.waitForTimeout(1500);
    },
});
await shot('routing-graph-smart', '/agent/claude_code', {
    waitMs: 5000,
    action: async (page) => { await scrollMain(page, 270); },
});
// Rule plugins: the header's "Plugins N" chip opens a popover; "Manage plugins"
// (or the ⋮ menu's Plugins row) opens the Flag Catalog dialog.
await shot('rule-plugins-popover', '/agent/claude_code', {
    waitMs: 5000,
    action: async (page) => { await click(page, page.getByRole('button', { name: /^Plugins \d+/ })); await page.waitForTimeout(800); },
});
await shot('rule-extensions', '/agent/claude_code', {
    waitMs: 5000,
    action: async (page) => {
        await click(page, page.getByRole('button', { name: /^Plugins \d+/ }));
        await page.waitForTimeout(600);
        await click(page, page.getByRole('button', { name: 'Manage plugins' }));
        await page.waitForSelector('[role="dialog"]', { timeout: 6000 }).catch(() => {});
        await page.waitForTimeout(1500);
    },
});
// Scenario-level Plugins strip -> panel.
await shot('agent-plugins-panel', '/agent/claude_code', {
    waitMs: 4500,
    action: async (page) => { await click(page, page.getByRole('button', { name: /Thinking/ })); await page.waitForTimeout(800); },
});
// Model Select dialog: click a service node.
await shot('model-select', '/agent/claude_code', {
    waitMs: 5000,
    action: async (page) => {
        await page.locator('button[aria-label="Direct routing mode"]').click().catch(() => {});
        await page.waitForTimeout(2000);
        const svc = await page.evaluate(() => {
            for (const el of document.querySelectorAll('div')) {
                if (window.getComputedStyle(el).cursor !== 'pointer') continue;
                const text = el.innerText?.trim() || '';
                if (text.startsWith('claude-sonnet') && text.includes('Anthropic') && !text.includes('routing')) {
                    const r = el.getBoundingClientRect();
                    if (r.height > 40 && r.height < 120 && r.width < 400) return { x: r.x, y: r.y, w: r.width, h: r.height };
                }
            }
            return null;
        });
        if (svc) {
            await page.mouse.click(svc.x + svc.w / 2, svc.y + svc.h / 2);
            await page.waitForSelector('[role="dialog"]', { timeout: 6000 }).catch(() => {});
            await page.waitForTimeout(1500);
        }
    },
});
// Routing guide: the once-per-user overlay, deliberately NOT suppressed.
await shot('routing-guide', '/agent/claude_code', {
    waitMs: 5000,
    context: await newContext({ suppress: { ...SUPPRESS, 'tb.routingGuideAutoShown': '' } }),
    action: async () => {},
});

// --- Done ------------------------------------------------------------------
await browser.close();
console.log(`\n${captured} screenshots written to ${OUT_DIR}`);
if (failed.length) console.log(`Needs attention: ${failed.join(', ')}`);
