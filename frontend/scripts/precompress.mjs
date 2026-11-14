#!/usr/bin/env node
// Copy a Vite build to a deploy directory and add .gz / .br / .zst siblings for
// every compressible file, so Caddy's `file_server { precompressed }` can serve
// them without compressing per request (and at the slowest, smallest level).
//
//   node scripts/precompress.mjs dist /srv/tingly-web
//
// The source dist is left untouched on purpose: `task web:dist` embeds it into
// the Go binary, and the compressed copies would only bloat the binary.
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';

const [src, dest] = process.argv.slice(2);
if (!src || !dest) {
    console.error('usage: precompress.mjs <dist-dir> <deploy-dir>');
    process.exit(2);
}

const TEXT = new Set(['.js', '.mjs', '.css', '.html', '.json', '.svg', '.txt', '.map', '.xml']);
const MIN_BYTES = 512; // matches Caddy's encode default; smaller isn't worth a variant

const encoders = [
    ['.gz', (b) => zlib.gzipSync(b, { level: 9 })],
    ['.br', (b) => zlib.brotliCompressSync(b, {
        params: {
            [zlib.constants.BROTLI_PARAM_QUALITY]: 11,
            [zlib.constants.BROTLI_PARAM_SIZE_HINT]: b.length,
        },
    })],
];
// zstd landed in Node 22.15 / 23.8; skip quietly on older runtimes.
if (typeof zlib.zstdCompressSync === 'function') {
    encoders.push(['.zst', (b) => zlib.zstdCompressSync(b, {
        params: { [zlib.constants.ZSTD_c_compressionLevel]: 19 },
    })]);
}

let files = 0, raw = 0;
const out = Object.fromEntries(encoders.map(([ext]) => [ext, 0]));

function walk(dir, rel = '') {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
        const from = path.join(dir, e.name);
        const r = path.join(rel, e.name);
        const to = path.join(dest, r);
        if (e.isDirectory()) {
            fs.mkdirSync(to, { recursive: true });
            walk(from, r);
            continue;
        }
        fs.mkdirSync(path.dirname(to), { recursive: true });
        fs.copyFileSync(from, to);
        if (!TEXT.has(path.extname(e.name))) continue;
        const buf = fs.readFileSync(from);
        if (buf.length < MIN_BYTES) continue;
        files++;
        raw += buf.length;
        for (const [ext, enc] of encoders) {
            const z = enc(buf);
            // a variant that doesn't win would only make Caddy pick a bigger file
            if (z.length >= buf.length) continue;
            fs.writeFileSync(to + ext, z);
            out[ext] += z.length;
        }
    }
}

fs.mkdirSync(dest, { recursive: true });
walk(src, '');
const kb = (n) => (n / 1024).toFixed(0) + ' KiB';
console.log(`${files} files, ${kb(raw)} raw; ` + Object.entries(out).map(([e, n]) => `${e} ${kb(n)}`).join(', '));
