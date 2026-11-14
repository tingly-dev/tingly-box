# Web UI static serving (in the gateway)

How `internal/server/webui_static.go` serves the embedded web build
(`internal/web/dist`, from `pnpm build`). Behind Caddy the same rules are
repeated at the edge: `.design/caddy-deploy.md`.

## Problem

Opening the home page loads ~40 files (~2.2 MB of scripts for the current
build). The gateway used `gin.StaticFS` over `embed.FS` for `/assets` and a
gzip middleware for `index.html`:

- no `Cache-Control`, no `ETag`/`Last-Modified` (embedded files have a zero
  mtime): the browser re-downloaded **every file on every visit**;
- assets sent uncompressed (only `index.html` was gzipped);
- an unknown `/assets/old-XXXX.js` fell through to the SPA catch-all and came
  back as `index.html` with status 200 (a stale tab then dies with a script
  syntax error, and a proxy may cache the HTML under a script URL);
- `index.html` was `no-store`, so even the 3 KB entry point was never revalidated
  cheaply.

## Decisions

| Concern | Choice | Why |
| --- | --- | --- |
| Compression | `pnpm build` runs `frontend/scripts/precompress.mjs dist`, which writes `x.js.br` + `x.js.gz` (brotli 11, gzip 9) next to every compressible file ≥ 512 B. The server picks br, then gzip, then the raw file by `Accept-Encoding` (q-values honoured). | Best ratio for zero request-time CPU. Node's `zlib` has brotli; Go's standard library has no encoder, so this has to happen at build time. zstd is left out: the server serves br/gzip, every browser supports br, and zstd would add a third copy to the binary. |
| No precompressed files (`pnpm build:dev`, an older `dist`) | gzip the file on first use and keep the bytes in memory | A dev/legacy build still gets compressed transfers; the cost is paid once per file, not per request. Skipped for non-text types (png, woff2) and files < 512 B. |
| Hashed chunks | `Cache-Control: public, max-age=31536000, immutable` | Vite names them `<name>-<8 char hash>.<ext>`: the URL changes with the bytes. A visit after the first costs no asset request at all. |
| What counts as hashed | `-<8 chars>.<ext>` where the 8 chars contain a digit or an upper-case letter | Files copied verbatim from `frontend/public/assets` (`icon.svg`, `leaves.png`, …) keep their names across releases and must not be cached for a year. The digit/upper-case rule keeps an 8-letter word (`foo-barbaric.png`) from looking like a hash. Do not give a `public/` file a name that looks like a hash. |
| Everything else (`index.html`, unhashed assets) | `Cache-Control: no-cache` + `ETag` → `304 Not Modified` | `no-cache` means "revalidate before reuse": the entry point always names the current chunks, and an unchanged one costs a 304 instead of a download. (The old `no-store` forbade any reuse.) |
| Validators | strong `ETag` = first 8 bytes of sha256 of the raw file, plus `-br` / `-gzip` for the encoded copies; computed lazily, once per file | Each representation has its own tag, so a cache can never answer a gzip client's revalidation with the brotli copy. No `Last-Modified`: an embedded file has no meaningful mtime. |
| Serving | `http.ServeContent` on the embedded file | Gives `If-None-Match`/`If-Range`/`Range`/`HEAD` for free, and `embed.FS` files are `io.ReadSeeker`s, so nothing is copied or buffered per request (~23 µs and 43 allocations per 1 MB gzip hit, `BenchmarkWebAssets_Precompressed`). `Content-Type` comes from the decoded file's extension, never from `.br`/`.gz`. A `Range` request is answered from the identity bytes (ranges address the file, not its encoding). |
| Missing `/assets/*` | `404`, `Cache-Control: no-cache` | Never `index.html`. The `.br`/`.gz` files are not addressable themselves. `/assets/../…` is rejected by `fs.ValidPath`. |
| SPA routes | unchanged: `NoRoute` → `index.html`; API-looking prefixes still answer JSON 404 | Out of scope; same behaviour as before. |
| Binary size | +~3 MB (the `.br` and `.gz` copies of 5.3 MiB of text) | Measured: 177.5 MB → 180.6 MB (+1.8%). Desktop (Wails) embeds the same directory and just serves the raw files; the extra files are unused there. |

## Measurements

Real server (`tingly-box start --ui`, isolated HOME and config dir), same
frontend build, headless Chromium via CDP, home page `/`, bytes =
`encodedDataLength` of `/` plus `/assets/*` (40 requests). "Warm" is a second
navigation in the same browser context (HTTP cache kept).

| Link | | Before | After |
| --- | --- | --- | --- |
| unthrottled | cold bytes | 2,251,186 | 584,514 (−74%) |
| | warm bytes | 2,251,186 | 394 (39/40 from cache, `index.html` 304) |
| 20 Mbit/s, 40 ms | cold `load` | 1.24 s | 0.66 s |
| | warm `load` | 1.19 s | 0.37 s |
| 5 Mbit/s, 80 ms | cold `load` | 3.99 s | 1.28 s |
| | warm `load` | 3.97 s | 0.40 s |

Before, warm was exactly as expensive as cold. The remaining time after `load`
is the app's own API calls, not assets.

## Not done

- Chunking: the entry chunk is ~870 KB raw (206 KB brotli) and `mui-vendor`
  ~480 KB raw. Splitting or trimming them is the frontend bundling work, not
  serving; the precompressed numbers above already include them as built.
- HTTP/2: the gateway speaks plain HTTP/1.1, so a browser opens ~6 connections
  for the 40 files. Behind Caddy (`deploy/caddy/Caddyfile`) it gets h2/h3.
- The Wails desktop window still serves raw files through its own asset
  server; it is local, so transfer size does not matter there.
