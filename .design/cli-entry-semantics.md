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
changes are either explicitly requested or explicitly confirmed.

**Bare invocation:**

- **`npx tingly-box` / `npm exec`** (shim detects `npm_command=exec`): keeps
  the historical run-now behavior as `restart --daemon -y` — the npx
  invocation is itself the consent a bare `restart` would prompt for.
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
  up; all interactive confirmation lives in `restart`.

  The running version comes from `<configDir>/tingly-server.version`
  (`pkg/lock.VersionFile`), a runtime artifact written next to the port file
  after the PID lock is acquired and removed on every shutdown path — same
  lifecycle and reader rules as `runtime-port-file.md`. Its one job is that
  mismatch hint — without it, `tb start` after an upgrade would show a
  healthy banner while the old version silently keeps serving. A server
  started by a build predating the file reads as "unknown" and simply gets
  the generic restart hint.

**`restart`:** confirms before interrupting. When the server is running, a
bare `restart` asks ("In-flight AI requests will be interrupted. [y/N]",
default No); `-y`/`--yes` proceeds directly (what npx passes); without a TTY
and without `-y` it leaves the server untouched and says to re-run with
`-y`. When the server is not running there is nothing to interrupt, so it
starts without asking — which keeps unattended first-boots (e.g. the Docker
npx image's pm2 wrapper) working. `restart` inherits daemon-by-default.

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
