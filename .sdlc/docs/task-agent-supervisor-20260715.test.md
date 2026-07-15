# Test Report: Task Agent Supervisor Backend

**Date:** 2026-07-15
**Scope:** Task core, Claude/Codex workers, Task HTTP API, Server wiring

## Result

PASS for the implemented backend slice.

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
- Server and servertest package compile.

## Existing unrelated repository failure

A broad `go test ./internal/... -run '^$'` was also attempted earlier. It is blocked by pre-existing mocks in `internal/remote_control/smart_guide/agent_integration_test.go` that still implement the old `HandleAnthropicStream` signature without `context.Context`. No Task code touches that package.
