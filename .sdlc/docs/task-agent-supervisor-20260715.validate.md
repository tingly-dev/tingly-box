# Validation Report: Task Agent Supervisor

**Date:** 2026-07-15
**Spec:** `task-agent-automation-20260715.spec.md`

## Scope status

- Task core: implemented.
- Claude vertical slice: implemented.
- Codex driver/transport: implemented.
- Swagger API and Server lifecycle: implemented.
- Frontend experimental flag and Task page: implemented.
- Sequential steps: implemented for ordered, same-agent, same-workspace execution.
- Real CLI end-to-end harness: pending.

## Architecture checks

- Reuses `internal/task`; no Automation/Run/Engine Task hierarchy added.
- Reuses the single Task row for cron and supervisor wake-ups.
- Stores ordered steps, cursor and completed outcomes in the versioned agent Payload; no child Task or Run table added.
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

## UX-principle checks for the frontend

- Tasks is a standalone top-level activity rather than being hidden under an implementation-oriented section.
- The list is grouped around user questions: needs attention, active/scheduled and completed.
- Scheduling and continuous follow-up are separate controls; there is no combined execution-mode picker.
- Steps are added inline below the goal; there is no simple/sequential mode picker.
- Detail answers which steps are done, which one is current and what comes next before showing execution metadata.
- The creation dialog starts with the goal and uses available agents as smart defaults.
- The detail surface shows concrete workspace, session and resume command values.
- The primary action is sending an instruction; stop and run-now remain visible secondary actions.
- Disabling the experiment hides creation/navigation without locking existing Task URLs or cleanup actions.
- Task visibility is persisted as the global experimental extension `extensions.task`; it is not registered as a scenario protocol flag.
- Desktop page and creation dialog were visually verified at 1440×900 in the mock runtime.

## Deferred by design

- Opening workspace and native take-over shell actions.
- Full execution history and Run table.
- Page-level one-off approval UI.
- Real credentialed Claude/Codex end-to-end tests.
- DAG, branching, parallel steps and per-step execution settings.
