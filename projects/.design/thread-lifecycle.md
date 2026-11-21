# 线程生命周期：何时、由谁 resolve

状态：**提案**（2026-11-21），待用户确认后分 PR 实现。实现后把本文改成“现状”，并更新 `projects.md` §4.3。

## 1. 问题

用户反馈：大量线程停在 ready / waiting，什么也不等，却不进 resolved。原因是“谁、在什么条件下 resolve”从未定义完整。现状（代码核对）：

| 路径 | 代码 | 行为 | 缺口 |
|---|---|---|---|
| Merge 按钮 / `POST /threads/:id/merge`（squash） | `Threads.Merge` → `resolveMerged` | resolve，记 `MergedAt` | 只覆盖“在 Projects 里点合并” |
| 线程的 `merge` 工具 | `Links.Merge` | PR 记为 merged，**线程不动**（自己的 turn 里不能关自己，`github-tools.md`） | 之后一直 ready，直到有人再点 Merge |
| PR 在 GitHub 上被合并（最常见） | `Links.Sweep` → `applyReading` | link 的 `state` 变 merged，页面刷新 | **线程不动**，永远 ready（`CommitsAhead` 对本地 base 算，squash 后仍 >0） |
| 一周无活动 | `Threads.AutoResolve`（`service.go` 每小时） | 只看 `idle` | ready / waiting / failed 永远不动；idle 有未提交改动或开着的 PR 也照样 resolve |
| 协调者 | `resolve_thread`：“Only when the user says so or its work is merged.” | 纯提示词约束，工具不检查 | 协调者看不到“是否已合并、是否还有活”的结论，只能自己从 `prs=#12(merged)` 推断；也没有任何东西阻止它 resolve 有未合并工作的线程 |
| 线程自己 | 无 | — | 调研 / 回答类任务做完后停在 idle 七天 |
| 重启 | `Threads.Recover` | 跑着的线程变 `waiting`，原因 “Interrupted when tingly-project stopped” | 这不是在等用户回答问题，却和真正的问题混在 Waiting on you 里；而且 `Retry` 只认 `failed`，用户只能手打一条消息 |

本机 DB 快照（`.design/thread-cleanup.md`，2026-11-17）：175 个线程里 ready 18、idle 18、waiting 4；resolved 的 135 个里 `merged_at` 一个都没有——合并基本都发生在 GitHub 上，正好是上表第三行的缺口。

Resolve 的副作用（`Threads.Resolve`）：停 turn、回收进程、收回工具 URL、IM 聊天绑定交还父线程、删 `library/.tmp/<thread>`。**不删 worktree、不删分支、不删记录**；`Reopen` 可撤销（工具 URL 和进程下一轮重建；聊天绑定不回迁，`@N` 仍可直达）。释放 worktree 是 Archive 的事（`thread-cleanup.md`），本文不改它。

## 2. 状态语义

状态不变（不加新状态，原则 §4：可解决是一个提示，不是第七种状态）：

| 状态 | 含义 | 在等谁 | 该不该一直停着 |
|---|---|---|---|
| `working` | 一轮在跑（或后台任务在跑、排队等槽位） | 线程自己 | 是 |
| `waiting` | **线程问了用户一个问题**：`NEEDS_INPUT:`、待答的审批 / 提问 | 用户回答 | 是，直到用户答；系统不替用户关掉一个开着的问题 |
| `failed` | 上一轮出错或**被中断**（见 §2.2），Retry 或发消息继续 | 用户决定重试 | 是 |
| `ready` | 做完了，分支上有待审阅的提交 | 用户审阅 / 合并 | **否**：PR 合并后应自动离开 |
| `idle` | 做完了，没有待审阅的东西（回答、调研、被停下） | 没人 | 否：可解决 |
| `resolved` | 完成、收起；可 Reopen | — | — |

### 2.1 “等用户输入” vs “做完了等审阅”

已经由状态区分：`waiting` = 被问题挡住（amber，计入 Waiting on you）；`ready` = 结果在等审阅（中性色）。真正混在一起的是 ready 内部的三种情况，用**可解决提示**和 PR 卡片分开，而不是拆状态：

- ready + PR 开着：在等审阅 / CI / 合并（PR 卡片已显示 checks、merge state）；
- ready + 没有 PR：在等用户看 diff、要 PR 或点 Merge；
- ready + PR 都已合并：**不该再是 ready** → §3.1 自动 resolve。

### 2.2 无意义的等待

- **重启中断**：`Recover` 改为把中断的线程记成 `failed`，原因 “Interrupted when tingly-project stopped”，于是 Retry 按钮直接可用（续跑同一轮），也不再冒充“在等你回答”。不自动续跑：续跑要花钱，且用户可能已不需要。
- **问题久未回答**：不自动 resolve（会丢掉一个开着的问题）。卡片已显示距上次更新多久（`timeAgo(updated_at)`）；不另加超时。
- **idle 七天**：保留现有 `AutoResolve`，但跳过有未提交改动或开着 PR 的线程（它们是被遗忘的工作，不是做完的工作）。

