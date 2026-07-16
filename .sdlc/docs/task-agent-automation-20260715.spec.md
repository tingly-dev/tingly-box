# Spec: Task Agent Supervisor

**Date**: 2026-07-15
**Last Updated**: 2026-07-16
**Status**: Draft for implementation
**Research**: `task-agent-automation-patterns-20260715.research.md`

## 1. Decision

> **TB 配置并触发 Task；Claude/Codex 在固定 workspace 和 native session 中执行一轮；TB 持久化每一轮 Run，并在原进程仍存活时承接审批/问答，随后决定完成、等人、重试或再次唤醒。**

直接演进现有 `internal/task`，不新增 `internal/automation`，也不建立通用 workflow engine。新增轻量 `TaskRun` 作为一次真实 CLI process 的审计和交互边界；Task 仍是长期目标和调度单元，native transcript 仍由 Claude/Codex 自己持有。

## 2. Scope

### Goals

- 新增实验性 Task 页面，支持创建、查看、停止、再次运行和追加指令。
- 执行器仅支持 Claude Code 与 Codex。
- 支持立即一次、指定时间一次和 cron 重复触发。
- 可选持续推进：一轮未完成时在同一 session 中再次唤醒。
- 可选顺序步骤：用户显式给出步骤，每个步骤独立执行一轮并自动接续。
- 保存每次 bounded execution 的 Run 历史，而不是只保留最后结果。
- Claude Run 遇到审批/问答时保持原进程存活，允许页面在超时前响应。
- Task 显式保存启动 profile 和工具范围；Run 保存实际生效的启动策略快照。
- 每个 Task 使用 TB 生成的稳定 workspace。
- 同一 workspace/session 同时只有一个执行者。
- 服务重启后保留 Task、workspace、session ID 和最新结果。

### Non-goals

- DAG、条件分支、并行步骤、多 Task 依赖、多 agent 协作。
- 独立的步骤级 agent、timeout、schedule 和 retry 配置。
- TB 自建 transcript store。
- 完整 token delta、无限 stdout 或完整 native transcript 复制。
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
  ├── Default execution policy
  ├── Latest outcome (compatibility projection)
  └── Runs
```

`TaskRun` 对应一次真实 CLI process。一次 Step 可以经历多个 Run；审批发生在同一个 Run 内，不创建新 Run。Task 可选携带一个有序步骤列表。没有步骤时保持当前自由目标语义；添加步骤后，TB 每轮只下发当前步骤，Agent 在步骤内部自主规划和调用工具：

```text
Task goal
  Step 1 → Run 1 (continue) → Run 2 (approval → resume) → checkpoint
  Step 2 → Run 3 → checkpoint
  Step 3 → bounded Run → Task complete
```

步骤是同一个 Task 内的顺序游标，不是子 Task，也不形成 DAG。所有步骤复用同一个 workspace、agent 和 native session。

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

- `_global.extensions.task` 开启后显示顶层 Task 入口，`/tasks` 直接打开工作面。
- 列表优先回答：什么在工作、什么在等我、下一次何时运行。
- 关闭全局实验扩展 `extensions.task` 只隐藏入口，不删除或静默停止已有 Task；Task 不注册为 scenario flag。

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

Execution 区域在选择 Agent 后展示该 runtime 真实支持的启动 profile 和工具能力：

- Claude：`plan` / `manual` / `accept_edits`；默认 `accept_edits`。
- Codex：`read_only` / `workspace_write`；默认 `workspace_write`。
- Claude 可选择 Files read / Files write / Terminal / Web 逻辑工具组；Codex v1 不伪造逐工具过滤能力。
- 不提供 Claude bypass/auto 或 Codex danger-full-access。

workspace 由 TB 生成，不让用户先选路径。

Goal 下方提供 `Add step`，而不是先选择“简单/多步骤”模式。用户未添加步骤时创建普通 Task；添加后按视觉顺序执行。v0 的步骤只输入 instruction，标题由 instruction 自动截取生成。

### Detail and actions

详情展示 goal、状态、最新摘要/错误、下一次运行、workspace 绝对路径、session ID、有效启动策略和可复现权限的 native resume 命令。顺序 Task 额外展示当前步骤、已完成步骤摘要和后续步骤。Run timeline 展示每轮 trigger、step、启动策略、结果和错误。

- `Stop`：取消当前进程并停止后续触发。
- `Run now`：立即重新唤醒非运行 Task。
- `Send instruction`：向同一 session 追加消息并唤醒。
- `Open workspace`：打开工作目录。
- `Take over`：先 Stop，再展示 native resume 命令。

存在活的 pending control 时，详情顶部显示具体 tool、input/cwd、请求时间和过期时间，并提供 `Approve once` / `Deny` / `Stop`。必须区分：

- `waiting_approval` / `waiting_input`：原进程仍存活，可响应原请求；
- Task `needs_input`：原进程已结束，只能通过新 Run 继续。

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
    Steps          []TaskStep     `json:"steps,omitempty"`
    CurrentStep    int            `json:"current_step,omitempty"`
    StepOutcomes   []StepOutcome  `json:"step_outcomes,omitempty"`
    Execution      ExecutionPolicy `json:"execution"`
}

type ExecutionPolicy struct {
    LaunchProfile string           `json:"launch_profile"`
    Tools         []ToolCapability `json:"tools,omitempty"`
}

type TaskStep struct {
    ID          string `json:"id"`
    Title       string `json:"title"`
    Instruction string `json:"instruction"`
}

type StepOutcome struct {
    StepID      string          `json:"step_id"`
    Result      AgentTaskResult `json:"result"`
    CompletedAt time.Time       `json:"completed_at"`
}

type FollowUpPolicy struct {
    Enabled      bool `json:"enabled"`
    DelaySeconds int  `json:"delay_seconds"`
    MaxWakeUps   int  `json:"max_wake_ups"`
}
```

