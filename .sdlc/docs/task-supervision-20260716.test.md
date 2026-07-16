# Task Supervision Test Report

**Status:** PASSED with repository baseline limitation
**Date:** 2026-07-16
**Scope:** Task execution policies, durable runs, live controls, and Task UI

## Automated checks

- `go test ./agentboot/... ./internal/task/... ./internal/data/db ./internal/server/module/task` — passed.
- `go test -race ./internal/task/... ./internal/server/module/task` — passed.
- `go vet ./agentboot/... ./internal/task/... ./internal/data/db ./internal/server/module/task ./internal/server` — passed.
- `git diff --check 543b7bb14^..HEAD` — passed.
- Targeted Oxlint for the four changed frontend files — passed.
- `pnpm build:dev` — passed (8,784 modules transformed).
- Mock browser journey — passed with no page or console errors:
  - pending approval is surfaced under **Needs you**;
  - Claude approval shows the concrete tool input;
  - **Approve once** clears the request and resumes the same run;
  - switching task creation to Codex replaces tool filtering with sandbox guidance.

## Focused coverage

- Claude and Codex launch-profile argument mapping.
- Sequential step run boundaries and session reuse.
- live ask/approval delivery and one-shot decisions.
- sensitive control-input field redaction.
- TaskRun persistence, status filtering, event history, and restart interruption.
- Task API run history, active attention, stale-control conflict, create/wake validation.

## Baseline limitation

`pnpm typecheck` is currently red because of pre-existing errors outside the Task feature (including TokenHistoryChart, LogExplorer, rule-card tests, provider hooks, and generated API consumers). The command reported no errors in `pages/task`, `services/taskApi.ts`, or the Task mock handlers. Coverage instrumentation was not run.
