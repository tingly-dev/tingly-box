# Spec: Automated Task Loop Engine

**Date**: 2026-07-15
**Last Updated**: 2026-07-16
**Status**: Draft for implementation
**Research**: `task-agent-automation-patterns-20260715.research.md`

## 1. Decision

> **TB 配置并触发 Task；Claude/Codex 在固定 workspace 和 native session 中执行一轮；TB 在创建时确定无人值守权限边界，持久化 Run 并决定完成、接续、等业务输入或转交原生 CLI。**

直接演进现有 `internal/task`，不新增 `internal/automation`，也不建立通用 workflow engine。`TaskRun` 是一次真实 CLI process 的审计边界；Task 是长期目标和调度单元；native transcript 和交互能力仍由 Claude/Codex 自己持有。TB 不是 Managed Agent UI，也不复制一个不完整的 agent console。

## 2. Scope

### Goals

- 新增实验性 Task 页面，支持创建、查看、停止、再次运行和追加指令。
- 执行器仅支持 Claude Code 与 Codex。
- 支持立即一次、指定时间一次和 cron 重复触发。
- 可选持续推进：一轮未完成时在同一 session 中再次唤醒。
- 可选顺序步骤：用户显式给出步骤，每个步骤独立执行一轮并自动接续。
- 保存每次 bounded execution 的 Run 历史，而不是只保留最后结果。
- 在预授权 workspace/tool 边界内默认无人值守执行，不为每次工具调用等待页面审批。
- 业务问题结束当前 Run，用户回复后在同一 workspace/session 启动新 Run。
- 权限越界结束当前 Run 并进入 native handoff，不在 TB 中代理 CLI 审批。
- 保存有用的 Run 事件、最终结果、退出原因和有效启动策略，而不是只显示最后一句话。
- Task 显式保存启动 profile 和工具范围；Run 保存实际生效的启动策略快照。
- 每个 Task 使用稳定 workspace：默认由 TB 生成，也可在创建时选择用户已有目录。
- 同一 workspace/session 同时只有一个执行者。
- 服务重启后保留 Task、workspace、session ID 和最新结果。

### Non-goals

- DAG、条件分支、并行步骤、多 Task 依赖、多 agent 协作。
- 独立的步骤级 agent、timeout、schedule 和 retry 配置。
- TB 自建 transcript store。
- TB 代理原生审批、终端 stdin、完整 tool console 或实时 token stream。
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

Task 的输入分为三层：

- `Goal`：持久目标，是每次 Run 都必须显式携带的当前主题；可在非 running/queued 状态编辑。
- `Step`：可选的顺序阶段定义；本阶段创建后只读，避免编辑游标和历史结果产生歧义。
- `Instruction`：只作用于下一次 Run 的临时补充，成功交给 worker 后清空，不改写 Goal。

内部 outcome/system appendix 属于 TB 的执行协议和安全边界，不是 Task 主题，不提供用户编辑入口。

`TaskRun` 对应一次真实 CLI process。一次 Step 可以经历多个 Run。Task 可选携带一个有序步骤列表。没有步骤时保持当前自由目标语义；添加步骤后，TB 每轮只下发当前步骤，Agent 在步骤内部自主规划和调用工具：

```text
Task goal
  Step 1 → Run 1 (continue) → Run 2 → checkpoint
  Step 2 → Run 3 → checkpoint
  Step 3 → bounded Run → Task complete
```

步骤是同一个 Task 内的顺序游标，不是子 Task，也不形成 DAG。所有步骤复用同一个 workspace、agent 和 native session。

每次 trigger 只推进一轮：

