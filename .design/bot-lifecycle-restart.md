# Bot lifecycle: event-driven restart (replacing the 30s sync poll)

## Background

The imbot subsystem used to run a background goroutine, `periodicBotSync`, that
called `Manager.Sync()` every 30 seconds to reconcile *running bot goroutines*
against the `enabled=true` rows in the settings DB.

Auditing the call graph showed the poll was **not** the mechanism for the common
paths:

- **Web/API config changes** are already event-driven: create/enable/disable/
  delete call `StartBot`/`StopBot` directly (`imbot/handler.go`).
- **Server boot** starts enabled bots via `StartRemoteCoder → StartAllEnabled`
  (`server_lifecycle.go`), not via the poll.

The one thing the poll uniquely provided was **crash/disconnect recovery**:
`runBotSupervised` caught panics and, on any exit (panic, error, or the imbot SDK
giving up after its 5 reconnect attempts), just deregistered the bot — it never
restarted. The poll was the only thing that brought a still-enabled dead bot back.

## Decision

Recover the bot where it dies, instead of sweeping every 30s.

`runBotSupervised` ends in one deferred handler that, after recovering any panic
and deregistering the bot (capturing whether it was an intentional `Stop`), calls
`scheduleRestart`. A restart is scheduled only when **all** hold:

- the exit was **not** an intentional `Stop` (`runningBot.stopped`);
- the manager's `baseCtx` is **not** canceled (server not shutting down);
- the bot is **still enabled** in the store.

When eligible, it schedules a single `time.AfterFunc(restartDelay, …)` (15s) that
re-checks the same guards and calls `Start`. No attempt counter, no goroutine
bookkeeping. A persistently failing bot retries naturally: each failed restart
exits and schedules the next one, so the loop is self-sustaining at `restartDelay`
cadence — the same forever-retry the old poll had, just event-driven and per-bot.

`baseCtx` is the server lifecycle context, set once in `NewBotManager` via
`Manager.SetBaseContext`. Canceling it (shutdown) makes pending restarts skip;
`StopAll` additionally sets the `stopped` flag on running bots.

## What this deliberately does *not* do

- No per-bot exponential backoff — a flat 15s delay is enough and far simpler.
- No CLI→server reload bridge. The old poll also picked up bots added by
  `tingly-box remote add` while the daemon ran; that incidental path is dropped.
  `remote add` already instructs the user to run `remote start` (standalone), and
  web-UI toggles remain instant. Re-add a reload call only if that workflow is
  actually needed.

## Tests

`manager_restart_test.go` covers `finishRunning` (the intentional-stop decision)
and that a canceled `baseCtx` suppresses restart. Existing lifecycle tests still
assert an intentional `Stop` leaves the bot down.
