# Start confirms the server is reachable before printing the URL

`start`, `restart`, `open` (when it has to start the server) and the TUI
quickstart never show the Web UI URL / token until the server answers
`GET /api/v1/info/health` (200). A held lock or a forked child proves nothing.

- **Background (default)**: the launching process takes no lock. It spawns the
  detached child (`daemon.Spawn`, which returns the child's exit channel),
  polls the health endpoint, then prints the banner. The child takes the
  single-instance lock itself and, being detached, logs its own startup error
  to the log file.
- **Foreground (`--no-daemon`)**: the server runs in-process; the banner is
  printed from a goroutine once the probe succeeds.
- **Already running**: `start` probes first too — the lock is held before the
  listener is up.
- **Failure**: child exit or `startReadyTimeout` (30s) → non-zero exit, error
  names the log path; no URL is printed. In the foreground a timeout stops the
  server and releases the lock.
- `server.Start` itself prints the URL / API endpoints only after its port
  is bound (it used to print them before `ListenAndServe`).

Code: `internal/command/readiness` (probe), `internal/command/confirm_start.go`
(`confirmStarted`), `launchDaemon` in `internal/command/server.go`.
