# Validation Report: Task Agent Supervisor Backend

**Date:** 2026-07-15
**Spec:** `task-agent-automation-20260715.spec.md`

## Scope status

- Task core: implemented.
- Claude vertical slice: implemented.
- Codex driver/transport: implemented.
- Swagger API and Server lifecycle: implemented.
- Frontend experimental flag and Task page: pending.
- Real CLI end-to-end harness: pending.

## Architecture checks

- Reuses `internal/task`; no Automation/Run/Engine Task hierarchy added.
- Reuses the single Task row for cron and supervisor wake-ups.
- TB owns configuration, scheduling, workspace and lifecycle.
- Claude/Codex own their native session stores; TB stores only the session ID.
- Stable workspace is generated before Submit and is also the serialization key.
- Claude execution routes through the existing TB gateway env builder.
- Codex uses its native persisted config/session and never enables a dangerous bypass flag.
- Stop occurs before database shutdown.

## UX-principle checks for the API contract

- Create does not expose workspace or session controls.
- Scheduling and continuous follow-up remain separate fields.
- Detail exposes concrete workspace, session, next wake-up and native resume command.
- Completed tasks remain wakeable.
- `needs_input` remains actionable and receives a concrete instruction through the same Task.

## Deferred by design

- Task page/navigation/experimental UI flag.
- Opening workspace and native take-over shell actions.
- Full execution history and Run table.
- Page-level one-off approval UI.
- Real credentialed Claude/Codex end-to-end tests.