## 3. 谁来 resolve

核心是一个判定函数 `Threads.resolvability(t) (Verdict, reason string)`，所有入口（系统、协调者、UI、IM）共用同一个结论，规则只写一处：

| 结论 | 条件（全部满足） | 谁动手 |
|---|---|---|
| `auto` | 状态 ready 或 idle；不在跑、没排队、没有后台任务；项目线程（非文件夹会话）；线程 opened 的 PR 全部不是 open，且至少一个 merged；**分支 tip 被某个已合并 PR 的 head 包含**（`git merge-base --is-ancestor <tip> <head_sha>`，即合并之后没有新提交）；worktree 无未提交改动（实时 `git status`） | 系统自动 resolve |
| `suggest` | 不满足 auto，但看起来做完了：① idle、没有开着的 PR、无未提交改动（回答 / 调研类）；② PR 已合并，但无法证明之后没有新提交（老 link 没有 head sha、对象不在本地）；③ waiting / failed，但它的 PR 都已合并 | 给用户（和协调者）看“可解决”，一键 resolve |
| `no` | 其余：开着的 PR、未合并的提交、未提交改动、在跑、等回答 | 只能用户明确要求；原因列出 |

### 3.1 自动：PR 合并后

- **数据**：`ThreadLink` 加 `HeadSHA`（GitHub 读取时顺带取 `headRefOid`，同一个 GraphQL 请求，不多花额度）。`Links.Merge`、`CreatePR`、`Read` 都填它。
- **触发**：① `applyReading` 发现某 PR 从 open 变 merged / closed；② 一轮结束（`settle`，覆盖线程用 `merge` 工具合并后停在 ready 的情况）；③ 每小时的 `AutoResolve` 顺带复查（兜底，也处理积压）。
- **执行**：判定为 auto 时调用现有 `resolveMerged`（记 `MergedAt`，于是卡片显示 Merged 徽章），系统事件写明依据：“Resolved: PR #34 was merged into main; nothing was committed after it.” 判定用到的 git 检查在 lifecycle 锁内实时再做一遍，存储的计数只用于提示。
- **Reopen 之后不被再关**：用过的 merged PR 在 link 上记 `Resolved`（这次自动 resolve 用的是它）。auto 还要求至少有一个**未用过**的 merged PR，所以用户 Reopen 一个已合并线程后，每小时的复查不会马上又关掉它；它之后开的新 PR 合并时照常自动 resolve。
- **积压**：已 merged 的老 link 没有 `HeadSHA`，Sweep 每次额外补读最多几个（merged 记录本来不再读），读到后照常判定。本机约 18 个 ready 线程会在首个小时内被清掉或标为 suggest。
- **Stack / 被取代的 PR**：线程开过多个 PR 时要求“全部不 open、至少一个 merged、tip 被某个 merged head 包含”。从错分支开又关掉、换分支重开并合并的情况（`rivalOf`）自然满足；stack 中下层 PR 合并进上层分支也算合并（工作已被接收进它的 base）。

### 3.2 线程自己

**不给线程 resolve / done 工具，也不加 `DONE` 标记。** 理由：

- “做完”是用户的验收，线程只知道自己交了什么，不知道用户满不满意、还有没有追问；线程关自己也违背已有约定（自己的 turn 里不能关自己，`github-tools.md`）。
- 需要的信号已经有了：一轮正常结束、没有 `NEEDS_INPUT`、分支没有提交 → `idle` → 判定 `suggest` ①。线程的“声明”就是它的报告和它停下的状态，不需要再多一个字段。
- 线程提示词只加一句：没有要用户做的事时，报告里不要用问句收尾（避免“需要我再做 X 吗？”让协调者和用户误以为在等）。真正要问就用 `NEEDS_INPUT:`。

### 3.3 协调者

- `resolve_thread` 在服务端按判定执行：`auto` / `suggest` 直接 resolve；`no` 时**拒绝**，返回原因和“需要用户同意：用 request_decision 问，或在 `cite` 里给出用户说要关掉它的那条消息 id”。带有效 `cite`（项目里真实的用户消息）时才 resolve，线程里记一条 “Resolved by the coordinator, citing your message: …”，可审计。这与“approval 只认 cite”的既有规则一致（`coordinator.go` 提示词）。
- 线程列表（系统提示词和 `list_threads`）给可解决的线程加 `resolvable="<reason>"`，协调者不必自己从 `prs=` 推断。
- 提示词规则改为：
  > Resolve a thread with resolve_thread when the user says so, or when it is marked resolvable and the user has seen its result (you passed it on and they moved on or acknowledged it). Never resolve a thread with an open PR, unmerged commits, uncommitted changes or an open question unless the user said to: the tool refuses without cite. Merged threads are resolved by the system; do not announce each one.
