# Research: 持久化 Agent Task 的成熟模式

**Date**: 2026-07-15
**Type**: Product / Architecture Research
**Question**: 对“定时、一次性或持续 loop 地调用 Claude/Codex，保留 workspace 与 session，并允许人工介入”的 Task 产品，业界是否已有成熟模式？

---

## 结论

**有成熟模式，而且产品形态已经高度收敛。** 最接近目标的不是传统 cron UI，也不是通用 DAG 工作流，而是 OpenAI Codex Automations 与 Claude Code Scheduled Tasks / Agent View：

1. **Task / thread 是长期上下文容器**，不是某一次进程执行。
2. **schedule 只是唤醒机制**；到点后继续同一会话，或启动一个独立的新会话。
3. **workspace 与 session 是两条独立持久化轴**：workspace 保存文件状态，原生 session 保存对话、工具调用和决策历史。
4. **loop 不是永不退出的 goroutine**，而是同一 Task 被周期性唤醒，执行一轮，再睡眠或结束。
5. **人工介入是主流程**：列表首先区分 Working、Needs input、Completed；用户可查看、回复、attach、stop、resume。
6. **无人值守权限必须显式收窄**，不能沿用交互模式下“执行时再询问”的假设。

对 Tingly-Box 最合适的方向是：**借鉴 Codex / Claude 的产品语义，按需改造现有 `internal/task`，暂不引入 Temporal/LangGraph 式通用 durable workflow 层。**

---

## 1. 最接近的成熟产品方案

### 1.1 OpenAI Codex Automations

Codex 已经覆盖本需求中的关键组合：

- scheduled task 可以选择每次生成独立 Task，也可以回到当前 Task，复用已有上下文；
- Git 项目可在本地 checkout 或独立 background worktree 中运行；
- Task 内的分钟级 schedule 用于轮询长操作、持续 review/research/triage；
- 结果进入 Scheduled inbox / review queue，等待用户继续处理；
- 无人值守执行使用明确的 sandbox 和 approval policy。

这背后的模型不是“Schedule → Run → Engine Task”的固定三层，而是：

```text
Task（持久上下文）
  ├── Workspace（本地目录或 worktree）
  ├── Session（可延续的 agent 对话）
  └── Wake-up schedule
        ├── 回到当前 Task
        └── 启动独立 Task
```

Codex 还明确建议：持续任务的 prompt 要描述每次醒来做什么、何时值得汇报、何时停止、何时请求人工输入。这意味着“完成条件”更适合作为 Task 指令的一部分，先不必设计一套通用条件 DSL。

### 1.2 Claude Code

Claude 已经把执行可靠性分成三档：

| 形态 | 生命周期 | 本地文件 | 适合场景 |
|---|---|---:|---|
| `/loop` | 当前 session 内，短期 | 是 | 快速轮询、等待构建/部署 |
| Desktop scheduled task / background agent | 本机持久 | 是 | 需要本地工具与文件的自动任务 |
| Cloud Routine | 云端持久 | fresh clone | 机器离线仍需可靠运行 |

几个直接相关的设计事实：

- Claude session 持续写入本地 transcript，可按 session ID resume；
- session 查找与项目目录及其 worktree 绑定，因此工作路径不是附属字段，而是恢复会话的锚点；
- Agent View 按 Needs input、Working、Completed 分组，可 peek、reply、attach；
- 后台 session 由 supervisor 托管，进程退出后仍可从原 session 重启；
- Claude Agent SDK 明确区分：session 持久化的是对话，不是文件系统。

Claude 的 `/loop` 还设置了过期边界，且不会补跑所有错过的 tick。这说明成熟实现默认把 loop 当成“观察世界并继续推进”的节拍，而不是要求 cron 的 exactly-once 语义。

---

## 2. 通用 durable workflow 方案

### 2.1 Temporal

Temporal 是成熟的生产级 durable execution 方案：Workflow 保存状态，LLM 与工具调用作为 Activity 执行，支持 retry、timer、signal、pause/resume、human-in-the-loop 和完整历史。它适合以下条件：

- agent 流程跨多服务、持续数天或数月；
- 每个工具步骤都要求故障恢复和审计；
- 有严格审批、补偿事务、幂等或合规要求；
- 多个 worker / 节点共同执行，需要分布式可靠性。

