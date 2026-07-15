# internal/task Architecture

**Last Updated**: 2026-07-15
**Cache Level**: Module
**Expires**: 2026-07-29
**Branch**: `feat/task`
**Hash**: `f56b71c9f8d3771ece300eee598b7a4611617b61`

## Overview

`internal/task` 已演进并接入实验性 Task 产品面。它提供延时/重复触发、按 workspace 串行、取消、重试、显式 reschedule、等待人工输入和启动恢复；agent Task 在同一 workspace/native session 中执行 bounded runs，并可通过 Payload 游标顺序推进多个显式步骤。

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
  → Agent Handler.Run(current step or free goal)
  → complete / reschedule / needs_input / failed / cancelled
  → checkpoint native session + optional step cursor/outcome
```

## Key Patterns

- `Payload` / `Result` 是 JSON，可先承载实验期强类型 DTO，避免立即扩表。
- `SerializationKey` 同时覆盖运行锁和 FIFO queue，适合使用 canonical workspace path。
- `UpdateStatus` 部分更新避免并发 goroutine 全量覆盖记录。
- `agentboot.ExecutionHandle` 已支持流事件、审批/问答响应、取消与最终结果。
- Claude adapter 已支持固定 `SessionID` 的创建和 `--resume`。
- 顺序步骤存放在版本化 agent Payload；`done` 原子 checkpoint outcome 并推进游标，下一步骤复用同一 Task 行。

## Constraints / Gaps

- restart 仍将 running 标记 interrupted；用户通过 Run now 从持久化 session/workspace/step cursor 恢复。
- 没有独立 Run 表或完整执行历史；步骤只保存完成结果，最近一轮仍使用 Task.Result。
- 步骤仅支持顺序推进；没有 DAG、分支、并行或步骤级 agent/schedule/retry。
- native session 丢失时明确失败，不自动创建替代 session。

## Integration Points

- `internal/data/db/store_manager.go`: 已 AutoMigrate `TaskRecord` 并暴露 `Tasks()`。
- `internal/server/server.go`: 需要创建/start/stop Task manager 并注册 agent handler。
- `agentboot/claude`: 可直接作为 Claude worker；需要新增平行 Codex driver/transport。
- `frontend/src/contexts/FeatureFlagsContext.tsx` 与 `GlobalExperimentalFeatures.tsx`: `_global.extensions.task` 实验扩展开关；不是 scenario flag registry 成员。
- `frontend/src/layout/useActivityItems.tsx` / `App.tsx`: 顶层 Task 导航与路由。
- API 必须先在 backend 注册 swagger model；frontend 暂用 placeholder，等待 codegen。

## Recommended Evolution

继续以单 Task 行和显式 outcome 作为产品边界。顺序步骤保持为 Payload 内的轻量游标；只有出现跨 Task 依赖、并行或完整审计需求时，才评估独立 Run/workflow 层。
