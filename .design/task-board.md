# Task Board — 异构执行器的控制平面

> Audience: 准备实现 task 板块（`internal/task/` 定义层、`internal/server/module/task/`、
> 前端 Task 页面、executor Handler）的贡献者。
>
> Status: **设计定稿，未实现**。`internal/task/` 已存在一个休眠的 run 引擎
> （代码注释中的 "Phase 4" 是本方向此前唯一的文字痕迹），本文档给出它的产品形态、
> 领域模型、与现有子系统的接线方式和分期计划。
>
> Related: `.design/ux-principles.md`（设计判断标准）、
> `.design/agentboot-refactor.md`（executor 运行时方向）、
> `.design/smart-guide-on-claude-code.md`（收敛到单一 agent 运行时）、
> `.design/dashboard.md`（板块式前端页面的模板）。

---

## 1. 动机与定位

Tingly-Box 已有三大件：LLM gateway（数据路径）、remote（远程控制 / IM 交互）、
guardrails（护栏）。Task 板块是把三者拧在一起的功能，而不是旁挂的第四个模块：

- 用户需要在 tb 内定义**一次性命令、定时任务、多步 loop**，并接入
  **不同的执行器**（Claude Code、shell，将来 codex / gemini cli 等）。
- 这与 Claude 原生的 tasks / Routines / workflows 有表面重叠——这并不意外，
  也不构成问题。tb 的 task 有三个 Claude 原生**结构性做不到**的点（§3）。

**一句话定位：tb 的 task 板块是异构执行器的控制平面（control plane）——管
when / where / with-what / how-much / who-approves；executor 是数据平面
（data plane）——管 how。**

## 2. 核心边界原则：run 之间 vs run 之内（anti-套娃）

套娃的根源是层次不清：tb 里做一个 agent loop，loop 里调 Claude，Claude 里
又有自己的 loop 和调度。避免的办法不是少做功能，而是**把边界钉死在一次 run
（一次 executor 进程调用）上**：

- **run 之内**发生的一切——agentic 推理、多步规划、fan-out 子 agent、
  工具调用——全部属于 executor。tb 不碰、不模仿、不包装。
- **run 之间**的一切——什么时候触发、在哪个 workspace 跑、用哪个
  executor/模型、跑完通知谁、花了多少钱、失败了要不要再来——全部属于 tb，
  且**不下放给 executor**。

判断任何候选功能的归属，只问一句：**"这件事发生在 run 之间还是 run 之内？"**

四条守则：

1. **tb 不在任务层再造 agent loop。** 一个 step = 一次 executor 调用；
   控制流是确定性的（线性 steps + 条件跳过 + 重试 + until），智能留给 executor。
   如果发现自己在 tb 里写 `while + LLM 判断 + 分支`，说明这段智能该下沉进
   executor（写进某个 step 的 prompt）。
2. **不复用 executor 内部的调度器。** tb 的定时任务绝不通过 Claude 原生
   Routines 实现；反过来也不引导用户把"每小时检查一次"写进 prompt——那段
   调度该上浮回 tb。
3. **executor 无状态化边界。** tb 提供 workspace + 输入（prompt/args/上一步
   产物）；executor 返回结构化结果（status / summary / artifacts）。tb 不保存、
   不理解 executor 的内部会话状态，续跑只记原生 handle（如 `claude -r
   <session-id>`）。这条守住了，加 executor 永远是薄适配。
4. **深度编排不做成 tb 概念。** fan-out 多 agent、评审汇总这类编排是
   "executor 一次 run 内部的事"。tb 的编排能力上限刻意停在：线性 steps +
   until 条件 + 重试。需求超出上限时，答案不是增强 tb 的 DSL，而是让
   executor 在 run 内自己 orchestrate。

## 3. 与 Claude 原生编排的关系（为什么不是重复造轮子）

1. **跨 executor。** Claude 的 cron 只能调 Claude。tb 的 task 天然可以调
   claude、codex、gemini cli、裸 shell——一个统一面板看所有自动化，是任何
   单一 vendor 工具给不了的。