但对当前 Tingly-Box Task v0，它会同时引入独立服务、worker、workflow determinism、activity 边界和事件历史，明显大于现阶段需求。**可以借鉴其思想，不建议现在引入其基础设施。**

值得借鉴的只有三点：

1. 外部副作用要可识别，重试前要考虑幂等；
2. 等待人工输入应是可持久化状态，而不是阻塞进程；
3. 长 loop 必须有步数、时间、预算或停止条件边界。

### 2.2 LangGraph

LangGraph 的成熟模式是 checkpoint + `thread_id` + interrupt/resume。重用 `thread_id` 继续旧状态，新 ID 创建全新 thread；人工输入通过 interrupt 暂停后恢复。

这个模型再次验证了“稳定 Task ID / session ID 是持久游标”。但 LangGraph 更适合我们自行编排 agent graph；当前目标是托管 Claude/Codex 这两个现成 agent CLI，因此再加一层 graph runtime 价值有限。

---

## 3. 对 Tingly-Box 数据模型的直接启示

成熟方案表明，最重要的是拆开两个正交维度：

### 维度 A：何时执行

- 立即一次
- 指定时间一次
- 按计划重复
- 当前轮结束后动态决定下次唤醒时间

### 维度 B：上下文如何延续

- `continue`：复用同一 workspace + native session
- `fresh`：复用 Task 定义，但每次创建新 session；workspace 是否复用另行决定

因此 one-shot、scheduled、loop 不需要三套领域对象。它们可以是同一个 Task 的不同 wake-up 配置。UI 也不必先让用户选择“模式”；用户只需表达“做什么”和“什么时候做”，continuation 使用聪明默认，必要时在高级配置中调整。

一个足够小的 v0 概念模型可以是：

```text
Task
  id
  title / prompt
  agent                 claude | codex
  status                pending | running | needs_input | sleeping | done | failed | stopped
  workspace_path        TB 生成的稳定路径
  native_session_id     Claude/Codex 原生 session ID
  next_run_at            nullable
  recurrence             nullable
  session_policy         continue | fresh（默认 continue，可暂不暴露 UI）
  last_summary / error
  timestamps
```

这里没有强制独立的 Automation、Run、Engine Task 三层。后续只有在出现真实需求时再拆：

- 用户需要查看每次定时执行的独立历史 → 增加 attempt/run history；
- 同一 Task 需要并行多个 agent → 增加 execution/session 子对象；
- 需要分布式恢复每个工具步骤 → 再评估 durable workflow engine。

---

## 4. workspace 与 session 应如何留存

### 推荐：混合方案

**TB 生成并管理稳定 workspace；Claude/Codex session 继续使用各自原生存储路径。** TB 只保存映射：

```text
task_id → agent + workspace_path + native_session_id
```

理由：

1. Claude 的 session 恢复与项目目录/worktree 有原生绑定；稳定 workspace 能直接满足恢复条件。
2. Codex app/CLI 会共享原生 session history；保留原生存储才能继续享受人工介入和跨 surface 恢复能力。
3. 自定义搬运或仿造 provider session 目录会依赖其未承诺的磁盘格式，升级风险高。
4. workspace 是 TB 真正需要拥有的产物：用户可进入目录查看文件、继续工作、复用到其他流程。

建议的默认路径形态：

```text
<TB_DATA>/tasks/<task-id>/workspace/
```

路径中的 Task ID 保证稳定与唯一；可另存展示名，不把可变 title 放进恢复主键。Git 隔离可后续在这个稳定根目录下引入 worktree，而不是 v0 就要求所有任务都创建 worktree。

### 暂不推荐：TB 自建统一 session store

统一复制 Claude/Codex transcript 到 TB 私有路径，看起来整齐，但会带来：

- session 格式和索引兼容成本；
- 原生 resume / picker / desktop 接管可能失效；
- 双写一致性和并发写入问题；
- TB 需要承担 transcript 版本迁移。

如果未来需要统一检索，可做只读索引或导出，不应先替代原生 session source of truth。

---

## 5. 对现有 `internal/task` 的判断

