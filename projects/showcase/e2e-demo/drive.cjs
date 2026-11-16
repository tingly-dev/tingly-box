// Drives the demo UI with Playwright and saves the screenshots in this folder:
// approve both threads' commits, review a diff, merge it.
// Usage: node drive.cjs http://127.0.0.1:12590/project/<id>
// (set PW to the playwright package path if it is not resolvable, and
//  CHROMIUM to a browser executable if Playwright's own is not installed)
const { chromium } = require(process.env.PW || 'playwright');
const path = require('path');

const shot = (p, name) => p.screenshot({ path: path.join(__dirname, 'screenshots', name) });

(async () => {
  const url = process.argv[2];
  const browser = await chromium.launch(process.env.CHROMIUM ? { executablePath: process.env.CHROMIUM } : {});
  const p = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const open = async (title) => {
    await p.locator('main').getByText(title, { exact: true }).first().click();
    await p.waitForTimeout(1500);
  };
  await p.goto(url, { waitUntil: 'load' });

  // The coordinator starts two threads; both stop to ask before committing.
  await p.getByText('Waiting on you').nth(1).waitFor({ timeout: 60000 });
  await p.waitForTimeout(1000);
  await shot(p, '1-conversation-waiting.png');
  await open('Add CHANGELOG');
  await shot(p, '2-thread-permission.png');

  for (const t of ['Add CHANGELOG', 'Add greeting helper']) {
    await open(t);
    await p.getByRole('button', { name: 'Allow once' }).click();
  }
  await p.getByText('Ready for review').nth(1).waitFor({ timeout: 60000 });
  await p.goto(url, { waitUntil: 'load' });
  await p.waitForTimeout(2500);
  await shot(p, '3-conversation-ready.png');

  await open('Add greeting helper');
  await p.getByRole('button', { name: 'Review changes' }).click();
  await p.waitForTimeout(1500);
  await shot(p, '4-review-diff.png');
  await p.getByRole('button', { name: 'Merge it' }).click();
  await p.getByText('Resolved').first().waitFor({ timeout: 15000 });
  await p.waitForTimeout(1000);
  await shot(p, '5-merged.png');
  await browser.close();
})();