2. **tb 在数据路径上。** task 的 run 走 tb gateway，每个 run 的 token 用量、
   成本、路由路径天然可归因（§6.6）。"这个定时任务这个月花了 $23" 是
   Claude 原生给不出来的视角。guardrails 可以按 task 生效：预算上限、
   越界自动停。
3. **remote 是现成的人机回路。** run 跑到一半需要人拍板（危险操作、审批、
   二义性）时，经由 `remote/channel` 把问题推到 IM，人回复后继续——remote
   管交互式会话，task 管 headless run，两者共用 interaction/channel 层，
   run 需要人时**升级**成一次交互。

推论：Claude 原生编排能力越强，tb 这层控制平面的价值越大——它调度的单元
更有能力了。重叠只发生在词汇表面（"定时""循环"），不发生在职责上。

## 4. 领域模型

### 4.1 三条正交轴，而不是三种任务类型

按 ux-principles #2（消解模式选择）与 #4（正交轴分离），**不做**
"一次性任务 / 定时任务 / loop 任务"三种类型——那是一个旋钮控制多件事。拆成：

- **TaskDef（定义）**：跑什么（steps）+ 在哪跑（workspace）+ 用什么跑
  （executor，模型走 tb 路由）+ trigger + repeat + 预算 + 通知目标。
- **Trigger（何时跑，TaskDef 的属性）**：manual / cron / 一次性定时（at）/
  事件（webhook、IM 消息、gateway 事件——后期）。"一次性命令"不是一种任务
  类型，就是 trigger=manual 且立即触发的 TaskDef——历史统一，跑过的东西
  可一键重跑（ux #10：完成 ≠ 锁死）。
- **Run（实例）**：一次 executor 调用的记录——状态、日志流、结构化结果、
  成本、产物（diff / PR 链接 / 报告路径）。

"loop（多步）"是两个被口语混在一起的概念，必须拆开：

- **Steps**：一轮（iteration）内的确定性流水线，每个 step 一次 executor
  调用，可异构（step1 shell，step2 claude）。控制流是**哑的**：顺序执行 +
  `when` 条件跳过 + 每 step 重试，仅此而已。
- **Repeat policy**：跨轮的重复（`until` 条件 + `max` 上限），条件从 run 的
  **结构化结果**判定，不引入任何 LLM 判断层。

### 4.2 定义示例

UI、将来的 CLI / 文件形式共享同一语义：

```yaml
name: nightly-deps
workspace: ~/code/myrepo
trigger: cron "0 3 * * *"        # 不填 = manual（立即/手动）
steps:
  - name: bump
    executor: shell
    run: pnpm up --latest && pnpm test 2>&1 | tee .tb/test.log
    on_fail: continue             # 失败不终止本轮，交给下一步
  - name: fix
    executor: claude
    when: steps.bump.failed       # 条件跳过：上一步成功就不跑
    prompt: 测试失败，日志在 .tb/test.log，修复并让全部测试通过
  - name: report
    executor: claude
    prompt: 总结本次改动写入 .tb/report.md
repeat:
  until: steps.bump.succeeded     # 整组 steps 重复，直到条件满足
  max: 3
budget: $2                        # guardrail：整个 task 实例的成本上限
notify: telegram
```

### 4.3 运行时语义

- **每个 step = 一条独立的 Run 记录**，同一 workspace 内串行执行
  （引擎现成的 SerializationKey 保证）。每个 step 有自己的日志、状态、成本。
- **step 之间不传对话上下文**，只传两样：workspace 里的文件（如
  `.tb/test.log`）和上一步的结构化结果（`steps.bump.failed` 这类字段级引用）。
  fix 步的 Claude 是一次全新的 headless run，它需要知道的都在磁盘上——
  刻意如此，tb 不理解、不搬运 executor 的会话状态（守则 3）。
