# Spec: Task Agent Supervisor

**Date**: 2026-07-15
**Status**: Draft for implementation
**Research**: `task-agent-automation-patterns-20260715.research.md`

## 1. Decision

> **TB 配置并触发 Task；Claude/Codex 在固定 workspace 和 native session 中执行一轮；TB 回收结果，决定完成、等人、重试或再次唤醒。**

首版直接演进现有 `internal/task`。不新增 `internal/automation`，不建立 Automation / Run / Engine Task 三层，也不复制 native transcript。

## 2. Scope

### Goals

- 新增实验性 Task 页面，支持创建、查看、停止、再次运行和追加指令。
- 执行器仅支持 Claude Code 与 Codex。
- 支持立即一次、指定时间一次和 cron 重复触发。
- 可选持续推进：一轮未完成时在同一 session 中再次唤醒。
- 每个 Task 使用 TB 生成的稳定 workspace。
- 同一 workspace/session 同时只有一个执行者。
- 服务重启后保留 Task、workspace、session ID 和最新结果。

### Non-goals

- 独立 Run 表和完整执行历史。
- DAG、多 Task 依赖、多 agent 协作。
- TB 自建 transcript store。
- 分布式 worker、exactly-once、错过 tick 全量补跑。
- 自动 clone/worktree、跨机器 session 迁移。
- 使用 Claude `/loop` 或 Codex Automations 自带调度。

## 3. Product Model

Task 是长期工作单元，不是一次进程：

```text
Task
  ├── Goal
  ├── Agent: claude | codex
  ├── Stable workspace
  ├── Native session ID
  ├── Next wake-up / recurrence
  └── Latest outcome
```

每次 trigger 只推进一轮：

```text
TB wakes Task
  → start/resume agent
  → collect events and final output
  → done | continue | needs_input | failed
```

一轮是一次可取消、可回收结果的 CLI process 生命周期；进程内部仍可执行多个 model/tool turns。

## 4. UX

### Entry and list

- `_global/task` 开启后显示顶层 Task 入口，`/tasks` 直接打开工作面。
- 列表优先回答：什么在工作、什么在等我、下一次何时运行。
- 关闭实验 flag 只隐藏入口，不删除或静默停止已有 Task。

### Create

首屏字段：

1. `What should be done?`
2. `Agent`: Claude Code / Codex，并显示本机可用状态。
3. `When`: Now / Later / Repeat。

高级项只有：

- `Keep checking until done`，默认关闭；
- follow-up delay，默认 5 分钟；
- max wake-ups，默认 20；
- cron/timezone；
- 单轮 timeout，默认 30 分钟。

workspace 由 TB 生成，不让用户先选路径。

### Detail and actions

详情展示 goal、状态、最新摘要/错误、下一次运行、workspace 绝对路径、session ID 和 native resume 命令。

- `Stop`：取消当前进程并停止后续触发。
- `Run now`：立即重新唤醒非运行 Task。
- `Send instruction`：向同一 session 追加消息并唤醒。
- `Open workspace`：打开工作目录。
- `Take over`：先 Stop，再展示 native resume 命令。

完成后的 Task 仍可查看和再次运行。

## 5. Workspace and Session

```text
<configDir>/tasks/<task-id>/workspace/
```

- 路径从 `AppConfig.ConfigDir()` 派生，目录权限 `0700`。
- Task ID 使用服务生成 UUID；DB 保存 canonical absolute path。
- TB 拥有 workspace；Claude/Codex 继续拥有各自 native session store。
- TB 只保存 `agent + workspace_path + native_session_id`。
- `SerializationKey = canonical workspace path`，禁止并发 resume。
- 首次执行创建/捕获 session ID，后续默认 resume 同一 session。
- session 丢失时明确失败，不静默创建新 session。

v0 不支持用户自选 workspace、attached path 或 fresh-session policy。

## 6. Evolve `internal/task`

