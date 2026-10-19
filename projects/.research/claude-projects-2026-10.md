# Claude Projects 2026-10 更新：与 10-10 调研相比的增量

调研日期：2026-10-17。这是 [`claude-projects.md`](claude-projects.md)（2026-10-10）的增量，那份里已有的内容这里不再重复。
主要来源是官方文档 [Let Claude coordinate ongoing work with Projects](https://code.claude.com/docs/en/claude-projects)（下称"文档"），它比发布博客细得多。其次是 anthropics/claude-code 的 issue 和第三方报道。
标记：**【文档】** 官方文档原意；**【报道】** 第三方或 issue，未经官方确认；**【代码】** 读自 `projects/internal/project/*.go`；**【建议】** 对我们的取舍建议。

---

## 0. 结论摘要

1. **本地线程已经上线。**【文档】现在可以在对话里选 "Work locally"，让某个线程通过 Remote Control 跑在自己电脑上，其它线程照样在云端。它默认**直接在文件夹里干活**，worktree 要在卡片上手动打开。这正好回答了 #2084。
2. **Claude Code 自己的答案是"改文件前再进 worktree"。**【文档】后台会话一开始在工作目录里，第一次编辑前才移进 `.claude/worktrees/` 下的 worktree，只读的会话永远不建。这比我们"每个线程开局就建 worktree"更合适。
3. **线程把结果报回协调者是官方设计的一部分，不是 bug。**【文档】协调者"看线程报回来的东西，不看每一步"。官方压成本靠的是协调者默认 **low effort**、汇报简短、协调者大多不说话，而不是取消汇报。我们的问题（#2083）在于每一轮结束都触发一次协调者满上下文回合，包括用户在线程里直接对话的那些轮。
4. **"会话转项目"有了现成的交互。**【文档】有 Continue as project 和 Move to project 两个入口，正好是 #2081 剩下的"把一个会话交给某个项目"。
5. **成本的几个硬事实**【文档】：
   - 默认配置是线程 Opus 加 high effort，协调者 Opus 加 low effort。
   - 每天最多新建 200 个线程。
   - 碰到额度上限的线程会等待，额度重置后自动继续。
   - 回到一个闲置超过缓存寿命（Pro/Max 1 小时）的线程，要把整段对话重读一遍，所以官方建议新工作开新线程。

---

## 1. 产品面的新事实（10-10 之后或当时未记录）

### 1.1 时间线
- 2026-10-03 起，社区开始要"本地线程一等公民"（claude-code#99156）。【报道】
- 2026-10-09，waitlist 上的 Pro/Max 用户全部开通。【报道】
- 2026-10-11，有报告说本地线程拿到的项目指令被截断在 8,192 字节（claude-code#101398，文档写的上限是 16,000 字符）。【报道】
- 2026-10-14，帮助中心说旧的 Chat/Cowork 项目开始分批迁移到新版：指令、文件、完整对话历史和定时任务都会带过去，"什么都不会丢"。记忆不在迁移清单里。【报道】

### 1.2 线程跑在哪
【文档】
- 线程默认是云端会话。用户在 + 菜单选 **Work locally**，消息会带上 **Local** 标签，线程就改在用户电脑上的某个文件夹里跑，经 Remote Control 连接。
- 授权卡片叫 "Allow Claude to work in a folder on your device"，用户在上面选文件夹。卡片还会提醒"两个线程同时在一个文件夹里会互相覆盖"，git 仓库可以在文件夹选项里打开 **Worktree**，最后点 **Allow once**。
- 在终端里，`claude remote-control` 用 `--spawn same-dir` 是默认，所有会话共用目录。用 `--spawn worktree` 则每个会话一个 worktree，运行中按 `w` 可以切换。
- 本地线程和云端线程的区别：
  - 本地线程用本机的文件、工具、MCP、settings、hooks 和权限规则。
  - 它会收到项目指令，但**不加载项目记忆**。
  - 电脑睡眠时它会暂停。线程头部的笔记本图标显示连接状态。
- 云端线程每个都是独立沙箱，各自克隆仓库，**不需要 worktree**。两次回合之间沙箱会暂停，恢复失败时从新克隆继续，未提交的改动可能丢失。
- 用户自己在本地开的会话**加不进项目**。社区在要求一个反方向的"伪线程"：本地会话 `/project join`，把状态和回复报给项目（claude-code#100715）。【报道】

### 1.3 Overview 与线程状态
【文档】
- 分组依次是 Ready for review、Waiting on you（含失败）、Working、Landing（PR 已批准或排队合并）、Idle、Resolved。
- 线程会自动 Resolved：用户走完最后一步（比如合并了 PR）时由 Claude 标记，或者一周无活动。
- Overview 有四个 tab：Threads、Library、**Pull requests**、**Routines**。
- 桌面端会推系统通知：协调者发言、线程出错、线程需要输入，都会通知。也可以设成每轮结束都通知，或按项目关掉。浏览器里只有 Overview 按钮上的小点。

### 1.4 线程卡片上的"下一步"按钮
【文档】线程推过分支或开过 PR 后，卡片上会出现按钮。
- **Resolve conflicts / Fix CI / Address comments / Merge it**：以用户的名义把这句话发给线程。
- **Review PR**：打开 GitHub。
- **Create PR**：线程闲着、推了分支但还没开 PR 时出现，直接从分支建 PR，不经过线程。

### 1.5 消息路由
【文档】
- 在线程自己的输入框里写，消息**直接到这个线程**。
- 在项目对话里写的追问，只有协调者判断它属于某个线程时才会转过去。
- 让线程继续的审批只能在线程里回答，在项目对话里说"继续吧"到不了线程。

### 1.6 权限
【文档】
- 线程默认用 **auto 模式**（模型支持时），大多数工具调用不问用户。
- 审批分两档："仅此一次"和"本线程都允许"。
- 单仓库项目会用仓库 `.claude/settings.json` 里的权限规则和 hooks。多仓库项目（Anthropic 托管环境）一个仓库的规则都不用，只靠 auto 模式加线程内审批。

### 1.7 仓库与上下文
【文档】
- 每个线程克隆**项目的所有仓库**，加载每个仓库的 `CLAUDE.md`、skills、agents 和 commands。
- 项目没有的仓库，线程可以中途加给自己，只对这个线程有效。不过它的 `CLAUDE.md` 和 skills 是中途才进来的。
- 官方建议只加几乎每个任务都用到的一两个仓库，其它仓库写进项目指令，让线程按需拉。
- 记忆：每个云端线程开局读 `MEMORY.md` 索引，其它文件**需要时再打开**。
- 项目指令最多 16,000 字符，线程和协调者都会收到。设置改动只对新线程生效。
- 协调者"基于最近消息、最近线程和项目记忆工作，而不是全量历史"，所以能一直跑下去。必须永远保留的东西放进记忆。
- 项目对话本身**没有连接器**，需要连接器的事要交给线程。

### 1.8 建项目与迁移
【文档】
- 新建项目只有名称必填，Goal 和 Context（仓库、文件、文件夹、Drive）都可选。
- 第一个项目建好后，Claude 会自己先跑一轮：开一个只读探索线程提出下一步，并根据最近的云端会话发 **Setup recommendations**（推荐仓库、routine 和线程，默认全勾选，点 Update setup 生效）。
- **Continue as project**：从一个云端会话新建项目，Claude 读这个会话，提出项目配置。原会话保留，正在跑的话继续跑。
- **Move to project**：把一个会话的工作带进已有项目。做法是在那个项目的对话里发一条"读这个会话、接着干"的消息，之后的工作由项目自己的线程做。原会话不变。
- 反过来不行：线程不能移出项目，项目也不能合并。

### 1.9 协调方式可以用话调节
【文档】可以说的话举例：
- "先提议，等我点头再开线程"
- "最多同时两个线程"
- "同一领域的后续复用已有线程"
- "更新短一点 / 只在完成或受阻时说"
- "先告诉我问题，什么都别修"
- "这个就地答，别开线程"

Claude 会自己把这些偏好记进项目记忆。它们是**偏好，不是硬上限**；想要精确措辞、对所有线程生效，就写进项目指令。

### 1.10 用量与成本
【文档】
- 消耗用量的有三部分：
  - 运行中的线程，每个都是完整会话，数量不设固定上限。
  - 协调者阅读线程汇报、决定下一步，本身也花 token。
  - 盯着 PR 的闲置线程，CI 失败或来了 review 评论时会被唤醒。
- 闲着的项目不花钱：没有运行中的线程、没有盯着的 PR、没有新消息。
- 用量页按线程、按模型列出，并单列项目对话花了多少。
- 碰到 5 小时或周额度上限时，线程显示 "Service is busy"，自己重试，额度重置后继续。routine 开的线程例外，会直接报错停下。
- 降本建议有三条：新工作开新线程，不要唤醒过期的大线程；线程或协调者换小模型、降 effort；少开线程，小问题就地答。

### 1.11 排错时体现的设计
【文档】
- 协调者卡住时有 **Restart Claude**，只重连协调者，不影响线程。
- 如果多个线程都猜错或停住，通常是项目配置缺了同一块。官方给的流程是：先让协调者列出每个线程的任务、假设和缺什么，把错的线程 resolve，在指令或环境里补一次，先跑一个线程验证，再重发其余的。

---

## 2. 对照我们的实现

| 主题 | 官方 | 我们（代码） | 建议 |
|---|---|---|---|
| 线程工作区 | 云端：每线程独立克隆。本地：默认同目录，worktree 可选。Claude Code 后台会话在**第一次编辑前**才进 worktree | 有仓库的线程开局就 `git worktree add -b tp/<slug>-<id6>`（`threads.go` Start）。没有 `.worktreeinclude` 之类的步骤 | 见 §3.1 |
| 汇报触发协调者 | 线程报结果，协调者 low effort，大多不说话 | 每轮正常结束都生成 `ReportFinished`，进协调者队列，触发一次满上下文 LLM 回合（`coordinator.go` onReport → turn）。用户直接在线程里聊的轮也一样 | 见 §3.2 |
| 协调者默认强度 | 同模型，low effort | 只有协调者模型，没有 effort | 加协调者 effort（或推荐用小模型），默认低 |
| 线程常驻与缓存 | 缓存寿命 1 小时，过期唤醒要重读，官方建议开新线程 | 常驻进程池闲置 10 分钟就回收（`agents.go`），之后用 `--resume` 重读全文。经过网关时缓存寿命取决于上游（Anthropic 默认 5 分钟） | 进 #2079：测一下 resume 的重读成本，考虑 1 小时缓存，或在协调者路由时偏向开新线程 |
| 权限默认 | auto 模式 | `acceptEdits`（命令要审批，所以 Waiting on you 多） | auto 模式依赖 Anthropic 的分类器，经网关接其它模型不一定可用。可以在连接是 Anthropic 时默认 auto |
| 状态分组 | 多一个 Landing（PR 批准或排队） | 本地合并是同步的，没有 Landing | 维持现状；有 GitHub PR（P2）后再加 |
| 自动 resolve | 合并后，或一周无活动 | 已有（`AutoResolve`） | 一致 |
| 卡片下一步按钮 | Resolve conflicts / Fix CI / Address comments / Merge it / Create PR | 有 Merge it | 本地版可以加 **Resolve conflicts**（合并失败时发给线程）。其余等 PR 功能 |
| 会话转项目 | Continue as project / Move to project | 工作目录会话（原 Desk）还不能转 | #2081 剩下的一步直接照官方：Move to project = 在项目对话里发"读这个会话、接着干"的消息，带会话 id，由协调者开线程接手 |
| 本地线程不加载记忆 | 是 | 不适用（我们的线程都在本地，都加载） | 维持现状 |
| 设置只对新线程生效 | 是 | 混合：settings 文件只在开线程时写一次；系统提示（指令、记忆索引）在每次启动进程时按当前项目重建（`threads.go` 里的 `threadSystemPrompt`），所以常驻进程被回收后，改动会进入已有线程 | 定一个规则写进设计文档。官方的"只对新线程生效"更好预期，也不会打断缓存 |
| 用量 | 按线程、按模型，单列协调者 | 只记线程 token | #2079 第 1 步顺带补上协调者 |
| 额度上限 | 线程等待，重置后自动继续 | 【推断，未测】网关返回 429 时这一轮按失败处理，线程进 Waiting on you | 低优先级。Tingly Box 有路由，可以先换路由 |
| 通知 | 桌面通知：协调者发言 / 出错 / 需要输入 | 没有 | Tingly Box 有 imbot，P2 |
| 每日线程上限 | 200 | 并发硬上限 4 | 本地不需要日上限 |

---

## 3. 对两个开放 issue 的建议

### 3.1 #2084：每个线程一个 worktree 是否合适

**问题**【代码 + 推断】。开局就建 worktree 有这些代价：
- 只读线程（调研、回答、review）也会建 worktree 和分支，留下大量空分支 `tp/...`。
- worktree 是新检出，没有 `.env`、`node_modules` 和构建缓存，线程第一件事往往是装依赖，又慢又费 token。
- 用户看不到线程的改动，也没法在自己正在跑的开发服务里试，必须先合并。

**官方怎么做**【文档】：
- 云端不用 worktree，靠独立沙箱。
- 本地默认同目录，worktree 由用户在卡片上开。
- Claude Code 自己的后台会话做的是**改文件前才进 worktree**，并支持用 `.worktreeinclude` 把被 gitignore 的文件（如 `.env`）复制进 worktree。

**建议（推荐 A + B）**：
- **A. 延迟建 worktree。** 线程开局在项目仓库的主检出里，**只读**。需要改文件时再进 worktree：
  - 可以由线程调用 Claude Code 的 `EnterWorktree` 工具，或者在第一次 Edit/Write/Bash 写操作的审批钩子里由我们建好再放行。
  - 只读线程永远不建分支。
  - 需要验证：stream-json/SDK 模式下 `EnterWorktree` 能不能用，以及 Claude Code 的 bg 隔离（`worktree.bgIsolation`）是否只在 `--bg` 会话生效。
- **B. 处理 `.worktreeinclude`。** 建 worktree 时按仓库的 `.worktreeinclude` 复制被 gitignore 的文件，和 Claude Code 行为一致，也就不用自己发明配置。
- **C（可选）. 项目级开关"直接在仓库里改"。** 对应官方本地的 `same-dir`，适合单线程、或用户想在自己开发环境里立刻看到改动的场景。需要提示"并行线程会互相覆盖"。按 UX 原则（聪明默认，不加开关），这个开关只放在项目设置的 Environment 里，默认关。

### 3.2 #2083：线程要不要每轮都把消息发回协调者

**结论**：总线要留。官方也是线程报给协调者，这是"一个对话就能看全局"的基础。要改的是**什么事件值得唤醒协调者的 LLM**。

**建议**：
1. **状态变化不需要 LLM。** 线程卡片原地变状态就够了（官方 UI 也是卡片实时变）。`ready`、`idle` 这类常规结束只记一条线程更新，不排协调者回合。
2. **只有三类事件唤醒协调者回合**：
   - 需要用户输入（`NEEDS_INPUT`）或失败：用户要知道，而且协调者可能要转述。
   - 协调者开的线程**第一次**完成任务：协调者要汇总，或决定下一步（比如开后续线程）。
   - 线程汇报里明确有给协调者的话。可以约定一个前缀，例如 `FOR_COORDINATOR:`。
3. **用户在线程输入框里直接对话的轮不报给协调者。** 用户就在现场，协调者不需要知道，状态原样更新即可。
4. 汇报正文截断到几行（#2079 第 3 步），协调者需要细节时用 `get_thread` 读。
5. 协调者 effort 默认低，或默认用便宜模型（#2079）。

已实施的是第 3 条（`Report.From`，见 `.design/projects.md` §4.4）：只有协调者交代的事（任务、`message_thread`）完成、需要输入或失败时才回报。协调者交代的事完成后仍会唤醒协调者，因为它要知道自己派的活做完了。要不要把这类回合也省掉（第 1 条），等 #2079 第 1 步的测量结果再定。

预计效果【推断，未测】：协调者回合数从"每个线程每一轮"降到"每个线程每次完成或受阻"。用户和线程来回聊得越多，省得越多。#2079 第 1 步的测量可以验证。

---

## 4. 从本线程自身观察到的协议增量（相比 `/mnt/project-files/research/claude-projects-runtime.md`）

【观察】本会话的系统提示里，比 10-15 那份观察多出的几项：

- **多入口**：项目消息可以来自短信或 Apple Messages，消息上带 `surface` 标记，回复走同一入口。项目被设计成"随手发一句"的地方，不只限网页。
- **富卡片**：线程可以发内联可视化（`post_widget`），包括图表、对比卡、选项卡、步骤卡，以及一种**可就地编辑再发回的草稿卡**（`writing_draft_v0`，用来写邮件或帖子）。另有决策卡 `ask_decision`：不阻塞，按推荐项继续，用户点了或打字回答都会唤醒线程。
- **旧项目迁移的工具化**：有一组"读原项目的对话、会话和文件"的工具（`list_original_project_chats` 等）。迁移后的新项目能按需回看旧项目历史，而不是把历史全部导入。
- **文档输出**：有 Claude Docs 连接器时，"写个计划、文档"默认产出可协作编辑的文档，而不是 .md 文件。
- **设备文件夹工具**：除了 Remote Control，线程还可以通过桌面端直接读写用户授权的文件夹（只做文件操作，shell 只看得见这些文件夹）。文件操作走这条路，要在本机运行才走 Remote Control。

【建议】这些大多是托管平台的能力。对我们有价值的是两条：
- 决策卡：线程遇到分歧时不必停在 `NEEDS_INPUT`，可以按推荐项继续，同时把选项挂在卡片上。
- 草稿卡：线程产出"要贴到别处的文字"时，用户能就地改。

---

## 5. 未能验证

- Landing 状态在没有合并队列的仓库里是否出现。
- 协调者的真实工具和触发规则，比如是否每条线程回复都唤醒它。只能从文档和线程一侧推断。
- 本地线程截断在 8,192 字节是否已修复。
- `EnterWorktree` 在 `-p` / stream-json 模式下是否可用（§3.1 A 的前提），要在本机实测。

## 来源

- 官方文档：[Projects](https://code.claude.com/docs/en/claude-projects)、[Worktrees](https://code.claude.com/docs/en/worktrees)、[Agent view（文件编辑隔离）](https://code.claude.com/docs/en/agent-view)、[Remote Control](https://code.claude.com/docs/en/remote-control)、[Run agents in parallel](https://code.claude.com/docs/en/agents)、[Cloud sessions](https://code.claude.com/docs/en/claude-code-on-the-web)
- 发布博客：[Projects redesigned: from folder to conversation](https://claude.com/blog/projects-redesigned)（2026-09-17）
- Issue：[claude-code#99156](https://github.com/anthropics/claude-code/issues/99156)、[#100715](https://github.com/anthropics/claude-code/issues/100715)、[#101398](https://github.com/anthropics/claude-code/issues/101398)
- 报道：[MACCUS：迁移开始](https://maccus.fr/en/claude-projects-migration/)、[The New Stack](https://thenewstack.io/claude-code-parallel-projects/)、[Developers Digest](https://www.developersdigest.tech/blog/claude-code-projects-coordinator-threads-pro-max-2026)
