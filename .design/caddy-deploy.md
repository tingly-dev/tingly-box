# Caddy deployment

Two reference configs for a public host in front of the gateway
(`localhost:12580`). Pick one:

| File | Serves | Everything else |
| --- | --- | --- |
| `deploy/caddy/Caddyfile` | the model API: `/tingly/*`, `/mirror/*` (rewritten to `/tingly`), `/hi` | `403` |
| `deploy/caddy/Caddyfile.full` | the whole service on one host: web UI, management API and model API | proxied to the gateway |

Smoke test for both: `deploy/caddy/test.sh` (docker + python3 + curl; stub
gateway, isolated, never touches `~/.tingly-box`).

## What Caddy is for

Since the gateway serves its UI precompressed and cacheable itself
(`.design/webui-static-serving.md`, #2571), Caddy does not need to touch the
UI's assets. Its job is what the gateway does not do:

- TLS and automatic certificates, HTTP/2 and HTTP/3 (the gateway speaks plain
  HTTP/1.1, so a browser opens ~6 connections for the ~50 files of the home page);
- compressing the responses nobody precompressed: JSON from the APIs, HTML;
- long timeouts for streaming (`write 0`, `read_timeout 0`), and keeping
  streams unbuffered;
- optionally, keeping the management UI/API off the public internet.

## Decisions

| Concern | Choice | Why |
| --- | --- | --- |
| Protocols | `protocols h1 h2 h3` | h3 needs UDP 443 open as well as TCP; clients fall back to h2 without it. It is the Caddy default, spelled out so the firewall requirement is visible. |
| Compression, model API only | `encode zstd gzip`, matching `application/json*` only | Non-stream JSON compresses well. Matching the content type keeps `text/event-stream` out of the encoder by construction, so each event is flushed as produced. |
| Compression, full service | the same, matching `application/json`, `text/html`, `text/css`, `text/javascript`, `image/svg+xml` | Covers the management API, the model API's JSON and any HTML/script the gateway did not compress. A response that already has a `Content-Encoding` is left alone, which is how the gateway's own precompressed `br`/`gzip` assets go through untouched (checked in `test.sh`: encoded once, decodes to the original). Streams are simply not in the list. |
| Cache headers, ETag, 404s for `/assets` | **not set at the edge** | The gateway owns them (`immutable` hashed chunks, `no-cache` + ETag for `index.html`, 404 for unknown assets). Repeating them in Caddy would drift. An earlier iteration of this config did it (separate UI host, optional on-disk `precompressed` files, an HTML→404 guard); that was dropped once the gateway did it itself. A gateway older than #2571 therefore gets no UI caching from this config: upgrade it. |
| Requests | not compressed | Caddy cannot compress a request body, and the gateway does not decode a `Content-Encoding` on requests, so a client sending one would most likely be rejected (not tested). |
| Exposure | model-only file is default-deny; the full file publishes the management UI/API | The gateway's login protects them. `Caddyfile.full` ships a commented `@admin` block to restrict everything except `/tingly/*` to your own networks. |
| Timeouts | `read_body 30m`, `read_header 1m`, `write 0`, `idle 10m`, upstream `read_timeout 0` | Unchanged from the original file: long uploads and streams must not be cut. |

## Measurements

Caddy 2.11 (docker `caddy:2`) in front of the real gateway built from main
(isolated HOME), same frontend build, headless Chromium loading `/`
(52 requests), `Caddyfile.full` over plain HTTP:

| Link | Visit | Bytes | `load` |
| --- | --- | --- | --- |
| unthrottled | cold | 596,025 | 0.25 s |
| unthrottled | warm | 410 (51 of 52 from cache) | 0.25 s |
| 20 Mbit/s, 40 ms | cold | 596,025 | 0.73 s |
| 20 Mbit/s, 40 ms | warm | 410 | 0.32 s |
| 5 Mbit/s, 80 ms | cold | 596,025 | 1.53 s |
| 5 Mbit/s, 80 ms | warm | 410 | 0.40 s |

These match the gateway without Caddy (595,193 B cold, 394 B warm): Caddy
passes the gateway's headers and encoding through unchanged.

Over HTTPS (Caddy's internal CA, `curl --http2`): `HTTP/2 200`,
`content-encoding: br`, `cache-control: public, max-age=31536000, immutable`
and the gateway's ETag reach the client. Chromium on that self-signed
certificate did not reuse its cache on the second visit (574,302 B warm), while
the identical config over HTTP did; my reading is that Chrome does not cache
responses behind a certificate error, so that run says nothing about the
config, but I did not test it with a trusted certificate.

Not measured: HTTP/3, compression ratios of the real APIs' JSON, a real model
stream through Caddy (`test.sh` covers SSE delivery with a stub).