`PendingInput` 成功交给 worker 后清空。

顺序步骤约束：

- 最多 50 个步骤；instruction trim 后不能为空。
- `CurrentStep` 指向下一次需要执行的步骤；已全部完成时等于 `len(Steps)`。
- `StepOutcomes` 只记录已完成步骤，按执行顺序追加。
- 每轮 prompt 明确包含总目标、当前步骤和 `Do not start later steps` 边界。
- 当前步骤 `done`：保存 outcome；有下一步时立即 reschedule 同一 Task，没有下一步时 complete。
- 当前步骤 `continue`：仍停留在当前步骤；follow-up 可用时 reschedule，否则转 `needs_input` 等待 Run now 或指令。
- 当前步骤 `needs_input` / error：仍停留在当前步骤。
- 已完成顺序 Task 的 `Run now` 从第一步重新开始；带 instruction 的额外唤醒只执行该 instruction，不重放步骤。
- cron 的下一次 occurrence 从第一步重新开始。

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

步骤推进以 checkpoint 后的 Payload 为事实来源；native session 只提供上下文连续性。即使 session 无法恢复，UI 仍能准确显示当前游标和已完成结果。

## 8. Agent Runtime

### Claude

复用 `agentboot/claude`：固定 ProjectPath、stream JSON、显式 SessionID、resume、timeout 和 event collection。

`agentboot.ExecutionOptions` 增加真正对应 Claude `--tools` 的 available tools 字段；不得继续用 `--allowedTools` 代替工具可见性。`--allowedTools` 表示免审批规则，v1 不由 Task UI 暴露。Claude runtime 开启 stdio control protocol，将 `manual` 纳入有效 permission mode。

### Codex

复用现有 `agentboot/codex` worker，并增加 per-run sandbox override：

- `read_only` → `sandbox_mode=read-only`；
- `workspace_write` → `sandbox_mode=workspace-write`；
- approval policy 保持 `never`，因为当前 JSON transport 没有可响应的交互协议；
- Codex 权限越界作为 Run failure/event 展示，不显示无效审批按钮。

### Execution policy

Task 保存默认 execution policy；每个 Run 保存不可变的 effective policy snapshot，包括 agent、launch profile、native permission/sandbox mode、实际工具列表、approval behavior 和 workspace。旧 payload 映射到 `legacy_inherited`，保持可读并在 UI 提示升级。

工具能力和审批规则必须分轴：

- `available tools` 决定模型能看到什么；
- `approval` 决定一个已存在工具的调用是否需要人工确认；
- `sandbox/permission mode` 决定 runtime 的执行边界。

`terminal` 是高权限能力：允许 Bash 后不能声称关闭 Write/Web 就形成安全边界。Task 不得在运行中升级策略；Wake 可带一次性 execution override，只作用于新 Run并被 Run snapshot 记录。

### Run history and intervention

新增 `task_runs` 表。Manager 在每次 handler invocation 前创建 Run，在 complete/reschedule/needs-input/failure/cancel 后终结 Run。Task.Result 继续作为 latest projection 保持兼容，详情历史以 Run 为准。