```text
TB wakes Task
  → start/resume agent
  → collect events and final output
  → done | continue | needs_input | handoff_required | failed
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

- Claude：`plan` / `accept_edits`；默认 `accept_edits`。历史 `manual` 配置可读，但不再提供给新建 Task。
- Codex：`read_only` / `workspace_write`；默认 `workspace_write`。
- Claude 可选择 Files read / Files write / Terminal / Web 逻辑工具组；Codex v1 不伪造逐工具过滤能力。
- Claude 默认预授权 Files read / Files write；Terminal 需要显式选择，并提示 Shell 可越过其它逻辑工具标签。
- Claude 的所选工具同时是可见工具和本 Run 的免交互 allowlist；Codex 固定 `approval_policy=never` 并使用所选 sandbox。
- 不提供 Claude bypass/full-access 或 Codex danger-full-access；Terminal 明确提示其可间接读写和访问网络。

Goal/Steps 下方提供可选 `Working directory`。留空时由 TB 生成隔离目录；填写时必须是服务所在机器上已存在的绝对目录。它是任务内容的一部分，不增加“generated/custom”模式选择器。

选择已有目录时就地说明：Agent 会直接读取或修改其中内容，TB 不复制、不清理该目录。服务端返回 canonical absolute path，页面创建后始终展示实际生效值。

Goal 下方提供 `Add step`，而不是先选择“简单/多步骤”模式。用户未添加步骤时创建普通 Task；添加后按视觉顺序执行。v0 的步骤只输入 instruction，标题由 instruction 自动截取生成。

### Detail and actions

详情展示 goal、状态、最新摘要/错误、下一次运行、workspace 绝对路径、session ID、有效自动化策略和原生接管命令。顺序 Task 额外展示当前步骤、已完成步骤摘要和后续步骤。Run timeline 展示每轮 trigger、step、启动策略、关键 agent/tool 事件、结果、退出原因和错误。

- `Stop`：取消当前进程并停止后续触发。
- `Run now`：尚未完成的非运行 Task 立即执行当前 Goal/Step，不增加临时消息。
- `Run again`：终态 Task 从当前持久定义重新执行；顺序 Task 从第一步开始。
- `Run with instruction`：向同一 session 追加只用于下一 Run 的临时补充并立即唤醒。
- `Edit task`：编辑 title 和持久 Goal，但不自动执行；running/queued 时禁用并由 API 拒绝。
- `Open workspace`：打开工作目录。
- `Take over`：运行中先 Stop，然后复制或运行 `cd <workspace> && <agent> resume <session>`，在原生 CLI 中完整交互。
- `Continue automation`：人工接管完成后，在同一 session 启动下一 Run，重新进入 Task 的预授权边界。

必须区分两个暂停原因：

- `needs_input`：业务问题；原进程已结束，页面可回复并创建新 Run。
- `handoff_required`：Agent 请求超出无人值守边界的权限或原生交互；页面只解释原因并提供接管命令，不显示无效的“Approve once”。

完成后的 Task 仍可查看和再次运行。

## 5. Workspace and Session

```text
<configDir>/tasks/<task-id>/workspace/  # generated default
<user-selected-existing-directory>/    # optional
```

- 默认路径从 `AppConfig.ConfigDir()` 派生，目录权限 `0700`。
- 用户路径必须 absolute、存在且为 directory；通过 `EvalSymlinks` canonicalize 后保存，不保留含 symlink 的别名。
- 用户目录的权限、内容和生命周期仍由用户负责；TB 不创建其子目录、不复制、不删除。
- Task ID 使用服务生成 UUID；DB 保存 canonical absolute path。
- TB 拥有 workspace；Claude/Codex 继续拥有各自 native session store。
- TB 只保存 `agent + workspace_path + native_session_id`。
- `SerializationKey = canonical workspace path`，禁止并发 resume。
- 首次执行创建/捕获 session ID，后续默认 resume 同一 session。
- session 丢失时明确失败，不静默创建新 session。

v0 不支持为一个 Task 挂载多个目录或运行后切换 workspace；需要新目录时创建新 Task。fresh-session policy 仍 deferred。

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
    OutcomeHandoff    OutcomeKind = "handoff_required"
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
- handoff_required：保存越界原因，进入 handoff_required；只有 Run now / Continue automation 才重新调度。
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

每次 Run 的 user prompt 都显式包含当前 `Goal`，不能只依赖 native session 的历史上下文。最终 prompt 必须以持久化 Goal 原文开头，`prompt[0:len(goal)] == goal`；TB 不得在 Goal 前增加 label/preamble，也不得重排或改写 Goal 内容。首次无步骤 Run 的 prompt 完全等于 Goal；只有 resume、Step 或本次 Instruction 存在时，才在 Goal 之后追加 TB run context。无步骤 Task 的 resume prompt 组合为 `Goal + 本次 Instruction（若有）+ continue 边界`；顺序 Task 组合为 `Goal + 当前 Step + 本次 Instruction（若有）+ 不得提前执行后续步骤的边界`。

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
    State             string   `json:"state"` // done | continue | needs_input | handoff_required
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
- runtime 收到未被预授权的 Approval/Ask → 拒绝请求、checkpoint session、结束进程并归一化为 handoff_required。
- envelope 缺失但 CLI 成功 → 按 done，final text 作为 summary；避免格式漂移形成无限 loop。
- suggested delay 限制在 1 分钟到 24 小时。

步骤推进以 checkpoint 后的 Payload 为事实来源；native session 只提供上下文连续性。即使 session 无法恢复，UI 仍能准确显示当前游标和已完成结果。

## 8. Agent Runtime

### Claude

复用 `agentboot/claude`：固定 ProjectPath、stream JSON、显式 SessionID、resume、timeout 和 event collection。

`agentboot.ExecutionOptions` 同时传递：

- `AvailableTools` → Claude `--tools`，限制模型可见的工具集合；
- `AllowedTools` → Claude `--allowedTools`，把用户创建 Task 时选择的能力预授权给无人值守 Run；
- `PermissionMode=acceptEdits|plan`，不设置 stdio permission prompt tool。

新建 Task 不再提供 `manual`。历史 manual Task 在下一次运行前迁移为保守的 `plan`，或要求用户明确更新执行策略；不能悄悄扩大权限。

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

### Run history and native handoff

新增 `task_runs` 表。Manager 在每次 handler invocation 前创建 Run，在 complete/reschedule/needs-input/failure/cancel 后终结 Run。Task.Result 继续作为 latest projection 保持兼容，详情历史以 Run 为准。

Run 保存 bounded structured timeline：started、session discovered、assistant/tool 摘要、progress、outcome、handoff、failed/completed。每条事件有大小上限并对常见 secret 字段脱敏。它不复制无限 token delta 或 native transcript；完整上下文继续通过 workspace/session 的原生 CLI 查看。

无人值守控制流：

1. Task 创建时确定可见工具、免交互 allowlist 和 sandbox；Run 保存不可变快照。
2. Agent 在这个边界内自动运行，TB 不暂停等待网页审批。
3. 若 runtime 仍产生 Approval/Ask，TB 默认拒绝以避免悬挂，checkpoint session 并结束当前 Run。
4. Approval 归一化为 `handoff_required`；Ask 若能识别为业务问题则归一化为 `needs_input`，否则同样 handoff。
5. 用户在原生 CLI 完成高交互工作后，点击 `Continue automation` 启动一个新 Run。TB 不声称能恢复已经消失的 stdin request。

## 9. API and Server Wiring

Backend 先定义 swagger models；frontend 在 codegen 前使用集中 placeholder service。

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/tasks` | List |
| POST | `/api/v1/tasks` | Create |
| GET | `/api/v1/tasks/{id}` | Detail |
| PATCH | `/api/v1/tasks/{id}` | Edit title / durable Goal |
| POST | `/api/v1/tasks/{id}/wake` | Run now / Run again / Run with instruction |
| POST | `/api/v1/tasks/{id}/stop` | Stop |
| GET | `/api/v1/tasks/agents` | Agent availability |
| GET | `/api/v1/tasks/{id}/runs` | Run history |
| GET | `/api/v1/tasks/{id}/runs/{runID}` | Run detail |

