# Bot lifecycle: event-driven restart (replacing the 30s sync poll)

## Background

The imbot subsystem used to run a background goroutine, `periodicBotSync`, that
called `Manager.Sync()` every 30 seconds to reconcile *running bot goroutines*
against the `enabled=true` rows in the settings DB. It started enabled-but-not-
running bots and stopped running-but-disabled ones.

Auditing the call graph showed the poll was **not** the mechanism for the common
paths:

- **Web/API config changes** are already event-driven: create/enable/disable/
  delete call `StartBot`/`StopBot` directly (`imbot/handler.go`).
- **Server boot** starts enabled bots via `StartRemoteCoder → StartAllEnabled`
  (`server_lifecycle.go`), not via the poll's initial sync.

So the 30s poll uniquely covered only two things:

1. **Crash / disconnect recovery** — `runBotSupervised` catches panics and, on
   any exit (panic, error, or the imbot SDK giving up after its 5 reconnect
   attempts), just deregistered the bot. It never restarted. The poll was the
   *only* thing that brought a still-enabled but dead bot back, with up to 30s of
   downtime.
2. **Out-of-process DB edits** — e.g. `tingly-box remote add` (a separate CLI
   process) writing `enabled=true` to the shared DB while the server runs.

## Decision

Replace the poll with **event-driven restart-on-crash + backoff**, and make the
CLI case event-driven too, then delete the loop.

### 1. Restart-on-crash (internal/remote_control/bot/manager.go)

`runBotSupervised` now ends in a single deferred exit handler that, after
recovering any panic and deregistering the bot, calls `afterBotExit`. A restart
is scheduled only when **all** of the following hold:

- the exit was **not** an intentional `Stop()` (tracked via `runningBot.stopped`,
  captured in `finishRunning`);
- the manager's `baseCtx` is **not** canceled (the remote-control service is not
  stopping/shutting down);
- the bot is **still enabled** in the store (`isEnabledInStore`).

Backoff (`nextRestartDelay`): exponential from `restartBaseDelay` (3s), doubling,
capped at `restartMaxDelay` (60s) — i.e. 3, 6, 12, 24, 48, 60, 60, … A bot that
ran healthily for at least `restartHealthyRun` (60s) before dying resets to the
base delay, so a long-lived bot that drops its connection recovers promptly
instead of inheriting an old crash-loop backoff. The steady-state 60s retry of a
permanently-failing enabled bot mirrors the old poll's forever-retry semantics.

`restartAfter` waits the delay (or aborts on `baseCtx.Done()`), then calls
`Start`. If `Start` itself fails — so no supervised goroutine exists to drive the
next attempt — it reschedules from there, so recovery never stalls on a transient
startup error.

`baseCtx` is wired to the remote-control service context via
`SetBaseContext`, called from `StartRemoteCoder`. Cancelling it (stop / shutdown)
abandons pending restarts instead of resurrecting bots after stop. `StopAll`
additionally sets the `stopped` flag, so both guards cover shutdown.

### 2. CLI out-of-process edits (internal/command/remote_add.go)

After `remote add` writes a new enabled bot to the DB, it makes a best-effort
`POST /api/v1/imbot-admin/reload` to a locally running server so the bot starts in
the server process immediately. If no server is reachable the call is silently
ignored — standalone `remote start` still works fully offline.

## Why this is better

- **No fixed-interval polling**; recovery is immediate (after backoff) instead of
  up to 30s late.
- Resilience is **decoupled from a timer** and lives next to the lifecycle it
  guards.
- Boot, web toggles, and CLI all remain covered; behaviour parity with the old
  poll is preserved (forever-retry of a failing enabled bot, no restart of
  intentionally-stopped/disabled bots).

## Tests

`manager_restart_test.go` locks in the backoff schedule, the healthy-run reset,
and `clearRestartAttempts`. Existing lifecycle tests continue to assert that an
intentional `Stop` leaves the bot down.
