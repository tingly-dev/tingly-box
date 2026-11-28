# Start readiness: a state machine, not a 30s guess

`start`, `restart`, `open` (when it has to start the server) and the TUI
quickstart never show the Web UI URL / token until the server answers
`GET /api/v1/info/health` (200) **for the instance they launched**. A held lock
or a forked child proves nothing.

## State machine

```
IDLE ──launch──▶ STARTING ──health 200 (this instance)──▶ READY
                    │
                    ├── process exited ──▶ FAILED          (fast, exit code + log tail)
                    ├── total timeout, process alive ──▶ STILL STARTING
                    └── ctx cancelled ──▶ CANCELED
```

`restart` prepends `STOPPING`: the old server is SIGTERM'd and must release its
lock within `stopGracePeriod` (10s); otherwise it is SIGKILL'd and must release
within `stopForcePeriod` (5s). If it still holds the lock, `restart` fails with
an explicit error rather than starting a second server over a half-dead one.
Only after the old process is gone does the new one start.

Every observation during `STARTING` is classified, so a timeout says *why*:

- `port unreachable` — nothing accepted a TCP connection;
- `port reachable, not ready` — connected, but the health endpoint was not 200
  (quoted status code);
- `answered by another instance` — 200, but the instance token did not match
  (an older server still shutting down, or an unrelated process on the port).

## Instance identity

Each launch generates a random token. A foreground server gets it through
`server.WithInstanceToken`; a daemon parent passes it to the re-exec'd child in
`_TINGLY_BOX_INSTANCE_TOKEN`. The health endpoint echoes it in
`X-Tingly-Instance` (and always sets `X-Tingly-Service: tingly-box`). The
launcher only accepts a 200 carrying **its** token — so `restart` cannot
mistake a still-dying old server answering on the same port for the new one.
Probing an already-running server (`start` when the lock is held) has no token
and accepts any tingly-box 200; a bare 200 from a pre-header build is still
accepted, but an explicit foreign `X-Tingly-Service` is rejected.

## Timeouts and options

| concern | default | override |
| --- | --- | --- |
| total readiness wait | 60s | `--start-timeout`, `$TINGLY_BOX_START_TIMEOUT` |
| keep waiting past it | off | `--wait` |
| poll interval | 100ms | — |
| per-request timeout | 1s | — |
| graceful stop | 10s then SIGKILL | — |
| port re-bind wait | 3s | — |

`--start-timeout` replaced the old single hard-coded 30s: first start runs
migrations and brings up remote bots, and a slow-but-healthy start must not be
reported as a failure.

## Failure reporting

- **Exited** → fails immediately (no waiting out the timeout) with the exit
  code and the last 20 lines of the log.
- **Timed out while alive** → *not* phrased as a failure: "the server is still
  starting (background process N is still running)…", with the log path and
  the next actions (`--wait`, `tingly-box log`, `tingly-box stop`). The daemon
  child is left running, because it may still finish on its own. `--wait`
  keeps waiting (until ready, process exit, or Ctrl+C).
- **Port taken by someone else** → `start` says so instead of "port in use".
- **restart after a successful stop** → if the new server fails, the error says
  plainly that the previous server is gone and no server is running now.

## Probes never use a proxy

The readiness HTTP client uses a transport with `Proxy: nil`. The default
`http.Client` honours `HTTP_PROXY`/`HTTPS_PROXY`; for a non-loopback `--host`
that could route the local health probe through a proxy and make a healthy
server look dead (or a proxy error look like a crash).

## Early bind failure

`server.Start` polls the listener *and* `ListenAndServe`'s error channel, so a
bind failure (port taken, unbindable host) returns at once instead of after the
2s poll ceiling. The daemon child then exits, and the launcher's exit watch
reports it.

Code: `internal/command/readiness` (state machine + probe),
`internal/command/confirm_start.go` (`startWait.confirm`), `launchDaemon` and
`stopServerWithFileLock` in `internal/command/server*.go`,
`internal/server/module/info` (health headers). Tests:
`internal/command/readiness/readiness_test.go`,
`internal/command/confirm_start_test.go`,
`internal/command/stop_server_test.go`.
