#!/usr/bin/env node
// Layout guard for the Workspace / Projects screens: English (and every other
// language) strings are longer than Chinese, so a fixed-width control or a
// non-wrapping row clips them. This opens each screen in mock mode at several
// widths and fails on text that is cut off with no ellipsis, on tabs wider
// than their strip, and on a nav label in the activity rail that is truncated.
// Deliberate truncation (text-overflow: ellipsis with a title) is allowed.
//
//   pnpm dev:mock --port 4417 --strictPort &
//   BASE_URL=http://localhost:4417 node scripts/check-workspace-overflow.mjs [en,ru] [1440x900,1280x800,1024x768]
//   (ru has the longest strings: treat it as the stress test; the default is en,ru)
//   CHROME_PATH=/path/to/chrome  # optional; defaults to Playwright's own browser
//   SHOTS=/some/dir              # optional; saves a screenshot per screen

import { createRequire } from 'node:module';
import { mkdirSync } from 'node:fs';

const { chromium } = createRequire(import.meta.url)('playwright');
const base = process.env.BASE_URL || 'http://localhost:4417';
const langs = (process.argv[2] || 'en,ru').split(',');
const sizes = (process.argv[3] || '1440x900,1280x800,1024x768').split(',').map((s) => s.split('x').map(Number));
const shots = process.env.SHOTS;
if (shots) mkdirSync(shots, { recursive: true });

const project = '/projects/p-latency';
const tab = (index) => async (page) => { await page.getByRole('tab').nth(index).click({ timeout: 5000 }); await page.waitForTimeout(500); };
const button = (name) => async (page) => { await page.getByRole('button', { name }).first().click({ timeout: 5000 }); await page.waitForTimeout(700); };
const SCREENS = [
    ['home', '/workspace'],
    ['list', '/projects'],
    ['folder', `/projects/folder?path=${encodeURIComponent('/Users/me/code/notes-app')}`],
    ['im', '/projects/im'],
    ['project', project],
    ['thread', `${project}?thread=t-backoff`],
    ['library-tab', project, tab(1)],
    ['status-tab', project, tab(2)],
    ['new-project', '/projects', button(/New project|Новый проект/)],
    ['project-settings', project, button(/Project settings|Настройки проекта/)],
];

// Runs in the page; returns the cut-off text it finds.
const detect = () => {
    const found = [];
    const label = (el) => `${el.tagName.toLowerCase()} "${(el.innerText || el.getAttribute('aria-label') || '').replace(/\s+/g, ' ').slice(0, 50)}"`;
    if (document.documentElement.scrollWidth > window.innerWidth + 1) found.push(`page scrolls sideways (${document.documentElement.scrollWidth} > ${window.innerWidth})`);
    for (const el of document.querySelectorAll('body *')) {
        const cs = getComputedStyle(el);
        const rect = el.getBoundingClientRect();
        if (cs.display === 'none' || cs.visibility === 'hidden' || rect.width <= 1 || rect.height <= 1) continue;
        if (el.closest('svg, legend')) continue;
        let clipped = cs.overflowX !== 'visible' && cs.overflowX !== 'auto' && cs.overflowX !== 'scroll' && el.scrollWidth > el.clientWidth + 1;
        // Screen-reader-only text (1px, clipped) inflates scrollWidth without showing anything.
        if (clipped && cs.textOverflow !== 'ellipsis') {
            const edge = rect.left + el.clientWidth;
            clipped = [...el.querySelectorAll('*')].some((d) => { const r = d.getBoundingClientRect(); return r.width > 1 && r.height > 1 && r.right > edge + 1 && getComputedStyle(d).clip !== 'rect(0px, 0px, 0px, 0px)'; });
        }
        if (clipped && cs.textOverflow !== 'ellipsis') found.push(`clipped without ellipsis: ${label(el)} (${el.scrollWidth} > ${el.clientWidth})`);
        if (clipped && cs.textOverflow === 'ellipsis' && el.matches('.MuiTypography-caption') && el.closest('.MuiListItemButton-root')) found.push(`nav label truncated: ${label(el)} (${el.scrollWidth} > ${el.clientWidth})`);
        if (el.matches('[role=tab]')) {
            const strip = el.closest('.MuiTabs-scroller');
            if (strip && getComputedStyle(strip).overflowX === 'hidden' && rect.right > strip.getBoundingClientRect().right + 1) found.push(`tab cut off: ${label(el)}`);
        }
    }
    return found;
};

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined, args: ['--no-sandbox', '--disable-dev-shm-usage'] });
let failures = 0;
for (const lang of langs) {
    for (const [width, height] of sizes) {
        const ctx = await browser.newContext({ viewport: { width, height }, locale: lang === 'zh' ? 'zh-CN' : 'en-US' });
        await ctx.addInitScript((lng) => {
            localStorage.setItem('user_auth_token', 'mock-token-for-screenshots');
            localStorage.setItem('i18nextLng', lng);
            localStorage.setItem('layout.activeActivity', 'projects');
            for (const k of ['agentVisibilityTip', 'workspaceTip', 'powerUpsTip', 'teamPowerTip']) localStorage.setItem(`layout.${k}.dismissed`, '1');
        }, lang);
        const page = await ctx.newPage();
        for (const [name, path, act] of SCREENS) {
            const key = `${lang} ${width}px ${name}`;
            try {
                await page.goto(`${base}${path}${path.includes('?') ? '&' : '?'}mockOnboarding=off`, { waitUntil: 'load' });
                await page.waitForTimeout(1500);
                // A cold lazy locale or chunk shows a spinner first; measure the real page.
                for (let i = 0; i < 30 && (await page.locator('[role=progressbar]').count()) > 0; i++) await page.waitForTimeout(300);
                await page.waitForTimeout(1000);
                // The narrow layout moves the overview into a drawer, where there is no tab to click.
                if (act) await act(page);
            } catch (e) {
                // A control the narrow layout folds away is not a layout fault.
                if (width <= 1100 && /Timeout/.test(String(e))) continue;
                console.log(`FAIL ${key}: ${String(e).split('\n')[0]}`);
                failures++;
                continue;
            }
            if (shots) await page.screenshot({ path: `${shots}/${lang}-${width}-${name}.png` });
            const found = await page.evaluate(detect);
            for (const f of [...new Set(found)]) { console.log(`FAIL ${key}: ${f}`); failures++; }
        }
        await ctx.close();
    }
}
await browser.close();
console.log(failures ? `${failures} layout problem(s)` : 'no clipped text found');
process.exit(failures ? 1 : 0);