Run 保存 bounded structured timeline：started、progress summary、control requested/responded、outcome、failed/completed。它不保存 token delta 或完整 stdout。

Claude control flow：

1. 收到 Approval/Ask 后把 pending control checkpoint 到当前 Run，并注册 `controlID → live ExecutionHandle/native request ID`。
2. handler 阻塞等待，CLI process 和 workspace serialization lock 保持存活。
3. API 以 control ID 单次 CAS 响应，再调用原 handle `Respond`；成功后清除 pending control并继续同一 Run。
4. 超时默认 deny；Stop 取消进程；server restart 将 Run 标记 interrupted、control 标记 expired。
5. late/duplicate/closed-handle response 返回 conflict，不伪装成功。

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
| GET | `/api/v1/tasks/{id}/runs` | Run history |
| GET | `/api/v1/tasks/{id}/runs/{runID}` | Run detail |
| POST | `/api/v1/tasks/{id}/runs/{runID}/control/{controlID}/respond` | Approve/deny/answer live control |

Create request 可选增加 `steps: [{instruction}]` 和 execution policy；服务端按 agent capability 校验并生成稳定 step ID 和展示标题。Create request 仍不接受 workspace path、session ID、current step 或 step outcome。`/tasks/agents` 返回 launch profiles、interactive control 和 tool filtering capabilities。

Server startup：从 `StoreManager.Tasks()` 创建 Manager，注册 agent handler 和 API，依赖就绪后 Start，graceful shutdown 时 Stop。

Task manager 始终启动；全局实验扩展 `extensions.task` 只控制入口和创建权限，确保隐藏功能后已有 Task 仍可停止和恢复。

## 10. Recovery, Security and Tests

### Recovery

- 重启时 running → interrupted，保留 session/workspace；Run now 从同一 session 恢复。
- 重启时 active Run → interrupted，pending control → expired；已丢失的 stdin/handle 不能再次审批。
- CLI、session 或 workspace 不存在时明确 failed，不自动换路径或 session。
- server 离线期间错过的 cron 最多执行一次，然后计算未来下一次。
- Stop 后不再自动 recurrence。

### Security

- workspace 服务端生成并 canonicalize，禁止路径穿越。
- Payload/Result 不保存 token 或完整 env。
- 子进程只传必要 env，日志不输出 secret。
- unattended approval 默认 deny；不提供 bypass/full-access profile。
- control response 单次消费，过期/重复/非当前 Run 请求返回 conflict。
- control input/result 入库和展示前截断并对常见敏感字段脱敏。
- artifact 打开动作仅允许 workspace 内路径。

### Required tests

- outcome 状态迁移、Wake/Cancel conflict、serialization、retry reset。
- cron timezone、missed tick、overlap skip。
- workspace 与 payload/result validation。
- Claude/Codex create/resume args 和 event normalization。
- fake agent：done、continue、needs input、cancel、restart resume。
- API validation/auth；前端创建、状态分组和主要动作。
- 顺序步骤逐轮 prompt、done 推进、continue/needs-input 不推进、最终完成和 recurrence 重置。
- Claude tools/profile 参数映射；Codex per-run sandbox 映射和不支持组合拒绝。
- Run lifecycle/history、旧 Task 兼容、restart interruption。
- Claude approval/ask 原 Run 继续、deny、timeout、stop、late response 和 duplicate response。

## 11. Implementation Order

1. Spec 与 execution policy：Task payload、agent capability、Claude tools/profile、Codex per-run sandbox。
2. Run persistence：domain/store、Manager lifecycle、restart recovery、history API。
3. Claude live control：broker、Run checkpoint、respond API、timeout/stop/restart。
4. Frontend：创建策略、attention controls、Run timeline、可复现 resume command。
5. Harness：Claude approve/deny/resume；Codex sandbox；multi-step multi-run；restart。

每阶段独立 commit，先保证后端状态和 API，再接前端。

## 12. Deferred

- fresh session per recurrence。
- worktree / attached path。
- notification inbox。
- 多 Task workflow 或 durable execution engine。
- DAG、步骤分支/并行、步骤级执行器与步骤级调度。
- Claude OS/container sandbox、自定义 Bash allowlist、Task-specific MCP/plugin/skill 管理。

## 13. UX Principles Check

- 直接打开工作面，不先选模式。
- When 与持续推进分轴。
- 不增加“任务模式”选择器；添加步骤自然形成顺序 Task。
- 展示具体 next run、workspace、session 和 resume command。
- Done 后仍可进入和 Run now。
- Stop、Send instruction、Take over 只影响当前 Task。