Create request 可选增加 `steps: [{instruction}]`、execution policy 和 `workspace_path`。服务端按 agent capability 校验、canonicalize 用户目录，并生成稳定 step ID 和展示标题。Create request 仍不接受 session ID、current step 或 step outcome。`/tasks/agents` 返回 launch profiles、automation boundary 和 tool filtering capabilities，不返回伪造的 live-control 能力。

Update request 使用 pointer 字段区分 omitted 与显式空值，只接受 `title` 和 `goal`。`goal` 若出现则 trim 后必须非空；`title` 可清空以恢复 Goal 作为列表标题。更新保留 workspace、session、steps、step outcomes、execution、schedule 和 recurrence，不触发 Wake。running/queued 返回 409；其它状态保存后由下一 Run 读取。

Server startup：从 `StoreManager.Tasks()` 创建 Manager，注册 agent handler 和 API，依赖就绪后 Start，graceful shutdown 时 Stop。

Task manager 始终启动；全局实验扩展 `extensions.task` 只控制入口和创建权限，确保隐藏功能后已有 Task 仍可停止和恢复。

## 10. Recovery, Security and Tests

### Recovery

- 重启时 running → interrupted，保留 session/workspace；Run now 从同一 session 恢复。
- 重启时 active Run → interrupted；已丢失的 stdin/handle 不存在网页恢复路径，只能新 Run 或 native handoff。
- CLI、session 或 workspace 不存在时明确 failed，不自动换路径或 session。
- server 离线期间错过的 cron 最多执行一次，然后计算未来下一次。
- Stop 后不再自动 recurrence。

