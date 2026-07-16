# ImBot State Sync: Event-Driven with a Reconcile Backstop

## Problem

Bot settings live in a shared SQLite store written by two kinds of actors:

- **The server itself** (web UI API handlers) — in-process, so handlers
  already start/stop bots inline right after mutating settings.
- **CLI commands** (`remote add`, `remote pair`, …) — a *separate process*
  opening the same SQLite file directly. The running server has no way to
  observe those writes (SQLite update hooks are per-connection; watching the
  WAL file is unreliable).

Historically the server papered over this with a 30-second polling loop
(`periodicBotSync`) that called `Sync()` unconditionally — a bot added via
CLI took up to 30s to start, and the loop logged
"Periodic bot sync completed" forever.

## Decision

Kubernetes-style split: **edge-triggered for latency, level-triggered for
correctness.**

1. **CLI pokes the server after writing** (`internal/command/remote_notify.go`).
   After a CLI mutation, `notifyServerBotReload()` calls the pre-existing
   `POST /api/v1/imbot-admin/reload` endpoint, which runs the same `Sync()`.
   The live port is discovered through the runtime port file gated on the
   PID lock (see `runtime-port-file.md`) — this is exactly why the port file
   exists. The call is best-effort: server not running → nothing to notify,
   the initial sync at next startup picks the change up. The caller gets a
   bool so UX can say what actually happened ("bot started automatically"
   vs. "run `remote start …`").

2. **The polling loop stays, but as a backstop** (`background.go`):
   one immediate sync at startup, then a 5-minute reconcile. It is no longer
   the propagation path; it exists for self-healing — `Sync()` restarts
   enabled-but-not-running bots, which is also the crash-recovery mechanism —
   and for direct store edits that bypass both the API and the CLI helper.
   Removing it entirely would silently lose crash recovery.

3. **Silence when idle**: the loop no longer logs successful no-op passes;
   `bot.Manager.Sync` already logs each bot it actually starts or stops.

## Non-goals

- `Sync()` reconciles *running state vs. enabled flag* only. Config changes
  to an already-running bot (SmartGuide model, pairing) still require a
  restart (`POST /imbot-admin/restart/:uuid`); CLI paths that only change
  config intentionally do not poke reload.
- The standalone-bot path (`tingly-box remote start`) running alongside a
  server that also starts the same bot is a pre-existing hazard, unchanged
  here.