- **`when` / `until` 只允许引用结构化结果字段**（status、exit code、
  executor 返回的 summary 字段）。"让 LLM 判断要不要继续"不属于这里——
  那种判断写进对应 step 的 prompt，让它在 run 内自己决定（守则 1）。
- **`repeat` 包在整组 steps 外面**：一轮跑完 → 判 `until` → 不满足且未到
  `max` → 物化下一轮 runs。板块里呈现为"第 2/3 轮，卡在 bump"。
- **引擎不感知 steps / when / until / repeat**——这些是定义层的解释器；
  引擎只见到一条条带串行键、可延迟调度的 run（§6.2）。

## 5. Executor 契约

```
Handler（run 级唯一执行契约，即现有 task.Handler 的演进）:
  Type() string
  Run(ctx, run, controller) → 流式日志 + 结构化结果 {status, summary, artifacts}
  能力声明: 可流式? 可恢复(原生 session handle)? 可交互升级? 结构化输出?
```

- **收敛为一套插件模式**（ux #3：一个概念一个词）。现状有两套并存：
  `task.Handler`（run 级）和 `internal/remote_control` 的
  `AgentRouter → AgentExecutor`（IM 专用）。目标态：`task.Handler` 是唯一
  的 run 级契约；IM 侧 AgentRouter 后期改为**创建 task 而不是直接执行**——
  顺便解决它"每 chat 同时只能跑一个、无排队"的硬限制，并让 IM 触发的 run
  自动进入统一历史与成本归因。
- **ClaudeCode Handler**：内部包 `agentboot.AgentService.Run()`（headless，
  `agentboot` 已预留 Codex / Gemini 的 AgentType 位置）。启动时注入
  run 归因标（§6.6）与 workspace；结果取 agentboot 的最终摘要 + 会话
  handle 存档。
- **Shell Handler**：exit code → status，stdout/stderr → 日志，约定
  `.tb/result.json`（可选）→ 结构化结果。
- 交互升级（后期）：Handler 声明 `interactive` 能力后，run 内的提问经由
  task scenario 插件走 `channel.Prompt`（§6.4），人答复后续跑。

## 6. 落到现有代码

### 6.1 复用休眠的 run 引擎（不动内核）

`internal/task/` 已是完整的 run 引擎：状态机
（pending→queued→running→{succeeded,failed,cancelled,interrupted}）、
`Handler` 注册表、轮询调度器（含延迟一次性）、SerializationKey 串行队列、
重试/退避、取消、崩溃恢复；GORM/SQLite 存储（`internal/data/db/task_store.go`）
已迁移。它当前零生产消费者——本设计让它上岗，且**内核不改**。

### 6.2 Task/Run 拆分（最大的一处结构调整）

现有 `task.Task` 一条记录既是定义又是实例。拆法：

- 现有 `internal/task` 原样作为 **Run 引擎**（它的名字里 "Task" 实为本设计
  的 Run；重命名与否见 Open questions）。
- 其上新增**定义层**：`TaskDef` 实体（steps / trigger / repeat / budget /
  notify）+ 解释器。cron 到点或手动触发 → 从 TaskDef **物化**一轮 run 记录
  逐条 submit；`ParentTaskID` 回链定义（"Phase 4" 预留字段的归宿）。
- 缺件清单：cron 解析、完成后按 repeat 重新物化、steps/when/until 解释器。

### 6.3 API 模块

新建 `internal/server/module/task/`，按 scenario 模块的三件套配方
（`handler.go` / `routes.go` / swagger 元数据）注册进
`server_webui_api.go`；`task.Manager` 在 server lifecycle 中构造并
`Start()`（存储用现成的 `StoreManager.Tasks()`）。前端跑 `task codegen`
生成 client SDK。

### 6.4 通知与审批：复用 remote 层

`remote/channel`（imchannel / autochannel）+ `remote/interaction`
（notify / confirm / choose / ask）即交付层。照
`remote/scenario/builtin/claudecode` 插件的模式（push 事件 → `rt.Notify`；
交互事件 → `channel.Prompt` + interaction registry 长轮询 resolve）写一个
**task scenario 插件**：run 完成 → Notify（附产物，见 §7）；run 中途提问 →
Prompt。

