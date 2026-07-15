# Runtime Port File

## Problem

The server port is intentionally **not persisted** in the config file
(`Config.ServerPort` is `json:"-"`): starting with `--port 9000` must not
silently rewrite the user's long-term configuration. The flip side is that
other CLI processes (`cc`, `profile`, `log`, `status`, `open`) resolve the
port from config and get the default (12580) — so after
`tingly-box start --port 9000`, `tingly-box profile p1` connects to the wrong
port unless the user remembers to repeat `--port 9000` every time.

## Decision

Treat the live port as a **runtime artifact, not configuration** — exactly
like the PID lock file:

- On startup, right after acquiring the PID lock, the server writes
  `<configDir>/tingly-server.port` containing the actual listening port
  (`pkg/lock.PortFile`, atomic temp-file + rename write).
- On every shutdown path (signal, web-UI stop, server error, hook failure,
  daemonize failure) the file is removed alongside the lock release.
- Readers go through `AppManager.GetRuntimeServerPort()`:
  1. explicit `--port` flag always wins (handled at each call site);
  2. if the PID lock is held (server running) and the port file is readable,
     use the recorded port;
  3. otherwise fall back to `GetServerPort()` (config / 12580 default).

## Start vs. restart

Binding a port and reading the runtime port are kept separate:

- **`start` (and a `start` on a stopped box)** resolves its port purely from
  `--port` → config → 12580. It never consults the port file, so a fresh
  start is predictable and a stale file from a crash can't resurrect an old
  port.
- **`restart`** is a *real* restart: a bare `restart` continues on the port
  the server is actually running on (read from the port file **before**
  stopping, since stop removes it), instead of silently relocating to the
  config default and breaking clients pointed at the live port. An explicit
  `--port` still wins. Restart prints the port it continues on and how to
  override it, so the otherwise-invisible sticky port is surfaced. To return
  to the default, `stop` then `start`.

### Daemon mode

`Daemonize()` backgrounds the server by re-exec'ing the process with its own
`os.Args`. A port the parent *resolved* (a preserved `restart` port, or a
config port) is not on that command line, so the detached child would
re-resolve and drift back to the default. The parent therefore pins the
resolved port by appending `--port <n>` to the child's args; the CLI parser
takes the last occurrence, so it wins without stripping any earlier value.

## Behavior matrix

| Command | Server state | Resulting port | Reads port file |
| --- | --- | --- | :---: |
| `start` | stopped | config → 12580 | no |
| `start --port N` | stopped | **N** | no |
| `start` | running | reports "already running" (shows live port) | display only |
| `restart` | running on X | **continues on X** | yes (before stop) |
| `restart --port N` | running on X | **N** | no |
| `restart` | stopped | config → 12580 | no |
| `restart --daemon` (bare) | running on X | **continues on X** | yes |
| `stop` + `start` | — | config → 12580 | no |
| `cc` / `profile` / `log` / `status` / `open` | running on X | **X** while lock held, else config | yes |
| any | stale file, not running | config → 12580 | ignored |

Invariant: an explicit `--port` always wins; binding never reads the port
file; only readers and `restart` continuation do, and only while the lock is
held.

## Staleness

A crashed server (SIGKILL) leaves the port file behind, but the flock is
always released by the OS. Readers therefore **must gate on
`FileLock.IsLocked()`** before trusting the port file; a stale file with no
lock held is ignored. The next successful start overwrites it.

## Scope

Only cross-process CLI readers use the runtime port. In-process consumers
(the server itself, TUI quickstart that starts the server in-process, GUI
wails service) already hold the correct port in the in-memory config and are
unchanged.
