# internal/task Architecture

**Last Updated**: 2026-07-15
**Cache Level**: Module
**Expires**: 2026-07-29
**Branch**: `feat/task`
**Hash**: `f56b71c9f8d3771ece300eee598b7a4611617b61`

## Overview

`internal/task` 是一个已实现但尚未接入产品的单进程持久任务队列。它提供延时触发、按 key 串行、取消、重试和启动恢复；当前状态机面向短作业，成功后一定终结，不支持同一 Task 睡眠后再次唤醒或等待人工输入。

## Components

| Component | Location | Purpose |
|---|---|---|
| Task model | `internal/task/task.go` | 通用 Task、状态、payload/result、scheduled time |
| Manager | `internal/task/manager.go` | submit/cancel/list/start/dispatch/run 生命周期 |
| Scheduler | `internal/task/scheduler.go` | 每 5 秒查找 due pending tasks |
| Runner contracts | `internal/task/runner.go` | handler registry 与 progress controller |
| Serialization | `internal/task/cancellation.go`, `queue.go` | 同一 key 单执行者与排队 |
| Persistent store | `internal/data/db/task_store.go` | GORM `tasks` 表及部分更新 |
| Agent runtime | `agentboot/` | CLI process、stream events、approval/ask、cancel/wait |
| Claude adapter | `agentboot/claude/` | Claude CLI driver、transport、session resume |

## Current Data Flow

```text
Manager.Submit
  → TaskStore.Create(status=pending)
  → Scheduler.FindDueTasks
  → dispatchTask(serialization key)
  → Handler.Run
  → succeeded / failed / cancelled / retry pending
```

## Key Patterns

- `Payload` / `Result` 是 JSON，可先承载实验期强类型 DTO，避免立即扩表。
- `SerializationKey` 同时覆盖运行锁和 FIFO queue，适合使用 canonical workspace path。
- `UpdateStatus` 部分更新避免并发 goroutine 全量覆盖记录。
- `agentboot.ExecutionHandle` 已支持流事件、审批/问答响应、取消与最终结果。
- Claude adapter 已支持固定 `SessionID` 的创建和 `--resume`。

## Constraints / Gaps

- `agentboot.AgentType` 当前只有 Claude；Codex runtime 尚不存在，只有 `ai/agent` 中的配置支持。
- Handler 成功后 Manager 固定写 `succeeded`，无法返回 `sleeping` / `needs_input`。
- `TaskStatus.IsTerminal` 与 `Wait` 假定 interrupted 是终态。
- restart 将 running 一律标记 interrupted；没有自动按 native session 恢复。
- `Recurrence` 与 `ParentTaskID` 尚未使用，注释假设 recurring child 模型，未被生产验证。
- `internal/task.Manager` 尚未在 server lifecycle、HTTP API 或 frontend 中接线。
- 当前 `Attempt/MaxAttempts` 混合“失败重试”和“业务轮次”风险；agent wake-up iteration 不应复用 retry attempt。

## Integration Points

- `internal/data/db/store_manager.go`: 已 AutoMigrate `TaskRecord` 并暴露 `Tasks()`。
- `internal/server/server.go`: 需要创建/start/stop Task manager 并注册 agent handler。
- `agentboot/claude`: 可直接作为 Claude worker；需要新增平行 Codex driver/transport。
- `frontend/src/contexts/FeatureFlagsContext.tsx` 与 `GlobalExperimentalFeatures.tsx`: `_global/task` 实验开关。
- `frontend/src/layout/useActivityItems.tsx` / `App.tsx`: 顶层 Task 导航与路由。
- API 必须先在 backend 注册 swagger model；frontend 暂用 placeholder，等待 codegen。

## Recommended Evolution

保留 `internal/task` 作为产品 Task 的起点，修改其完成协议，使一次 handler execution 可以返回 `done`、`sleep` 或 `needs_input`。不要先创建独立 automation/run engine；执行历史在真实产品需求出现后再拆。
