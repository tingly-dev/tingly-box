# CLI entry semantics: npx vs installed CLI, daemon default

Decided 2026-08, alongside re-enabling `npm install -g` (see `npm.md`).

## Problem

The npm shims historically ran `restart --daemon` when invoked with no
arguments. That default was designed for `npx tingly-box@latest`, where the
invocation itself expresses "run (the version I just fetched) now". Once
`npm install -g` became viable again, the same default made a casually typed
`tingly-box` silently restart a running server — killing in-flight AI
requests, which for an LLM gateway can be minutes-long streams.

## Decision

Split the entry semantics by how the process was invoked; server lifecycle
changes are always explicitly requested — by a lifecycle verb (`start`,
`stop`, `restart`) or by a bare `npx` invocation, never by a casual run of an
installed bin.

**Bare invocation:**

- **`npx tingly-box` / `npm exec`** (shim detects `npm_command=exec`): keeps
  the historical run-now behavior as `restart --daemon` (the shim still
  passes `-y`, now a hidden no-op kept for compatibility).
- **Installed CLI** (global `npm install -g` bin run directly, or the raw Go
  binary): shows **help**. An installed CLI is a toolbox (like `git`,
  `docker`); the server is started deliberately with `tingly-box start`.
  Implemented twice so both layers agree: the shims pass `--help` when not
  under npx, and `cli/tingly-box/main.go` maps zero args to `--help`.

**`--source` records the channel truthfully, handling stays unified:** the
shims report `npx` / `npx-bundle` under npx and `npm` / `npm-bundle` when run
as an installed bin (`internal/shortcut.npmShimSource` groups all four). The
only consumer that cares about the split is shortcut generation, which keeps
relaunching via a version-pinned `npx -y <package>@<ver>` for every npm shim
source — a global install's own exePath sits in the version-tagged download
cache that a later update orphans, and npm's cache still holds the installed
tarball so the npx relaunch works offline. The bundle variants exist so a
`tingly-box-bundle` install's shortcut relaunches that package, not the cli
one; the bundle package is retired since the cli package ships its binary
through npm (`npm.md`, F), but installs made from it still report these
sources and their shortcuts keep relaunching the pinned bundle version.

Both bins are always shipped (`tingly-box` and `tb`), so command hints print
both forms (e.g. `'tingly-box restart' / 'tb restart'`) rather than guessing
which one the user typed.

**`start`:**

- Daemonizes **by default** (`--no-daemon` for foreground). "Start the
  server" is service semantics; the terminal is handed back with the access
  banner. Foreground stays one flag away for debugging.
- Containers pass `--no-daemon` explicitly (see `build/docker/*.Dockerfile`)
  — daemonizing would exit PID 1 and kill the container. This is deliberate
  configuration at the call site, not runtime environment sniffing.
- When the server is **already running**, `start` never restarts it and
  never asks — it prints the access banner (Web UI URL + token, API
  endpoints — the thing the user actually came for) and exits. If the
  recorded server version differs from this launcher (typical right after
  `npm install -g`), one extra hint line says so and points to
  `tingly-box restart`. `start` is purely informational when the server is
  up; interrupting a running server is what `restart` / `stop` are for.

  The running version comes from `<configDir>/tingly-server.version`
  (`pkg/lock.VersionFile`), a runtime artifact written next to the port file
  after the PID lock is acquired and removed on every shutdown path — same
  lifecycle and reader rules as `runtime-port-file.md`. Its one job is that
  mismatch hint — without it, `tb start` after an upgrade would show a
  healthy banner while the old version silently keeps serving. A server
  started by a build predating the file reads as "unknown" and simply gets
  the generic restart hint.

**`restart`:** acts immediately, no second confirmation. Typing the verb is
already the intent — exactly like `stop`, which has never asked — and a
`[y/N]` prompt on top of it was inconsistent (`stop` kills in-flight requests
without asking) and broke unattended use (without a TTY the old code refused
to act and told the user to re-run with `-y`). The in-flight-requests cost
is documented instead of gated. `-y`/`--yes` is still accepted as a hidden
no-op so existing invocations (the npx shim, the Docker npx image's pm2
wrapper, user scripts) keep working. When the server is not running,
`restart` simply starts it. `restart` inherits daemon-by-default.

**`stop`:** remains the explicit, immediate lifecycle verb.

**Shortcuts** (desktop / start menu) launch `open` instead of the former
`restart --daemon`: a double-click means "give me Tingly Box" — open the web
UI, starting the server only if needed. The old restart target would now
prompt in a popup terminal, and npx shortcuts are pinned to one version so
their restart carried no update semantics anyway.

## Explicitly out of scope (for now)

