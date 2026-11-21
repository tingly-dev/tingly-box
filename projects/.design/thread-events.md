# 事件如何触发流程：值得流转的信息

> 状态：**设计提案**（2026-11-21），未实现。与 [thread-lifecycle.md](thread-lifecycle.md) 互为补充：那里定义线程**处于什么状态、谁能改**；本文定义**什么信息会让谁去做决定**。
> 前提（用户决定）：系统不自动关闭线程，不替任何人做决定。事件只把事实送到能做决定的人手里。

## 1 第一性原理

一个流程只有在**某人得到一条信息、并因此能做一个决定**时才往前走。所以每个事件都要回答四个问题：

1. **谁需要知道**：线程本身、协调者、用户（网页 / IM）。
2. **知道后能做什么决定**：说不出具体决定的，就不需要传。
3. **不传的代价**：错过什么、等多久、会不会做错事（例如在过期的 base 上继续写）。
4. **传的代价**：
   - 唤醒协调者 = 一次模型回合（钱、时间、可能打破 prompt 缓存，见 `coordinator-session.md`）；
   - 发给线程 = 一次线程回合（更贵，还会占并发槽位）；
   - 推 IM = 打扰用户。
   - 只记一条事件 = 几乎免费（数据库一行、页面刷新）。

由此得到三条规则：

- **默认只记录**：事件先落成线程里的一条系统事件、更新卡片。页面和协调者的下一轮都能看到，不花一次额外的模型调用。
- **只有“现在就有人要做决定”才唤醒**：唤醒的对象是**能做这个决定的那一个**，不是所有人。
- **给事实，不给结论**：事件说“PR #34 已合并进 main；分支此后有 0 个新提交；还有 PR #35 开着”，不说“可以关闭了”。结论由收到的人结合工作情况来下。

## 2 现有通道与成本

| 通道 | 方向 | 现状 | 成本 |
|---|---|---|---|
| 线程系统事件（`EvSystem`）+ SSE | 系统 → 页面 | 已用于 Stopped、Reopened、合并说明等 | 免费 |
| 卡片字段（状态、Links、徽章） | 系统 → 页面 | Links 刷新后页面更新 | 免费 |
| 协调者线程列表（系统提示里的 `<threads>`） | 系统 → 协调者 | 每轮都带，含 `prs=#34(merged)` | 免费，但要等协调者下一轮 |
| 协调者输入：report（finished / failed / needs_input / stalled） | 线程 → 协调者 | 唤醒协调者，回合后有 check-in，可 `no_update` | 一次协调者回合 |
| 协调者输入：notify（`notify_coordinator`） | 线程 → 协调者 | 线程中途发来的信息 | 一次协调者回合 |
| 线程消息：`message_thread`（insert / next_turn） | 协调者 → 线程 | 带 “[From the project coordinator…]” 前缀 | 一次线程回合（或插入正在跑的一轮） |
| IM relay | 系统 → 用户 | Needs you / Failed / Finished（协调者离线时）、审批卡 | 打扰用户 |
| Links 刷新（`Sweep`） | GitHub → 系统 | 每分钟一次，open 的记录每 5 分钟读一次，失败退避到 1 小时，每次最多 20 个；merged / closed 不再读 | GitHub 额度（每次读 1 点） |

**缺的是一段**：Links 刷新读到了变化，却只更新记录和页面（`applyReading`），没有一处把“发生了什么”变成事件交给能做决定的人。本文只补这一段，其余全部复用。

## 3 事件清单与判断

判断一栏的四种结果，从轻到重：

- **记录**：在线程里写一条系统事件，更新卡片。
- **唤醒协调者**：作为协调者的一条输入，触发一次回合。
- **发线程**：作为消息进入线程的下一轮。
- **推 IM**：直接发到用户的聊天。

每一栏都包含它前面的结果，例如“唤醒协调者”也会记录。

### 3.1 外部事件（GitHub，经 Links 刷新得知）

