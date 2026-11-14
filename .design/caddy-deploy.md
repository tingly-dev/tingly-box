# Caddy deployment: transfer cost

`deploy/caddy/Caddyfile` is the reference reverse-proxy config for a public
host. This note records why it looks the way it does. Smoke test:
`deploy/caddy/test.sh` (docker + python3; stub gateway, isolated, never
touches `~/.tingly-box`).

## What the gateway does by itself

`internal/server/webui_static.go` serves the embedded UI:

- `/assets/*` via `gin.StaticFS` over `embed.FS`: no `Cache-Control`, no
  `ETag`/`Last-Modified` (embedded files have a zero mtime), not compressed.
  So every page load re-downloads every chunk, in full.
- everything else that is not an API prefix gets `index.html`, **status 200**,
  `no-store` (correct: it names the current chunks), gzip-compressed.

The old Caddyfile only proxied `/tingly/*` and answered 403 for the rest, so the
UI was not reachable through it at all.

## Decisions

| Concern | Choice | Why |
| --- | --- | --- |
| Protocols | `protocols h1 h2 h3` explicit | h2 multiplexes the ~40 chunks of a first load; h3 needs UDP 443 open, clients fall back to h2 without it. Already the Caddy default; spelled out so the firewall requirement is visible. |
| Compression, model API (`/tingly`, `/mirror`) | `encode zstd gzip`, **only** `Content-Type: application/json*` | Non-stream JSON compresses well. Matching on the JSON content type rather than Caddy's default list keeps streams (`text/event-stream`) out of the encoder by construction; the test asserts events still arrive as produced and are not encoded. |
| Compression, UI host | `encode zstd gzip` (default types) | Fallback for whatever is not precompressed. Caddy ships no brotli *encoder*; brotli is only available through precompressed files. |
| Hashed assets | `Cache-Control: public, max-age=31536000, immutable` | Vite emits `<name>-<8 char hash>.<ext>`; the URL changes when the bytes do. A repeat visit then costs only `index.html` (~1 KB compressed). |
| Unhashed assets (`icon.svg`, `leaves.png`, from `frontend/public/assets`) | `max-age=3600` | Same name across releases, so not immutable; they are tiny. |
| `index.html` | left to the gateway (`no-cache, no-store`) | It is the pointer to the current chunks. |
| Cache headers only on 2xx | `header … { match status 2xx }` | A cached error would stick for a year. |
| Missing chunk | `handle_response` turns a `text/html` answer on `/assets/*` into 404 | The gateway's SPA fallback answers an unknown `/assets/old-XXXX.js` with `index.html` and 200. A stale tab, or a rollback, would otherwise cache HTML under a script URL as immutable. |
| Precompressed assets | `file_server { precompressed br zstd gzip; pass_thru }` from `TINGLY_WEB_ROOT` (default `/srv/tingly-web`), then fall through to the gateway | Max-level brotli/zstd beats on-the-fly gzip by ~25% on the largest chunk and costs no CPU per request; `file_server` also adds `ETag`/`Last-Modified`, so revalidation is a 304. `pass_thru` makes the directory optional: with no build deployed, the gateway's embedded copy is proxied and compressed on the fly, so the config works either way. Brotli is listed first because it was smaller than zstd-19 on this bundle and is supported by every browser. |
| Producing the variants | `pnpm build` (once the gateway change in `.design/webui-static-serving.md` is in) writes `.br` + `.gz` into `frontend/dist`; for older builds or for `.zst`, compress with any tool (loop below) | Caddy only serves what is on disk. |
| Exposure | UI is a **separate host block** (`ui.you.domain`) | The old config deliberately 403'd everything but the model API. Publishing the admin UI/API is an operator decision, so it is its own block: delete it (or leave its DNS unset) to keep the UI local-only. |
| ETag | not added in proxy mode | The gateway sends none, and with `immutable` the browser never revalidates hashed files. Validators come for free from `file_server` in precompressed mode. |

## Deploy recipe

```bash
task web:dist                                       # builds frontend/dist and embeds it
cp -R frontend/dist/. /srv/tingly-web/        # already holds .br/.gz when built with the gateway change
# otherwise, or to add zstd:
find /srv/tingly-web -type f \( -name '*.js' -o -name '*.css' -o -name '*.svg' \) -size +512c \
  -exec sh -c 'gzip -9 -k -f "$1"; brotli -q 11 -k -f "$1"; zstd -19 -q -f "$1"' _ {} \;
```

Keep the previous release's `assets/` in `/srv/tingly-web` for a while (copy,
don't wipe) so tabs opened before a release can still load their lazy chunks;
the gateway only has the newest set.

## Measurements

Local Caddy 2.11 (docker `caddy:2`), stub gateway reproducing
`webui_static.go` (gin `StaticFS` over `embed.FS`) serving a real
`pnpm build` of the frontend (5.3 MiB raw, 39 files on a cold load of `/`),
`Accept-Encoding: gzip, deflate, br, zstd`.

| | cold load (bytes) | repeat visit |
| --- | --- | --- |
| Before (plain `reverse_proxy` to the gateway; the shipped Caddyfile 403s the UI) | 2,190,702 | 2.19 MB again: no validators, no cache headers |
| After, proxy only (zstd on the fly) | 661,896 (−70%) | `index.html` only (~0.9 KB) |
| After, precompressed (`br`) | 516,281 (−76%) | `index.html` only; revalidation = 304 |

Largest chunk (`index-*.js`, 874,141 B): identity 874,141 → gzip 276,871 (on the
fly) → zstd 216,671 → br 206,591 (precompressed).
With `Accept-Encoding: gzip` only: 668,695 on the fly, 640,207 precompressed.

Headers seen after the change (`/assets/index-BJ-7R1vk.js`):
`Cache-Control: public, max-age=31536000, immutable`, `Content-Encoding: br`,
`Etag`, `Vary: Accept-Encoding`; a conditional request returns `304`.
A missing chunk returns `404` with no cache header. SSE on `/tingly/*` is
delivered per event, uncompressed.

## Not done / follow-ups

- The gateway itself should do the same for `/assets` (precompressed files,
  immutable hashed names, ETag/304, 404 for unknown `/assets/*`) so that
  deployments without Caddy benefit too: `.design/webui-static-serving.md`
  (separate change). With it, the `/assets` rules here are redundant but
  harmless; until then they are what fixes a Caddy deployment.
- `Content-Security-Policy` etc. are not touched.