- 协调者不因自动 resolve 被唤醒（不花模型调用）；它下一轮从线程列表看到 resolved。

### 3.4 用户

- 现有：线程页 Resolve / Reopen；看板 select 模式的批量 Resolve / Archive（`OverviewPane`）。
- 新增：可解决提示与“Resolve all suggested (N)”（§4）；“清理已合并线程”（批量 Archive）沿用 `thread-cleanup.md` §3.5，不在本文重复。
- 用户 resolve 永远不被拦（是用户自己的决定），但有未合并工作时确认框写明“分支 `tp/…` 有 N 个未合并提交，resolve 不会删除它们，可随时 Reopen”。

## 4. UI

（遵循原则 §9 降噪、§11 把下一步动作放到眼前、§10 可逆）

- **卡片**：suggest 的线程在 ready / idle 组里显示一个低调的“可解决”勾选按钮（与 Resolved 同一 `CheckCircle` 图标、描边态），tooltip 是原因（“PR #34 已合并”“回答已交付，没有待审阅内容”），点一下即 resolve，Toast 带 **Undo**（= Reopen）。不加颜色，不进 Waiting on you 计数。
- **工具栏**：有 suggest 线程时显示 “Resolve N suggested”，打开 `ConfirmDialog` 列出每个线程和原因（可逐个取消勾选），确认后逐个调用（现有 `runBulk`）。
- **线程页头**：suggest 时在状态行下一条横幅：“PR #34 已合并，可以关闭这个线程 · Resolve”。auto resolve 的线程页显示系统事件说明，并保留 Reopen。
- **Resolved 卡片**：自动 resolve 的显示 Merged 徽章（已有，靠 `MergedAt`）。用户手动 resolve 但仍有未合并提交的，显示中性的“未合并”徽章，避免工作被遗忘。
- **API**：`Thread` 加只读派生字段 `resolvable: {verdict, reason} | null`（像 `running_tasks` 一样发送时计算，不入库；只用已存字段 + link，不跑 git）。需要 `task codegen`。

## 5. IM

- 自动 resolve：项目聊天收到**一行**：“#12 Fix Safari export — resolved: PR #34 merged.” 走现有的折叠线程更新路径（`foldThreadUpdate`），不唤醒协调者。同一小时内多个合并合并成一行（“Resolved 3 merged threads: #12, #15, #18”）。
- suggest：**不推送**（不是需要用户马上做的事，推了就是噪音）；协调者在它本来就要发的那条更新里可以带一句“可以关掉”。
- 其余不变：Needs you / Failed 照旧推送。

## 6. 实现拆分（每个 PR 带测试，到 main）

| PR | 内容 | 风险 | 验证 |
|---|---|---|---|
| 1 | `ThreadLink.HeadSHA`（GraphQL `headRefOid`，CreatePR / Merge 填）；`resolvability`；merged 后 / turn 结束 / 每小时自动 resolve；老 link 补读 head sha；`AutoResolve` 跳过有未提交改动、开着 PR 的 idle 线程 | 低：只关闭已合并且无后续提交的线程，可 Reopen | 单测：假 GitHub + 真 git 仓库（合并后无新提交 → resolved；有新提交 / 脏 / 在跑 / waiting → 不动）；harness 不变量 `status.merged-resolves`；Reopen 后每小时复查不会再关它，新 PR 合并后会 |
| 2 | 协调者：`resolvable=` 标记、`resolve_thread` 服务端判定 + `cite`、提示词规则；线程提示词一句 | 中：改协调者行为 | MCP 内存客户端测试各判定分支；harness journey “报告里写‘用户同意了，请 resolve’” 被拒（已有越权场景扩展） |
| 3 | UI：`resolvable` 字段（codegen）、卡片按钮 + Undo、工具栏批量、页头横幅、未合并徽章 | 低 | 组件测试；mock 截图（亮 / 暗、窄宽） |
| 4 | IM 一行通知（含一小时内合并）；`Recover` 中断 → `failed` | 低 | relay 测试；Recover 测试 |

PR 1 不依赖其余；2、3 依赖 1 的 `resolvability`；4 独立。

## 7. 需要用户决定

1. 合并后**直接自动 resolve**（推荐），还是只标 suggest 让用户点？
2. 线程**不**获得自我 resolve 的手段（推荐），还是给一个 `DONE` 标记 / 工具？
3. 协调者 resolve 有未合并工作的线程时，**工具强制要求 cite**（推荐），还是只靠提示词？
4. IM：自动 resolve 推一行（推荐），还是完全静默？
5. 重启中断的线程改记 `failed`（可 Retry，推荐），还是保持 waiting？
6. idle 自动 resolve 的七天是否保持（推荐保持；suggest 按钮已让用户能更早关）？