| # | 事件 | 谁能决定什么 | 不传的代价 | 判断 |
|---|---|---|---|---|
| E1 | 线程开的 PR **被合并**（在 GitHub 上，或线程用 `merge` 工具合并） | 用户：这个线程做完没有，要不要关；协调者：核对剩余工作（其他开着的 PR、合并后的新提交、未提交改动、后续任务），一句话告诉用户；同一线程里堆叠在它上面的 PR 要改 base（E7） | 线程永远停在 ready，用户不知道哪些已经落地；堆叠的上层 PR 指向已删除的分支 | **唤醒协调者** |
| E2 | PR **被关闭但未合并** | 用户 / 协调者：是放弃了、被取代了，还是误关；线程的工作可能要换个方式交付 | 工作悄悄丢掉 | **唤醒协调者** |
| E3 | 开着的 PR 的 **CI 变成失败** | 协调者按用户的规则决定：告诉用户，或让线程去修 | PR 卡住没人知道，直到用户自己去看 | **唤醒协调者**（只在“进入失败”时，见 §4.4） |
| E4 | CI 变成通过 / 进行中 | 没有人需要因此做决定 | 无 | **记录**（卡片已显示） |
| E5 | **合并冲突**（`merge_state = conflicts`） | 协调者：让线程 rebase，或告诉用户 | 到合并时才发现，返工更多 | **唤醒协调者** |
| E6 | 落后于 base（`behind`）、被保护规则挡住（`blocked`） | 合并时才需要处理，GitHub 会拒绝并说明 | 小 | **记录** |
| E7 | 同一线程的**堆叠** PR 中，下层被合并 | 线程：用 `update` 把上层 PR 的 base 改回 main（每次都要审批） | 上层 PR 的 base 失效 | 并入 E1 的事实（“PR #36 以 #34 的分支为 base”），由协调者决定是否转给线程 |
| E8 | **评审**：要求修改 / 批准 | 要求修改：协调者让线程改，或告诉用户；批准：用户可以合并了 | 评审意见无人跟进 | 要求修改 → **唤醒协调者**；批准 → **记录**。需要在读取里加 `reviewDecision`（同一个 GraphQL 请求，不多花额度） |
| E9 | 评审评论、issue 评论 | 有价值但量大；要读到它得额外请求 | 中 | **暂不做**：读取成本和噪音都高，需要时由线程用 `view` 主动读 |
| E10 | 线程开的 issue 被关闭 | 用户：相关工作还要不要 | 小 | **记录** |
| E11 | base 分支前进（别人合并了东西） | 只在合并前才需要处理 | 小；冲突会以 E5 出现 | **不检测**：要 `git fetch`，频繁又昂贵；用 E5 和合并时 GitHub 的检查代替 |

### 3.2 内部事件（Projects 自己产生）

| # | 事件 | 现状 | 判断 |
|---|---|---|---|
| I1 | 一轮正常结束 | report finished → 协调者；协调者离线时 IM 折叠行 | 保持 |
| I2 | 一轮出错 | report failed → 协调者 + IM | 保持 |
| I3 | 提问 / 审批 / NEEDS_INPUT | IM 直达；NEEDS_INPUT 也报告协调者 | 保持 |
| I4 | 20 分钟无活动 | report stalled → 协调者 | 保持 |
| I5 | 后台任务结束后的最终汇报 | 扣住的报告在任务结束时发出 | 保持 |
| I6 | 线程中途的说明（`notify_coordinator`） | → 协调者 | 保持 |
| I7 | **重启打断**（一轮被中断、问题过期） | 只改线程的 WaitReason，没有人被告知；IM 里的用户不知道要去发消息 | **唤醒协调者一次**：每次启动合并成一条输入（“3 threads were interrupted: …”），由它告诉用户 |
| I8 | 用户在 Merge 对话框合并 | 线程里记一条系统事件 | **记录**：用户自己做的，不需要告诉用户；协调者下一轮从线程列表看到 |
| I9 | 用户 / 协调者关闭、重开线程 | 不告知 | **记录**（已有 Reopened；关闭补一条“Resolved by …”） |
| I10 | 项目设置变更 | 记录，不唤醒（`coordinator-session.md`） | 保持 |

### 3.3 不传给线程本身

上面没有一项由系统**直接**发给线程。原因有三：