### 6.5 前端

新 Task 页面，参照 dashboard 页的板块做法（§7 为信息架构）。API 未生成前
按惯例留 placeholder。

### 6.6 成本归因接线

ClaudeCode Handler 启动 agentboot 时把 **run ID 作为归因标**注入
（agent 流量本来就走 tb gateway），usage tracking 侧按该标聚合到 run / task。
这是差异化点里工程量最小的一个，但 Handler 契约要从第一天就带上这个字段。
budget guardrail 基于同一聚合：超限 → cancel 当前 run + 终止本轮。

## 7. UI：一个板块回答三个问题

按 ux-principles #1，页面围绕用户脑中的问题组织，而不是"任务列表 CRUD"：

- **现在在跑什么？** — 活跃 run：实时日志 / 状态 / 当前轮次与 step。
- **跑完的结果怎么样？** — run 历史：结果摘要、**可点的产物**
  （PR / diff / 报告路径——ux #11，给物件不给通知）、单 run 成本。
- **接下来会跑什么？** — 定时任务的 next-run、（后期）事件任务在监听什么。

创建入口一个 dialog（ux #2，无前置类型选择）：

1. 默认单条指令 + 单 executor（executor 智能默认：配置过 Claude Code 就
   默认它；workspace 默认当前——ux #6）。80% 场景填完点 Run 结束。
2. **"+ Add step"** 把表单从"一条指令"展开成步骤列表——每 step 独立选
   executor 和指令。
3. 高级折叠区：trigger（默认 now）、repeat（until + max）、budget、notify。

其余对照项：展示真实 cron 表达式与下次触发的具体时间而非"每天"别名
（ux #5）；跑完的 task 可展开、可一键重跑（ux #10）；run 日志滚动限定在
run 面板内（ux #12）。

## 8. 分期建造

1. **Phase 1 — 骨架成立**：TaskDef 定义层 + cron（在引擎上加，不动内核）；
   ClaudeCode Handler（包 agentboot）+ Shell Handler；单 step；
   `module/task` API + 前端板块；run 级成本归因；结果经 channel 推 IM。
   ——做完这一步，三个差异化点（跨 executor、成本可见、远程可达）全部成立。
2. **Phase 2 — 多步与循环**：steps / when 解释器；repeat（until + max）；
   budget guardrail。
3. **Phase 3 — 人机回路**：交互升级（run 中途经 IM 提问）；事件 trigger
   （webhook / IM / gateway 事件）。
4. **Phase 4 — 入口收敛**：IM AgentRouter 改造为 task 入口（IM 指令 =
   创建 trigger=manual 的 task），移除单 chat 单执行限制。

## 9. 明确不做（上限即特性）

- 不做 DAG / 并行分支 / fan-out——超出"线性 steps + until"的编排下沉进
  executor 的一次 run。
- 不做 LLM 条件判断（"让模型决定要不要继续"）——写进 step prompt。
- 不做跨 step 的会话延续——状态传递只有 workspace 文件 + 结构化结果字段。
- 不代管 executor 内部调度（不创建 Claude Routines，反之亦然）。

这些"不做"是边界原则的直接推论，出现相关需求时先回到 §2 的判断句。

## 10. Open questions

- **命名**：`internal/task` 的 `Task` 实为本设计的 Run。是重命名引擎类型
  （侵入大、语义清晰）还是保留并在定义层用 `TaskDef` 区隔（侵入小、
  两个 "Task" 并存违反 ux #3）？倾向后者起步、稳定后统一重命名。
- **workspace 隔离**：并发 run 撞同一 repo 时是否引入 worktree-per-run？
  Phase 1 用 SerializationKey 串行规避，观察真实需求再定。
- **定义的文件形态**：是否支持 repo 内 `tb-task.yaml` 声明式定义
  （版本可控、可 review）？语义已按此预留（§4.2），入口后置。
