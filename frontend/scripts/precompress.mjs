#!/usr/bin/env node
// Add precompressed siblings (index-AbCd.js -> .js.gz / .js.br) for every
// compressible file of a Vite build, in place, so the Go server
// (internal/server/webui_static.go) can serve them with no per-request
// compression, at the slowest and smallest level. `pnpm build` runs this on
// `dist`, which `task web:dist` copies into internal/web/dist and embeds.
//
//   node scripts/precompress.mjs dist
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';

const dir = process.argv[2];
if (!dir) {
    console.error('usage: precompress.mjs <dist-dir>');
    process.exit(2);
}

const TEXT = new Set(['.js', '.mjs', '.css', '.html', '.json', '.svg', '.txt', '.map', '.xml']);
// Keep in step with minCompressBytes in webui_static.go: smaller files are not
// worth a Content-Encoding variant.
const MIN_BYTES = 512;

const encoders = [
    ['.gz', (b) => zlib.gzipSync(b, { level: 9 })],
    ['.br', (b) => zlib.brotliCompressSync(b, {
        params: {
            [zlib.constants.BROTLI_PARAM_QUALITY]: 11,
            [zlib.constants.BROTLI_PARAM_SIZE_HINT]: b.length,
        },
    })],
];

let files = 0, raw = 0;
const out = Object.fromEntries(encoders.map(([ext]) => [ext, 0]));

function walk(d) {
    for (const e of fs.readdirSync(d, { withFileTypes: true })) {
        const p = path.join(d, e.name);
        if (e.isDirectory()) {
            walk(p);
            continue;
        }
        if (!TEXT.has(path.extname(e.name))) continue;
        const buf = fs.readFileSync(p);
        if (buf.length < MIN_BYTES) continue;
        files++;
        raw += buf.length;
        for (const [ext, enc] of encoders) {
            const z = enc(buf);
            // a variant that doesn't win is not written: the server then
            // falls back to the next one
            if (z.length >= buf.length) continue;
            fs.writeFileSync(p + ext, z);
            out[ext] += z.length;
        }
    }
}

walk(dir);
const kb = (n) => (n / 1024).toFixed(0) + ' KiB';
console.log(`${files} files, ${kb(raw)} raw; ` + Object.entries(out).map(([e, n]) => `${e} ${kb(n)}`).join(', '));