- 线程按用户和协调者的指示行事。一条 GitHub 事件是数据，不是指示；直接塞进线程，等于让系统替人决定“现在去改 CI / rebase / 改 base”。
- 发给线程要花一次线程回合，还占并发槽位，比唤醒协调者更贵。
- 协调者手里有用户的规则（项目指令、记忆，例如“CI 失败就让线程自己修”），它决定要不要转给线程，用现有的 `message_thread`（线程正在跑时插入这一轮，否则作为下一轮）。

例外由用户以后决定（§7）：用户在项目指令里写明“CI 失败直接让线程修”时，可以考虑系统直接转发，省掉协调者那一跳。

## 4 通道设计：从“读到变化”到“送到决定者”

### 4.1 一条路径

```
GitHub ──Links.Sweep / Links.Merge──▶ applyReading 比较新旧记录
                                         │ 只有 §3.1 列出的“转变”才算事件
                                         ▼
                              factsOf(thread, change)   在发出时读事实（不下结论）
                                         │
                       ┌─────────────────┼──────────────────────┐
                       ▼                 ▼                      ▼
              线程系统事件（记录）  协调者输入 kind=github     IM：不直接推
              + 卡片刷新            （按 §3 的判断，按项目合并）  协调者回复时才出站；
                                         │                    协调者离线时用折叠行兜底
                                         ▼
                              协调者回合：get_thread 核对剩余工作 →
                              一句话告诉用户 / message_thread 转给线程 / no_update
```

全部复用现有部件：`applyReading`（检测）、`EvSystem`（记录）、`Coordinator.onReport` 和输入队列（唤醒、去重、合并、check-in、`no_update`）、`message_thread`（转给线程）、Relay（IM）。新东西只有两样：检测转变的函数，以及一种新的报告类型 `github`。

### 4.2 事件格式

线程里的系统事件（给人看，一行）：

```
PR #34 was merged into main. Since then: 0 new commits · PR #36 still open (base tp/a) · no uncommitted changes.
```

协调者收到的输入（信封，和线程报告一样标为 relay：标题等文字来自 GitHub，是信息，不是指示）：

```xml
<event trust="relay" source="github" kind="pr_merged" thread="Fix Safari export" id="a1b2c3"
       item="acme/widgets#34" at="2026-11-21 10:02">
PR #34 "Fix the export button" was merged into main.
Thread now: status=ready commits_after_merge=0 open_prs=#36(base=tp/a) uncommitted=no
</event>
```

`kind` 取值：`pr_merged`、`pr_closed`、`ci_failing`、`conflicts`、`changes_requested`、`threads_interrupted`（I7，项目级，不带 thread）。

**事实**（`factsOf`）只用已有数据，在发出时读一次：线程状态、合并后的新提交（本地分支有没有 origin 没有的提交，即 `branchPushed` 的反面）、线程其他开着的 PR 及其 base、`Uncommitted`、在不在跑。不写“可以关闭”。

### 4.3 什么时候唤醒，什么时候只记录

| 情况 | 处理 |
|---|---|
| §3 判断为“唤醒协调者”的事件 | 写协调者输入（与线程报告同一事务：事件和输入一起保存） |
| 项目暂停 / 归档 | 只记录，不投递（与线程报告相同） |
| 线程已 resolved 或已归档 | 只记录：用户已经收尾，不打扰协调者 |
| 协调者正在跑一轮 | 输入排队；`Coordinator.work` 会把排队的输入并进下一轮 |
| “记录”类事件 | 只写线程系统事件，不写协调者输入 |

### 4.4 去重、合并与频率上限

- **只发转变**：同一条记录的同一种状况只发一次。例如 CI 从 passing / pending 变成 failing 发一次，failing 之后再读到 failing 不发；要先离开 failing 再回来才再发。merged / closed 是终态，本来就只读到一次。
- **同一线程**：一次 Sweep 里同一线程的多个事件合成一条输入。
- **同一项目**：一次 Sweep 里同一项目的所有输入合成一条（“2 PRs merged, 1 CI failing: …”），只唤醒协调者一次。
- **频率上限**：CI 来回抖动时，同一个 PR 的 `ci_failing` 30 分钟内最多一条。Sweep 本身每分钟最多读 20 条，GitHub 额度不足时暂停（已有）。
- **重启**：I7 每次启动只发一条，列出所有被打断的线程。