现有实现已有可复用的骨架：

- `ScheduledAt` 与 due-task polling；
- cancellation；
- serialization key，能避免同一 workspace 并发写；
- handler registry；
- 基础 retry / restart 状态处理；
- `Recurrence` 与 parent 字段虽未启用，但已经预留。

它并非必须原样保留。当前更值得调整的地方是：

- `TaskStatus` 需要表达 `needs_input` / `sleeping`，或至少允许 handler 把“等人”和“下次唤醒”持久化；
- restart 时一律把 running 标成 interrupted，只适合短 job；未来后台 agent supervisor 接入后需要可恢复语义；
- recurrence 不应默认等同于“父 Task 派生很多 child Task”，因为成熟方案同时支持继续同一 Task；
- `Payload` 可以先承载 provider/session/workspace 配置，验证稳定后再提炼强类型字段；
- `SerializationKey` 可直接使用 workspace identity，确保同一路径同一时刻只有一个执行者。

因此推荐定位是：**`internal/task` 就是首版产品 Task 的演进起点；可以改 schema、状态和调度语义，不要求 100% 服从当前抽象。**

---

## 6. 建议采用与暂缓的能力

### v0 建议采用

1. 单一 Task 记录，支持 now / once / recurring wake-up。
2. 默认继续同一 native session。
3. TB 生成稳定 workspace，原生 session store 不搬家。
4. 同 workspace 串行执行。
5. `needs_input`、Stop、Resume、Open workspace / Attach session 为一等能力。
6. loop 每轮结束后进入 sleeping，再次唤醒；必须有可停止条件。
7. 无人值守权限沿用 TB guardrails，并明确展示执行范围。

### 暂缓

- 通用 DAG / workflow designer；
- 独立 Automation + Run + Engine Task 三层；
- exactly-once 与错过 tick 全量补跑；
- 跨机器迁移 native session；
- TB 自建统一 transcript 格式；
- 多 agent 编排、分支与合并；
- Temporal/LangGraph 运行时。

---

## 7. 推荐决策

首版可以把产品定义压缩成一句话：

> **Task 是一个有稳定工作目录和可恢复 agent session 的长期任务；它可以立即执行、在指定时间醒来，或反复醒来，直到完成或需要人。**

这比“定时任务 / 一次性任务 / continuous loop 三种模式”更接近成熟产品，也更小。下一版 spec 应围绕这句话重写，并把首版重点放在：创建、唤醒、恢复、人工接管、停止，而不是领域分层。

---

## 8. 顺序步骤扩展决策

复杂目标如果完全交给一次 native session 内部规划，会隐藏检查点、扩大失败重试范围，也让人工介入只能依赖自然语言上下文。首个扩展应采用“外部顺序骨架 + 步骤内部自主执行”：

- TB 持久化有序步骤和当前游标；
- 每一步对应一次 bounded execution；
- Claude/Codex 仍决定该步骤内部的 model/tool turns；
- `done` 推进游标，`continue` 与 `needs_input` 保持当前步骤；
- 所有步骤复用 Task workspace 和 native session。

这不是通用 workflow engine：不引入 DAG、分支、并行、子 Task 或步骤级调度。它只把已有“一轮一次结果”的 supervisor 语义外显成稳定、可观察、可介入的顺序检查点。

---

## 参考资料

- [OpenAI — Scheduled tasks](https://learn.chatgpt.com/docs/automations)
- [OpenAI — Introducing the Codex app](https://openai.com/index/introducing-the-codex-app/)
- [Claude Code — Run prompts on a schedule](https://code.claude.com/docs/en/scheduled-tasks)
- [Claude Code — Manage sessions](https://code.claude.com/docs/en/sessions)
- [Claude Code — Manage multiple agents with agent view](https://code.claude.com/docs/en/agent-view)
- [Claude Agent SDK — Work with sessions](https://code.claude.com/docs/en/agent-sdk/sessions)
- [Temporal — AI Agent Reference Architecture](https://go.temporal.io/platform-hub/ai-engineering/ai-reference-architecture)
- [Temporal — Durable Execution](https://temporal.io/)
- [LangGraph — Interrupts](https://docs.langchain.com/oss/python/langgraph/interrupts)