- Graceful drain (waiting for in-flight requests before restarting) and an
  in-flight request counter. The current guard is consent, not draining;
  `ServerManager.StopTimeout` is still short.
- `tb update` (npm.md plan C) — once it lands, it becomes the primary update
  verb and the npx restart default matters less.

## UX principles applied

- Smart defaults over toggles: daemon default for a service verb; container
  fallback instead of a flag every image must know.
- Scope side effects to the current surface: a bare command never restarts;
  only npx (where invocation = intent) keeps the run-now default.
- Surface the artifact for the next action: `start` on a running same-version
  server prints the access banner instead of "already running".

## Process boundary: a CLI process is a database client

Added 2026-10, from #1912.

`cli/tingly-box/main.go` used to build the full `AppManager` for **every**
subcommand before dispatching. Building one is not free of side effects:
`appconfig.NewAppConfig` creates the config tree, opens `tingly.db` (SQLite,
WAL) through `db.NewStoreManager` and runs every store's `AutoMigrate`, plus
the deprecated-table drop, legacy-JSON imports and config migrations. So
`tingly-box version` — Docker's `HEALTHCHECK`, every 30s — and
`tingly-box mcp-builtin` — a stdio child the *running server itself* spawns
— were each a second process writing to the server's database, just by
being dispatched. On a Docker Desktop bind mount, where SQLite's
cross-process locks and WAL shared memory are not honoured, that corrupted a
user's `usage_records` right after an upgrade, when both processes were also
migrating schema. Nothing in `internal/db` had changed between the releases.

### Decision

`AppManager` is **lazy** (`app.NewLazyAppManager`). It resolves the config
directory up front and builds `AppConfig` on the first call to `AppConfig()`
/ `GetGlobalConfig()`. A command that never asks never opens the database;
there is no allowlist of "config-free commands" to keep in sync — the
dependency is expressed by what each `Run` actually calls. Two accessors
are guaranteed never to build it:

- `ConfigDir()` — the lock file, port file and log paths live here.
- `GetRuntimeServerPort()` — the lock + runtime port file, falling back to
  `constant.DefaultServerPort` (the port is not persisted in `config.json`,
  so there is nothing to load).

`appconfig.UserTokenFromFile(configDir)` reads the one value a
talk-to-the-running-server command needs from `config.json` without a
`Config`.

Per command, as of this note:

| Command | Needs | Opens DB? |
|---|---|---|
| `version`, `shortcut` | nothing (`Run()` takes no `AppManager`) | never |
| `mcp-builtin` | nothing (`Run()` takes no `AppManager`); spawned by the server | never |
| `stop` | lock file | never |
| `log` | lock, port file, user token from `config.json`, HTTP | never |
| `open` on a running server | lock, port file, user token, browser | never |
| `open` on a stopped server, `start`, `restart` | the server itself | yes — it *is* the server |
| `status`, `provider …`, `rule …`, `agent …`, `token …`, `quota …`, `oauth`, `cc`, `profile`, `remote …`, `tui`, `swagger` | providers / rules / profiles | yes — they are the toolbox over the data |

`TestCommandsThatMustNotOpenTheDatabase` (`cli/tingly-box/main_test.go`)
runs the never-rows through the same `AppManager` `main()` builds against a
config dir that does not exist and asserts it still does not. A `Run`
method that grows an `*app.AppManager` parameter it does not need fails
that test rather than a container's health check.

### The GUI binary follows the same boundary

`gui/wails3` starts the same gateway, so it uses the same rules: a lazy
`AppManager`, and the single-instance lock taken from `ConfigDir()` *before*
anything builds `AppConfig`. A second launch that only asks the running
instance to show its window (`notifyRunningGUI`) reads the runtime port file
and `UserTokenFromFile`, like `open` on a running server, and never opens
the database. Its server flags are the CLI's own `command.ServerFlagsKong`
(also embedded in `StartCmdKong`), and `AppLauncher.Start` receives those
flags unresolved, because resolving them is what builds `AppConfig`. The
one deliberate difference is an empty `--host`: the CLI binds every
interface (Docker relies on it), the GUI pins it to `localhost`.

### What this does not solve

The yes-rows are correct on any filesystem SQLite supports. Against a live
server on a Docker Desktop bind mount they carry the risk the health check
did, which `docs/docker.md` now says; the container images probe
`/api/v1/info/health` over HTTP instead of running a subcommand. `status`
on a running server could be served entirely over HTTP too and stay out of
the database — not done here, since it also lists providers and rules and
would need an API round-trip for each; worth doing if a second report like
#1912 names it.

### UX principles applied

- Diagnostics must traverse the real path: a health check asks the server,
  not a sibling process that happens to share its files.
- Scope side effects to the current surface: printing a version or stopping
  a server touches nothing it does not need.