### Security

- workspace 服务端生成并 canonicalize，禁止路径穿越。
- 用户 workspace 只接受 canonicalizable 的现有绝对目录；相对路径、文件、缺失路径在创建 API 直接拒绝。
- Payload/Result 不保存 token 或完整 env。
- 子进程只传必要 env，日志不输出 secret。
- 无人值守执行只发生在显式 allowlist + workspace sandbox 内；未授权请求默认拒绝并 handoff。
- 不提供 bypass/full-access profile，也不把 `manual` 伪装成可自动化 profile。
- 运行事件入库和展示前截断并对常见敏感字段脱敏。
- artifact 打开动作仅允许 workspace 内路径。

### Required tests

- outcome 状态迁移、Wake/Cancel conflict、serialization、retry reset。
- cron timezone、missed tick、overlap skip。
- workspace 与 payload/result validation。
- Claude/Codex create/resume args 和 event normalization。
- fake agent：done、continue、needs input、cancel、restart resume。
- API validation/auth；前端创建、状态分组和主要动作。
- Goal 更新状态约束、字段保留、空 Goal 拒绝；编辑后下一 Run prompt 显式包含新 Goal。
- 顺序步骤逐轮 prompt、done 推进、continue/needs-input 不推进、最终完成和 recurrence 重置。
- Claude tools/profile 参数映射；Codex per-run sandbox 映射和不支持组合拒绝。
- Run lifecycle/history、旧 Task 兼容、restart interruption。
- Claude selected tools 同时映射到 available/allowed tools；不启用 stdio broker。
- Claude approval/ask 默认拒绝并分别形成 handoff/needs-input；没有挂起进程泄漏。
- 结构化 Run 事件有长度上限、脱敏并能在历史详情读取。

## 11. Implementation Order

1. Spec 收敛：明确 Loop Engine / native handoff 边界，删除 live-control 产品承诺。
2. Runtime：Claude allowlist、Codex no-approval sandbox、Ask/Approval 自动拒绝和结果归一化。
3. Run detail：持久化受限的真实事件、退出原因与执行快照；移除 respond API。
4. Frontend：自动化边界、reply-and-resume、native handoff、Run detail。
5. Harness：Claude unattended/handoff；Codex sandbox；multi-step multi-run；restart。

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
- Done 后仍可进入和 Run again。
- Goal、Step、Instruction 分轴展示；不暴露内部 system appendix 编辑器。
- Stop、Run with instruction、Take over 只影响当前 Task。
