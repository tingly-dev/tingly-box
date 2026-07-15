# Test Report: Task Agent Supervisor

**Date:** 2026-07-15
**Scope:** Task core, Claude/Codex workers, Task HTTP API, Server wiring, experimental frontend

## Result

PASS for the implemented backend and frontend slices.

## Commands

```text
cd agentboot
GOCACHE=/tmp/tingly-box-bot-go-cache go test ./... -count=1

cd ..
GOCACHE=/tmp/tingly-box-bot-go-cache go test ./internal/task/... ./internal/server/module/task -count=1 -race
GOCACHE=/tmp/tingly-box-bot-go-cache go test ./internal/data/db -run TestTaskStore_RoundTripSupervisorFields -count=1 -race
GOCACHE=/tmp/tingly-box-bot-go-cache go test ./internal/server ./internal/servertest -run '^$' -count=1
GOCACHE=/tmp/tingly-box-bot-go-cache go vet ./internal/task/... ./internal/server/module/task
cd agentboot && GOCACHE=/tmp/tingly-box-bot-go-cache go vet ./codex
git diff --check

cd frontend
pnpm exec oxlint src/pages/task/TaskPage.tsx src/pages/task/types.ts src/services/taskApi.ts src/services/experimentalExtensions.ts src/contexts/FeatureFlagsContext.tsx src/components/GlobalExperimentalFeatures.tsx src/layout/useActivityItems.tsx src/App.tsx src/mocks/handlers.ts
pnpm build:dev
```

All commands passed.

## Covered behavior

- explicit complete/reschedule/needs-input transitions;
- retry-attempt reset and same-row wake-up;
- Wake/Cancel conflicts and needs-input cancellation;
- five-field cron, timezone handling, invalid recurrence and same-row recurrence;
- DB round-trip for supervisor payload/result/recurrence/timestamps;
- stable canonical private workspace generation;
- Claude create/resume, outcome normalization and pause on ask/approval;
- Codex safe CLI arguments, resume arguments, JSONL parsing, thread-ID capture and final text;
- immediate Codex native-session checkpoint from thread.started;
- API creation, unsupported-agent validation and paused-task instruction wake-up;
- sequential step normalization, one-run-at-a-time prompts, durable outcome checkpoints, automatic advancement and completed-sequence restart;
- `continue` without follow-up pauses the current step instead of silently completing the sequence;
- Server and servertest package compile;
- experimental flag, standalone navigation, grouped task list and actionable task detail;
- immediate/later/recurring creation flows and orthogonal continuous follow-up controls;
- mock API flows for empty/loading/error-free visual development;
- 1440×1000 browser screenshots for sequential detail and the two-step creation dialog;
- browser assertion confirms Create sends only ordered `steps[].instruction` inputs and renders normalized steps after creation;
- regression interaction verifies Task reads and writes `_global.extensions.task`, never `/flag/task`, and immediately updates navigation in both directions.

## Existing unrelated repository failure

A broad `go test ./internal/... -run '^$'` was also attempted earlier. It is blocked by pre-existing mocks in `internal/remote_control/smart_guide/agent_integration_test.go` that still implement the old `HandleAnthropicStream` signature without `context.Context`. No Task code touches that package.

The broad frontend `pnpm typecheck` is also blocked by existing generated-client and test-global errors outside the Task feature. The targeted Task lint and production-style development build pass.