继续复用单一 `tasks` 表、Manager、Store、scheduler、cancel、retry、serialization queue，以及 Payload/Result/Recurrence JSON 字段。

### Status

新增：

```go
const StatusNeedsInput TaskStatus = "needs_input"
```

`needs_input` 非 terminal，但 scheduler 不会自动选择；只有 `Send instruction` / `Run now` 才回到 pending。

不新增 sleeping 状态。未来时间上的 pending Task 在 UI 显示为 Waiting。

### Explicit handler outcome

```go
type OutcomeKind string

const (
    OutcomeComplete   OutcomeKind = "complete"
    OutcomeReschedule OutcomeKind = "reschedule"
    OutcomeNeedsInput OutcomeKind = "needs_input"
)

type TaskResult struct {
    Outcome   OutcomeKind
    Result    json.RawMessage
    NextRunAt *time.Time
}
```

- complete：非重复 Task → succeeded；cron Task → 计算下一次并回 pending。
- reschedule：NextRunAt 必填，回 pending。
- needs_input：保存问题，进入 needs_input。
- error：沿用 retry / failed / cancelled。
- Outcome 为空按 complete 处理，避免破坏已有 handler。

一次成功 wake-up 后 retry attempt 清零。业务 wake-up 次数放在 agent payload，不和失败重试混用。

Manager 增加：

```go
Wake(ctx, taskID string, at time.Time) error
UpdatePayload(ctx, taskID string, payload json.RawMessage) error
```

HTTP 不直接修改 Store。running/queued Task 执行 Wake 返回 conflict。

Controller 同步增加 `UpdatePayload`，用于 worker 在取得 native session ID 后立即 checkpoint，避免进程中断时丢失恢复句柄。

## 7. Agent Task Contract

`Type = "agent"`，Payload 使用版本化 JSON：

```go
type AgentTaskPayload struct {
    Version        int            `json:"version"`
    Title          string         `json:"title"`
    Goal           string         `json:"goal"`
    Agent          string         `json:"agent"` // claude | codex
    WorkspacePath  string         `json:"workspace_path"`
    SessionID      string         `json:"session_id,omitempty"`
    PendingInput   string         `json:"pending_input,omitempty"`
    FollowUp       FollowUpPolicy `json:"follow_up"`
    WakeCount      int            `json:"wake_count"`
    TimeoutSeconds int            `json:"timeout_seconds"`
}

type FollowUpPolicy struct {
    Enabled      bool `json:"enabled"`
    DelaySeconds int  `json:"delay_seconds"`
    MaxWakeUps   int  `json:"max_wake_ups"`
}
```

`PendingInput` 成功交给 worker 后清空。

Recurrence 继续使用现有字段：

```go
type RecurrenceSpec struct {
    Cron     string `json:"cron"`
    Timezone string `json:"timezone"`
}
```

`ScheduledAt` 保存下一个具体 UTC 时间。上一次仍运行或等待输入时跳过当前 tick，不创建 backlog。

Worker 最终输出统一为：

```go
type AgentTaskResult struct {
    State             string   `json:"state"` // done | continue | needs_input
    Summary           string   `json:"summary"`
    Question          string   `json:"question,omitempty"`
    Artifacts         []string `json:"artifacts,omitempty"`
    NativeSessionID   string   `json:"native_session_id"`
    SuggestedDelaySec int      `json:"suggested_delay_seconds,omitempty"`
}
```

TB 通过统一 system appendix 要求该 envelope；adapter 负责提取和校验。

- done → complete。
- continue → 仅在 FollowUp 开启且未超过 MaxWakeUps 时 reschedule。
- needs_input → needs_input。
- envelope 缺失但 CLI 成功 → 按 done，final text 作为 summary；避免格式漂移形成无限 loop。
- suggested delay 限制在 1 分钟到 24 小时。

## 8. Agent Runtime

### Claude

