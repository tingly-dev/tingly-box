# Task Supervision Validation Report

**Status:** PASSED
**Date:** 2026-07-16
**Criteria:** `.sdlc/docs/task-agent-automation-20260715.spec.md`

## Goal validation

1. **One bounded run per explicit step** — passed. Steps advance one at a time while reusing the task workspace and native session; every invocation has its own TaskRun.
2. **Inspectable history instead of only the last result** — passed. TaskRun stores input projection, progress, outcome, error, timestamps, execution policy, and bounded events; the UI renders the run timeline.
3. **Human intervention without killing the native process** — passed for Claude. Approval and question events persist a waiting state, remain attached to the live execution handle, accept one response, and continue the same run.
4. **Safe restart semantics** — passed. Running/waiting runs become interrupted and pending controls are cleared because native stdin cannot be reconstructed.
5. **Explicit startup authority** — passed. Claude exposes review/manual/edit profiles plus tool filtering; Codex exposes read-only/workspace-write sandbox profiles and clearly reports that per-task tool filtering is unavailable.
6. **Manual takeover artifact** — passed. Resume commands include `cd <workspace> && ...` plus the effective permission/sandbox arguments.
7. **Attention is visible at list level** — passed. Active controls are queried directly and tasks are grouped under **Needs you**.

## Active journey evidence

The mock browser journey created the same API/UI path used by production code, verified the approval request, submitted **Approve once**, observed the request disappear, and observed the existing Run return to `running` with a `control_answered` event.