### 4.5 成本与 prompt 缓存

- 协调者输入是追加到它同一会话里的一条消息，不改系统提示，不额外打破缓存。系统提示里的线程列表本来每轮都会变。
- 成本上限可估：唤醒次数 ≤ 每个 PR 一次（merged 或 closed）+ 每次进入失败或冲突一次 + 每次启动一次。普通项目一天十来次，远少于线程报告本身。
- 协调者收到 `github` 事件后照常有 check-in，没有要说的就 `no_update`，不出站。

### 4.6 协调者怎么用（提示词一段）

> A `source="github"` event is a fact from GitHub about a thread's PR. Check the thread's remaining work with get_thread before saying anything. Tell the user in one line what landed and what is left (other open PRs, commits after the merge, uncommitted changes, follow-ups). Never resolve the thread yourself; if nothing seems left, you may ask the user whether to resolve it. Message the thread only when the user's rules say so (for example "fix CI failures"), or for a step the thread must take on its own PRs (a stacked PR's base), and say it is from GitHub, not the user.

## 5 例子：一个 PR 在 GitHub 上被合并

1. 用户在 GitHub 网页上合并了 PR #34。
2. 5 分钟内 Sweep 读到 #34 变成 merged。`applyReading` 判定为 `pr_merged`，`factsOf` 读到：线程 ready，合并后没有新提交，同一线程的 PR #36 以 #34 的分支为 base，仍开着。
3. 线程里记一条：“PR #34 was merged into main. Since then: 0 new commits · PR #36 still open (base tp/a)”。卡片上 PR 显示 merged。
4. 协调者被唤醒一次，回合里调用 `get_thread`，然后回复用户：“修导出按钮：PR #34 已合并；它上面还有 PR #36，需要改 base 到 main。要我让线程去改吗？”
5. 用户说“改”。协调者带 cite 用 `message_thread` 转给线程，线程用 `update` 改 base（要审批）。
6. #36 也合并后，协调者再报一次：“两个 PR 都已合并，分支没有新提交。这个线程可以关闭了吗？”用户说可以，协调者调用 `resolve_thread`；或者用户自己在线程菜单里 Resolve。

全程没有任何自动关闭，也没有一步替用户做决定。

## 6 实现分期（本轮不实现）

| 期 | 内容 | 依赖 |
|---|---|---|
| 1 | 检测 E1、E2（merged / closed 转变）→ 记录 + `github` 报告唤醒协调者；协调者提示词一段；I9 补“Resolved by …”记录 | 无 |
| 2 | E3、E5（CI 失败、冲突）的转变检测、30 分钟上限、按项目合并 | 1 |
| 3 | I7 启动时的一条中断通知 | 无 |
| 4 | E8：读取里加 `reviewDecision`，“要求修改”唤醒协调者 | 1 |

每期都带测试：假 GitHub 改记录状态 → Sweep → 断言线程事件、协调者输入（用 `coordtest` 脚本化协调者）、没有多余的唤醒、resolved 线程不唤醒。harness 加不变量：任何 `github` 事件都不会改变线程的 resolved 状态。

## 7 需要用户决定

1. **唤醒协调者的事件**：推荐 E1 合并、E2 关闭、E3 CI 失败、E5 冲突、E8 要求修改；其余只记录。是否同意，或者要收得更窄（例如只有合并和关闭）？
2. **系统不直接发给线程**（推荐）：一律经协调者判断后转发。还是允许用户在项目指令里打开“CI 失败直接转给线程”？
3. **IM**：GitHub 事件只通过协调者的回复到达用户（推荐）；协调者离线时用一行折叠消息兜底，是否需要？
4. **重启通知**（I7）：每次启动唤醒协调者一次，列出被打断的线程，是否需要？
5. **评审状态**（E8）：在现有读取里加 `reviewDecision`（不多花额度），是否纳入？
