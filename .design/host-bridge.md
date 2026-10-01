# Host bridge: one frontend for the browser tab and the desktop window

The same frontend build runs in a browser tab (`tb open`, team deployments,
dev) and in the Wails desktop window. Everything that differs between those
hosts goes through one interface, `HostBridge` (`frontend/src/host/types.ts`):
how the gateway is reached, the desktop's auth token, opening the main
window, opening external links, saving files. Pages never branch on the host.

## One build, chosen at runtime

`frontend/src/host/index.ts` picks the implementation once, at startup, from
the page's origin (`detect.ts`). The Wails asset server serves the window from
`wails://localhost` on macOS/Linux and `http://wails.localhost` on Windows,
also in `wails3 dev`, where it proxies to vite. `window._wails` is **not** a
signal: the shell injects it only after the page finishes loading.

The CLI and the GUI embed the same build, `internal/web/dist`
(`internal/assets.go`). The gateway serves it on its port, and the desktop
window's asset server serves it in the window (`gui/wails3/app.go`). Before
this, GUI builds filled only a GUI-only dist, so a browser opening a running
GUI's port (where `tb open` and the `tb start` banner point) got HTTP 500.

`build/Taskfile.yml`: `build:frontend:dist` is cached on the frontend
sources; `build:frontend` always copies the result into `internal/web/dist`.
The copy must stay outside the cached task. Inside it, an unchanged frontend
skipped the copy and the binary silently embedded the placeholder.

## The Wails runtime is loaded only in the desktop window

`desktop.ts` loads `@wailsio/runtime` with a dynamic import, which starts as
soon as the desktop bridge is created. A static import is never used: the
runtime registers global handlers on import. One of them suppresses the
right-click menu on ordinary elements; others capture mouse events for
drags. In a browser tab that breaks the page. The browser build keeps the
runtime in its own chunk, which is neither in the entry nor preloaded.

## Go is called by name; no generated bindings

The page calls three Go methods, `GetPort`, `GetUserAuthToken` and
`OpenMainWindow`, by fully-qualified name (`Call.ByName`) rather than through
`wails3 generate bindings`. So no frontend build needs the wails3 CLI or a
Go binding pass, and the Wails Vite plugin, which refuses to build without
generated bindings, is gone.

The price is that a rename on the Go side no longer fails to compile. Two
tests take that role:

- `desktop.contract.test.ts` checks each name in `BOUND_METHODS` against
  `gui/wails3/services/tingly_service.go`.
- `TestBoundMethods` (Go) pins the set of methods Wails binds. Wails binds
  *every* exported method of a service for any script in the window, so
  `TinglyService` exports exactly those three. What `main` needs and the page
  must not call is unexported or a field: starting the gateway, the gin
  engine, the open-window callback.

## Prefer HTTP over the Wails IPC bridge

Wails events and bound calls have proven unreliable in these WebViews
(`gui/wails3/window.go`). The shell therefore:

- navigates the main window by URL, never by event (`onShellNavigate` was
  removed: nothing had emitted its event since);
- takes the hub panel's "open main window" over `POST /api/v1/gui/open`,
  with the bound method only as fallback;
- saves files over `POST /api/v1/gui/save` (below).

These are GUI-only routes registered on the gateway's engine in
`TinglyService.ServiceStartup`, and they check the user token. A CLI server
does not have them.

## Saving files

Wails v3 wires no download handling into any WebView, so in the macOS window
an `<a download>` click does nothing. The desktop bridge POSTs the blob to
`/api/v1/gui/save?name=…`, which shows the native Save dialog and writes the
file (`gui/wails3/services/save_file.go`). A cancelled dialog is final. Only
a failed request falls back to the anchor. The suggested name is reduced to
a bare file name, so the page cannot pick the directory.

Wails ignores `SetFilename` on Linux (GTK4), so the name field opens empty
there. A name typed without an extension gets the suggested one.

## No editions

The desktop build used to be a "lite" edition that hid Remote, Desk, Skills
and the one-click config Apply from the nav, but not from the routes. The
host now decides capabilities only, never how much of the product is shown.
