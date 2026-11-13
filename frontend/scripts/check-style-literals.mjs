#!/usr/bin/env node
// Style-literal guard (`.design/visual-system.md` §2.4 rule 7).
//
// Colours, text sizes and corner radii come from the theme: palette / tokens,
// typography variants or `fontSizes`, and `radius`. This check counts the
// literals that bypass it in every source file outside the theme and compares
// each file with a frozen baseline. Existing literals are tolerated; a file
// that gains new ones fails `pnpm lint`.
//
//   node scripts/check-style-literals.mjs           # check (runs in `pnpm lint`)
//   node scripts/check-style-literals.mjs --update  # re-freeze after removing literals
//
// A literal that really belongs at a call site (a chart series, a brand
// colour, a dark overlay on an image) is fine — run --update and say why in
// the PR.

import { readFileSync, writeFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = fileURLToPath(new URL('..', import.meta.url));
const BASELINE = join(ROOT, 'scripts', 'style-literals-baseline.json');
const SCAN = ['src', 'packages'];
const SKIP_DIR = new Set(['node_modules', 'dist', 'client', 'mocks', 'locales', 'assets']);
// The theme is where these values are supposed to live.
const ALLOWED = [`packages${sep}ui${sep}src${sep}theme${sep}`, `src${sep}theme${sep}`];

const RULES = {
  // '#2563eb', 'rgba(0, 0, 0, 0.1)' — use the palette or theme tokens.
  colour: /#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})\b|\brgba?\(/g,
  // fontSize: '0.85rem' / '13px' — use a typography variant or `fontSizes`.
  // (Bare numbers are icon sizes and stay allowed.)
  textSize: /fontSize:\s*['"`][\d.]+(?:rem|px|em)['"`]/g,
  // fontWeight beyond the three weights (400 / 500 / 600).
  weight: /fontWeight:\s*(?:['"`]bold['"`]|[7-9]00|650)/g,
  // borderRadius: 2 / '8px' — use `radius.*` ('50%' for dots and 0 are fine).
  radius: /borderRadius:\s*(?:[1-9][\d.]*|0\.\d+|['"`]\d+px['"`])/g,
};

const stripComments = (src) =>
  src
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:'"`\\])\/\/.*$/gm, '$1');

function* files(dir) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIR.has(name)) continue;
    const path = join(dir, name);
    if (statSync(path).isDirectory()) yield* files(path);
    else if (/\.(ts|tsx)$/.test(name) && !/\.(test|spec)\.tsx?$/.test(name) && !name.endsWith('.d.ts')) yield path;
  }
}

const count = () => {
  const out = {};
  for (const top of SCAN) {
    for (const path of files(join(ROOT, top))) {
      const rel = relative(ROOT, path);
      if (ALLOWED.some((a) => rel.startsWith(a))) continue;
      const src = stripComments(readFileSync(path, 'utf8'));
      const hits = {};
      for (const [rule, re] of Object.entries(RULES)) {
        const n = src.match(re)?.length ?? 0;
        if (n) hits[rule] = n;
      }
      if (Object.keys(hits).length) out[rel.split(sep).join('/')] = hits;
    }
  }
  return Object.fromEntries(Object.entries(out).sort(([a], [b]) => a.localeCompare(b)));
};

const now = count();

if (process.argv.includes('--update')) {
  writeFileSync(BASELINE, `${JSON.stringify(now, null, 2)}\n`);
  console.log(`style literals: baseline written (${Object.keys(now).length} files)`);
  process.exit(0);
}

const base = JSON.parse(readFileSync(BASELINE, 'utf8'));
const grown = [];
let shrunk = 0;
for (const [file, hits] of Object.entries(now)) {
  for (const [rule, n] of Object.entries(hits)) {
    const was = base[file]?.[rule] ?? 0;
    if (n > was) grown.push(`  ${file}: ${rule} ${was} → ${n}`);
  }
}
for (const [file, hits] of Object.entries(base)) {
  for (const [rule, n] of Object.entries(hits)) if ((now[file]?.[rule] ?? 0) < n) shrunk++;
}

if (grown.length) {
  console.error('style literals: new colour / size / weight / radius literals outside the theme');
  console.error(grown.join('\n'));
  console.error('Use the palette, theme tokens, typography variants, fontSizes or radius instead.');
  console.error('If the literal is deliberate, run `node scripts/check-style-literals.mjs --update`.');
  process.exit(1);
}
if (shrunk) console.log(`style literals: ok (${shrunk} entries below baseline — run with --update to lock them in)`);
else console.log('style literals: ok');
