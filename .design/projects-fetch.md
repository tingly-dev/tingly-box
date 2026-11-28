# projectsFetch: replaying a cross-origin body without buffering it

> Status: implemented. Scope: `frontend/src/services/projectsApi.ts` and its test.

## The problem

Projects (the embedded `@tingly/projects` package) builds same-origin URLs
under `/api/v1/projects`. `projectsFetch` is the fetch Tingly Box hands it: it
adds the control API's base URL and Bearer header.

- In a browser tab the page is served by the gateway, so those URLs are already
  correct and the request goes out as-is.
- In the desktop (Wails) window the page comes from the asset server
  (`wails://localhost` or `http://wails.localhost`), while the gateway listens
  on its own localhost port. A Request's URL cannot be rewritten, so the
  function builds a second request to `http://localhost:<port>` + path.

That second request used to copy the body with `await req.arrayBuffer()`.
For a large Library/composer upload (a `multipart/form-data` body with a
200 MB cap) this holds the whole file in the JS heap, on top of the `File`
itself — two copies of a large upload, and the browser must materialize bytes
it already has on disk.

## Why a second request at all

Not CORS preflight or retries — neither is implemented here. The only reason is
the URL move described above. The header set is preserved (`new Headers(req.headers)`
plus the auth header), as are method, signal and the error mapping (the caller
sees the gateway's `Response`).

## Options

| Option | Large body in memory | Needs | Verdict |
| --- | --- | --- | --- |
| `body: req.body` + `duplex: 'half'` | No (streamed) | Streaming request bodies: Chrome 105+, Firefox 116+, Safari 16.4+. The desktop app runs the OS webview — WebView2/Chromium on Windows, WKWebView on macOS, WebKitGTK on Linux — so older WebKit exposes no `Request.body`. A stream body also cannot be replayed across a redirect. | Not the primary path |
| Same-origin proxy route | No | A new gateway route on the asset server; backend work for a frontend concern | Not needed |
| Replay the caller's `BodyInit` | No (File/Blob streamed from disk) | Nothing beyond `fetch` | **Chosen** |
| Reject over a size threshold | Yes (until the cap) | An arbitrary client limit | Rejected |

## The chosen replay

`apiFetch` uploads call `projectsFetch(path, { method: 'POST', body: form })`
with a `FormData`. `FormData`, `Blob`/`File`, `string`, `URLSearchParams` and
buffers are *values* the browser serializes per send — constructing a Request
does not consume them, and a `File`'s bytes are streamed from disk rather than
copied. So the cross-origin branch hands the original `init.body` to the second
fetch instead of reading it back.

One trap: a copied `Content-Type` from `req.headers` carries the multipart
boundary of the *first* serialization. Re-sending the FormData re-serializes it
with a new boundary, so a copied header would not match the body (a real
`expected multipart form with field "files"` failure). The branch drops that
auto-generated header and lets `fetch` regenerate it; a caller-set
`Content-Type` is kept.

## What is still buffered

`openapi-fetch` (the typed client) hands `projectsFetch` a `Request`, not
`(url, init)`, so there is no raw body to replay. Those bodies are JSON and
small, so that path keeps `await req.arrayBuffer()`. A future caller that sends
a large body as a `Request` would buffer again; send it as `init.body` instead,
or extend this to stream with `duplex`.

## Browser differences not covered here

The boundary behaviour was checked against Node/undici (the test environment);
it was not re-run in WKWebView or WebKitGTK, where the desktop branch actually
runs. The fix relies only on `FormData` re-serialization, which every engine
supports, so it does not depend on request-body streaming.
