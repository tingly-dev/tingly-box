# internal/task Architecture

**Last Updated**: 2026-07-16
**Cache Level**: Module
**Expires**: 2026-07-29
**Branch**: `feat/task`
**Hash**: `f56b71c9f8d3771ece300eee598b7a4611617b61`

## Overview

`internal/task` 已演进并接入实验性 Task 产品面。它提供延时/重复触发、按 workspace 串行、取消、重试、显式 reschedule、等待人工输入、native handoff 和启动恢复；agent Task 在同一 workspace/native session 中执行无人值守 bounded runs，并可通过 Payload 游标顺序推进多个显式步骤。

## Components

| Component | Location | Purpose |
|---|---|---|
| Task model | `internal/task/task.go` | 通用 Task、状态、payload/result、scheduled time |
| Manager | `internal/task/manager.go` | submit/cancel/list/start/dispatch/run 生命周期 |
| Scheduler | `internal/task/scheduler.go` | 每 5 秒查找 due pending tasks |
| Runner contracts | `internal/task/runner.go` | handler registry、progress/checkpoint 和 bounded Run event controller |
| Serialization | `internal/task/cancellation.go`, `queue.go` | 同一 key 单执行者与排队 |
| Persistent store | `internal/data/db/task_store.go` | GORM `tasks` 表及部分更新 |
| Run store | `internal/data/db/task_run_store.go` | GORM `task_runs` 表、有效输入/策略、结果和受限事件历史 |
| Agent handler | `internal/task/agenttask/` | workspace/session、步骤游标、无人值守策略、事件摘要和 handoff 归一化 |
| Agent runtime | `agentboot/` | CLI process、stream events、cancel/wait |
| Claude adapter | `agentboot/claude/` | Claude CLI driver、transport、session resume |

## Current Data Flow

```text
Manager.Submit
  → TaskStore.Create(status=pending)
  → Scheduler.FindDueTasks
  → dispatchTask(serialization key)
  → Agent Handler.Run(current step or free goal)
  → complete / reschedule / needs_input / handoff_required / failed / cancelled
  → checkpoint native session + optional step cursor/outcome
```

## Key Patterns

- `Payload` / `Result` 是 JSON，可先承载实验期强类型 DTO，避免立即扩表。
- `SerializationKey` 同时覆盖运行锁和 FIFO queue，适合使用 canonical workspace path。
- `UpdateStatus` 部分更新避免并发 goroutine 全量覆盖记录。
- `agentboot.ExecutionHandle` 提供流事件、取消与最终结果；Task 不代理 native stdin 审批。
- Claude adapter 已支持固定 `SessionID` 的创建和 `--resume`。
- 顺序步骤存放在版本化 agent Payload；`done` 原子 checkpoint outcome 并推进游标，下一步骤复用同一 Task 行。
- Claude 所选工具同时映射为 `--tools` 和 `--allowedTools`；默认只自动读写文件，Shell 需要显式授权。Codex 固定 `approval_policy=never` 并使用 read-only/workspace-write sandbox。
- 未预授权的 Approval/permission denial 结束当前 Run 并进入 `handoff_required`；业务 Ask 进入 `needs_input`。二者都通过新 Run 接续，不保活 stdin。
- Run 事件最多保留 200 条；单字段/单事件有大小限制并对常见 secret key 脱敏，不复制完整 transcript 或 reasoning。

## Constraints / Gaps

- restart 仍将 running 标记 interrupted；用户通过 Run now 从持久化 session/workspace/step cursor 恢复。
- `task_runs` 保存 bounded structured history，但不是完整 native transcript；深度人工交互仍通过 `cd <workspace> && <agent> resume <session>`。
- 步骤仅支持顺序推进；没有 DAG、分支、并行或步骤级 agent/schedule/retry。
- native session 丢失时明确失败，不自动创建替代 session。

## Integration Points

- `internal/data/db/store_manager.go`: 已 AutoMigrate `TaskRecord` 并暴露 `Tasks()`。
- `internal/server/task_runtime.go`: 创建/start Task manager，注册 Claude/Codex agent handler，并在 server shutdown 时停止 scheduler。
- `agentboot/claude`, `agentboot/codex`: 提供 native worker；Task 只使用其 unattended CLI surface。
- `frontend/src/contexts/FeatureFlagsContext.tsx` 与 `GlobalExperimentalFeatures.tsx`: `_global.extensions.task` 实验扩展开关；不是 scenario flag registry 成员。
- `frontend/src/layout/useActivityItems.tsx` / `App.tsx`: 顶层 Task 导航与路由。
- API 必须先在 backend 注册 swagger model；frontend 暂用 placeholder，等待 codegen。

## Recommended Evolution

继续以 Task + bounded TaskRun + native session 作为产品边界。顺序步骤保持为 Payload 内的轻量游标；不要在 TB 内补建半套 Managed Agent console。只有出现跨 Task 依赖、并行或 durable branching 需求时，才评估 workflow 层。
