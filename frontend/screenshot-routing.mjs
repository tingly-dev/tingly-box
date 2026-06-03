import { chromium } from 'playwright';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BASE_URL = 'http://localhost:3000';
const OUTPUT_DIR = path.join(__dirname, '../docs/guide/images');

const sleep = (ms) => new Promise(r => setTimeout(r, ms));

async function takeScreenshot(page, name) {
    const outPath = path.join(OUTPUT_DIR, `${name}.png`);
    await page.screenshot({ path: outPath, fullPage: false });
    const stat = (await import('fs')).statSync(outPath);
    console.log(`  ${name}.png (${stat.size} bytes)`);
}

async function main() {
    const browser = await chromium.launch({
        headless: true,
        executablePath: '/opt/pw-browsers/chromium-1194/chrome-linux/chrome',
    });
    
    try {
        const context = await browser.newContext({
            viewport: { width: 1400, height: 900 },
        });
        await context.addInitScript(() => {
            localStorage.setItem('user_auth_token', 'mock-token');
        });
        
        const page = await context.newPage();
        
        // Screenshot 1: Claude Code page with routing graph (Direct/Tier mode)
        console.log('Taking routing graph - Direct/Tier mode...');
        await page.goto(`${BASE_URL}/scenario/claude-code`, { waitUntil: 'networkidle' });
        await sleep(5000);
        await takeScreenshot(page, 'routing-graph-direct');
        
        // Screenshot 2: SDK Proxy page (shows routing graph with different config)
        console.log('Taking routing graph - SDK Proxy...');
        await page.goto(`${BASE_URL}/scenario/sdk-proxy`, { waitUntil: 'networkidle' });
        await sleep(5000);
        await takeScreenshot(page, 'routing-graph-sdk');
        
        console.log('\nDone!');
    } finally {
        await browser.close();
    }
}

main().catch(console.error);