复用 `agentboot/claude`：固定 ProjectPath、stream JSON、显式 SessionID、resume、timeout 和 event collection。

### Codex

`agentboot` 当前尚无 Codex worker，需要新增 `agentboot/codex`：

- Driver：binary discovery、`codex exec` / `exec resume`、cwd/env。
- Transport：解析 JSONL，提取 thread ID、message、approval 和 terminal result。
- Agent：复用通用 Runner。
- fixtures 和 CLI builder tests。

同时启用 `agentboot.AgentTypeCodex`。`ai/agent.CodexConfig` 只负责配置，不作为 runtime。

### Intervention

- Message events 更新 Task progress。
- Stop 取消 ExecutionHandle。
- AskUserQuestion 转为 needs_input。
- approval 可由明确的 Task permission policy 放行；否则拒绝并转 needs_input。
- 用户追加指令后 resume 同一 session。

审批/问答不无限占用进程：捕获请求后结束当前 bounded execution，持久化等待状态。v0 不做页面内单次审批；用户可追加替代指令，或 Stop 后使用 Take over 在 native CLI 中处理。

## 9. API and Server Wiring

Backend 先定义 swagger models；frontend 在 codegen 前使用集中 placeholder service。

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/tasks` | List |
| POST | `/api/v1/tasks` | Create |
| GET | `/api/v1/tasks/{id}` | Detail |
| POST | `/api/v1/tasks/{id}/wake` | Run now / Send instruction |
| POST | `/api/v1/tasks/{id}/stop` | Stop |
| GET | `/api/v1/tasks/agents` | Agent availability |

Create request 不接受 workspace path 或 session ID。

Server startup：从 `StoreManager.Tasks()` 创建 Manager，注册 agent handler 和 API，依赖就绪后 Start，graceful shutdown 时 Stop。

Task manager 始终启动；实验 flag 只控制入口和创建权限，确保隐藏功能后已有 Task 仍可停止和恢复。

## 10. Recovery, Security and Tests

### Recovery

- 重启时 running → interrupted，保留 session/workspace；Run now 从同一 session 恢复。
- CLI、session 或 workspace 不存在时明确 failed，不自动换路径或 session。
- server 离线期间错过的 cron 最多执行一次，然后计算未来下一次。
- Stop 后不再自动 recurrence。

### Security

- workspace 服务端生成并 canonicalize，禁止路径穿越。
- Payload/Result 不保存 token 或完整 env。
- 子进程只传必要 env，日志不输出 secret。
- unattended approval 默认 deny。
- artifact 打开动作仅允许 workspace 内路径。

### Required tests

- outcome 状态迁移、Wake/Cancel conflict、serialization、retry reset。
- cron timezone、missed tick、overlap skip。
- workspace 与 payload/result validation。
- Claude/Codex create/resume args 和 event normalization。
- fake agent：done、continue、needs input、cancel、restart resume。
- API validation/auth；前端创建、状态分组和主要动作。

## 11. Implementation Order

1. Task core：outcome、needs_input、Wake、recurrence 和 tests。
2. Claude vertical slice：handler、workspace/session、fake integration test。
3. Codex driver/transport。
4. Backend swagger API 与 server lifecycle。
5. Frontend flag、navigation、Task page 和 placeholder API。
6. 真实 Claude/Codex harness：create、resume、stop。

先用 Claude vertical slice 验证 supervisor 模型，再接 Codex；两者最终使用同一 Task handler contract。

## 12. Deferred

- Run history。
- fresh session per recurrence。
- worktree / attached path。
- notification inbox。
- 多 Task workflow 或 durable execution engine。

## 13. UX Principles Check

- 直接打开工作面，不先选模式。
- When 与持续推进分轴。
- 展示具体 next run、workspace、session 和 resume command。
- Done 后仍可进入和 Run now。
- Stop、Send instruction、Take over 只影响当前 Task。
