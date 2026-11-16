# Claude 官方 "Projects" 调研

调研日期：2026-10-10。资料来自 Anthropic 官方文档、博客、帮助中心，以及第三方报道和评测（来源列表在文末）。
本次没有上手实测官方产品，下文所有"行为"都基于公开资料。社区里也几乎找不到 Reddit 或 HN 上的一手使用反馈（§8 有说明）。

---

## 0. 结论摘要

1. **官方有三个都叫 "Projects" 的东西**，而且正在合并成一个：
   - **Chat Projects**（claude.ai，2024-06）：文件夹形态，把对话、知识和指令归在一起。
   - **Cowork Projects**（桌面端）：本地文件夹、指令和记忆，由 Dispatch 路由后台任务进来。
   - **Redesigned Projects**（2026-09-17，最先在 Claude Code 上线）：一个"协调者对话"，加上一批并行干活的"线程"。
   官方明确说前两者"会在新设计覆盖到时升级"。**新版就是 Projects 的未来形态。**
2. **范式转变：从文件夹到对话，从"组织上下文"到"委派工作"。** 旧版回答"怎么让每次对话都带上背景"。新版回答"怎么把一串持续冒出来的工作交出去、走开、回来只处理需要我的部分"。
3. **新版的核心体验要素**：
   - 一个入口：所有任务都发到同一个对话里。
   - 协调者负责路由：开新线程、交给已有线程，或者直接就地回答。
   - 线程就是完整的 agent 会话：独立分支，会开 PR、自己修 CI。
   - Overview 按状态分组，"等你处理"（Waiting on you）是核心视图。
   - 记忆由 Claude 自己写，用户可以查看和编辑。
   - 可以用自然语言调节自主程度。
   - 信任来自"每个线程最后都落成一个要人 review 的 PR"。
4. **外界的主要疑虑**：
   - 消耗快：每个线程都是一个完整会话。
   - 协调者能否在真实的复杂代码库里把任务拆准、合准。
   - 只能在云端运行：代码不能出内网的团队用不了，本地线程要通过 Remote Control。
   - 记忆的可靠性和可审计性。
5. **对照我们已经做的东西**：已提交的 tingly-project 复刻的是 **Chat Projects（旧版）**。对新版的核心体验（协调者、线程、Overview、PR、审批）复刻度为零。详见 §9。

---

## 1. 时间线与三条产品线

| 时间 | 事件 |
|---|---|
| 2024-06-25 | claude.ai 推出 Projects：200K 上下文的知识库、自定义指令、Artifacts、Team 共享的活动流 |
| 之后 | 付费计划的知识接近上限时，自动开启 RAG 模式，容量最多扩大约 10 倍 |
| （时间未确认） | Cowork（桌面端）推出自己的 Projects 和 Dispatch |
| 2026-09-17 | "Projects redesigned: from folder to conversation"，最先在 Claude Code 公开测试（Pro/Max） |
| 后续计划 | 推广到 Chat、Cowork、Team 和 Enterprise；本地线程（"very soon"） |

|  | Chat Projects | Cowork Projects | Redesigned Projects |
|---|---|---|---|
| 形态 | 文件夹：对话、知识、指令 | 文件夹：本地目录、指令、链接、记忆 | 对话：协调者加线程 |
| 存放位置 | Claude 账户（云端） | 只在本机 | 云端（线程是云端会话） |
| 干活的单元 | 用户自己开的每一个对话 | 一个 Cowork 会话（Dispatch 可以拆出子任务） | 协调者自动开的线程（Claude Code 会话） |
| 上下文 | 知识文件 + 指令，对话之间互相隔离 | 本地文件夹 + 指令 + 链接 + 项目记忆 | 仓库、文件库、指令（≤16k 字符）、记忆文件、云端环境 |
| 记忆 | 每个项目有独立记忆，对话开始前可以关闭 | 项目级记忆 | `MEMORY.md` 索引加多个记忆文件，由 Claude 自己写，可在设置里编辑 |
| 产出 | 对话、Artifacts | 文件写进项目目录 | PR、文件库里的文件 |
| 共享 | Team/Enterprise 支持查看或编辑权限 | 不支持 | 不支持（单用户，beta） |

---

## 2. Chat Projects（经典版）

### 2.1 定位与功能
- 自成一体的工作区：有自己的对话历史和知识库。所有人都能用，免费用户最多 5 个项目。
- **名称和描述 Claude 看不到。** 起作用的只有知识和指令。
- **知识**：在右侧面板点 "+" 添加文档、文本或代码，项目里所有对话都会用到。付费用户的知识接近上下文上限时，自动切换到 RAG。
- **指令**：设置语气、角色、行业视角等。
- **对话之间互相隔离**，除非把内容加进知识库。
- **记忆**：每个项目有独立的记忆，和项目外的对话分开。在第一条消息之前，可以在 "+" 菜单里为单个对话关闭记忆。
- **管理**：收藏后会出现在侧栏。可以把单独的对话"添加到项目"或"移动到其他项目"。归档后的项目不能删除，要先取消归档。删除时需要确认 "Yes, delete"。
- **共享**（Team/Enterprise）：分"可查看"和"可编辑"两种权限，可以对全组织可见，有 "Shared with you" 标签页。

### 2.2 典型使用场景
- **写作和品牌声音**：存放风格指南和过去的好作品，比如营销邮件、Newsletter 系列。
- **研究分析**：一批论文、财报（10-K）、竞品报告，在里面反复提问、对比、总结。
- **战略规划**：路线图、决策标准、情景分析，跨多个对话沿用早先的决策。
- **领域助手**：数据分析师风格的 SQL、团队内部知识问答。
- 官方客户案例 North Highland：内容和研究任务最多提速 5 倍。

### 2.3 用户的使用模式（第三方指南总结）
- 只有当工作会**反复出现或逐步积累**时才建项目，一次性的问题直接普通对话。
- **一个项目对应一件事**，混在一起会稀释上下文。
- 指令里写清楚主题、你的角色、期望的输出。
- 把定稿的产出**放回知识库**，供以后的对话使用。
- 上传文件后先让 Claude 总结一遍，检查有没有遗漏；定期让它审查指令和文件之间有没有冲突、是否过期。
- 定期手动整理决策，标记哪些假设已经失效。

### 2.4 用户痛点
- 对话之间看不到彼此。去问新对话之前的决策，可能会得到编造的回答。
- 知识是静态快照，源文件更新了要手动删掉重传。
- 不能执行任务：没有任务、截止日期、负责人、状态或进度。第三方指南的原话是"更像图书馆而不是工作区"。
- 产出要搬到别的工具里才能真正用起来。

> 这些痛点正是新版要解决的：从"资料库"变成"能把活干完并汇报状态的地方"。

---

## 3. Cowork Projects 与 Dispatch（桌面端）

- **项目包含**：描述（Dispatch 选项目时会读）、一个或多个本地文件夹（可读写）、指令、参考链接、可链接的 Chat 项目、项目级记忆。
- **创建方式**：从零开始、导入 claude.ai 的项目，或者直接指向已有的文件夹。
- **只存在本机**，不同步也不共享。归档会删掉项目的元数据，但不动本地文件。
- **Dispatch**：一个长期运行的 agent 对话。用户描述想要的结果，它拆成子任务：
  - 编码类任务交给 Code 的 workspace，知识类任务交给 Cowork 项目。
  - 子任务在侧栏显示状态：Running / Awaiting input / Awaiting answer / Completed / Error / Archived。
  - 审批请求会转给用户，**10 分钟内不回应就自动拒绝**。
  - 可以在手机上派任务，实际在桌面上运行。
- **意义**：Dispatch 已经是"一个对话负责调度、多个子任务干活"的形态，新版 Projects 是把这套做法用到代码和云端。

---

## 4. Redesigned Projects（核心）

### 4.1 心智模型
> "A project is one ongoing conversation where Claude coordinates a stream of related work for you."
> "Threads do the work, Claude directs it." 用户要"像给幕僚长（chief of staff）布置工作一样"给它下达任务。

不用项目时，用户自己就是协调者：决定每个会话做什么，在每个会话开头重复同样的背景，再挨个回去看哪个做完了、哪个在等回答。项目把这三件事接了过去：
- **一个入口**：随时把 bug、报错栈、任务清单贴进来。
- **背景只说一次**：比如"PR 目标分支是 main"，所有线程都会知道。
- **走开再回来**：Overview 显示哪些做完了、哪些 PR 可以 review、哪个线程在等你回答。

### 4.2 组成
| 部分 | 说明 |
|---|---|
| 项目对话（协调者） | 一个长期运行的会话。它接收消息，决定哪些开成线程，并跟踪所有线程。**只看线程汇报的结果，不看每一步。** 依据最近的消息、最近的线程和项目记忆工作，所以可以一直运行下去 |
| 线程（工作者） | 每个都是独立的完整会话，有自己的上下文，做完一件事就向协调者汇报。云端线程在自己的分支上工作，需要时开 PR。线程内部还可以再用 subagent 或 workflow |
| 每个线程启动时带的东西 | 项目的仓库和文件、指令和记忆、各仓库的 `CLAUDE.md` 和 skills、账户上的 connectors、云端环境（网络、密钥、工具） |
| Overview 面板 | 四个标签页：Threads（按状态分组）、Library、Pull requests、Routines |

### 4.3 创建流程
- **从零开始**：名称必填。目标（Goal）选填，用一句话说明要达成什么，比如 "Hold p95 API latency under 200 ms"。上下文（Context）选填，填 GitHub 仓库、文件、文件夹或 Google Drive。
- **从已有的云端会话转过来**：选 "Continue as project"，Claude 会读这个会话并给出设置建议；或者选 "Move to project"，把会话里的工作交给一个已有项目。
- **第一个项目创建后，Claude 会主动做一轮**：开一个只读的探索线程，并根据你最近的会话给出 **Setup recommendations**，推荐要加的仓库、要建的 routine、可以开的线程，默认全部勾选，用户可以取消。
- **前置条件**：代码在 github.com 上，有 push 权限，装了 Claude GitHub App；网络和工具由云端环境决定。

### 4.4 日常使用
- **发消息**：快速问题就地回答；新工作开新线程，或者交给负责同一领域的已有线程，并告诉你交给了谁；一条消息里的多个无关任务会拆成多个线程。每个新线程以**卡片**形式出现在消息下面，显示标题和状态，点开就能看。
- **Suggested threads**：有时协调者只给出建议，不直接开线程，用户点击后才启动，也可以一键全部启动。
- **PR 生命周期**：线程从默认分支切出新分支，需要时开 PR。开了 PR 之后会自动跟进：CI 失败就修，有 review 评论就处理，检查通过后告诉你可以 review 了。卡片上有下一步按钮：Resolve conflicts / Fix CI / Address comments / Merge it / Review PR / Create PR。
- **Overview 中线程的状态**：

  | 分组 | 含义 |
  |---|---|
  | Ready for review | PR 已开，等你 review |
  | **Waiting on you** | 需要你回复或审批，或者失败了 |
  | Working | 运行中 |
  | Landing | PR 已批准或正在排队合并 |
  | Idle | 做完了，没有在等什么 |
  | Resolved | 已标记完成（你手动标、合并后由 Claude 标，或者一周无活动后自动标），可以重新打开 |

- **打开线程**：可以看完整过程、直接给这个线程发消息纠偏、回答审批、停止或中断。
- **审批**：线程默认在 auto 模式下运行。需要审批时，**请求只在那个线程里出现**，在项目对话里说"继续"是没用的。审批可以只批这一次，也可以批这个线程后续所有同类请求。
- **在本机运行线程**：在 "+" 菜单里选 Work locally，然后在 "Allow Claude to work in a folder on your device" 卡片上选择文件夹，可以开启 worktree。它通过 Remote Control 连到本机。这种线程用的是本机的工具、MCP、hooks 和权限设置，带项目指令但**不加载项目记忆**，电脑睡眠时会暂停。
- **通知**：桌面端在 Claude 发帖、线程出错或需要你回应时提醒；网页端只在 Overview 按钮上显示一个小红点。

### 4.5 用自然语言调节自主程度（不是开关）
- 先提议线程，等我同意再启动；或者不用确认，直接开始。
- 最多同时跑两个线程；同一领域的后续工作复用已有线程。
- 更新写短一点；只在完成或卡住时才发消息。
- 给我每个线程的状态汇总。
- 这个任务用小一点的模型。
- 先给我看计划，再开 PR。
- 只告诉我问题在哪，先别修。
- 这个就在这里回答，不要开线程。

Claude 会把这些偏好**自己写进项目记忆**，之后的线程也会遵守。但它们是"偏好"，不是硬性限制。

### 4.6 持久上下文
| 类型 | 内容 | 怎么设置 |
|---|---|---|
| 项目记忆 | 需求、决策、踩过的坑，存成多个文件。线程启动时读 `MEMORY.md` 索引，用到时再打开具体文件 | 在对话或任意线程里让 Claude "记住"或"忘掉"；在 Project settings > Memory 里查看、编辑、删除 |
| 项目指令 | 发给每个新线程和协调者，最多 16,000 字符 | 在设置里填写，或者让 Claude 改 |
| 仓库、文件、环境 | 每个线程都会 clone 的仓库，挂载在 `/mnt/project-files` 的文件，以及云端环境 | 在设置里配置，或在对话里让 Claude 加仓库 |

**官方的指令模板**：说明项目目标；工作在哪里（仓库、分支、PR 命名）；线程怎么自检（比如跑 `make test` 并贴出结果）；**缺东西时说明缺什么然后停下，不要替代、mock 或猜**；哪些事要先经过我同意（合并、force-push、改 CI）。

**仓库的取舍**：只把几乎每个任务都会用到的一两个仓库加进项目，其余的写在指令里，线程需要时自己加。

**第一批任务的建议**：先写指令；先发一个小任务，看它怎么汇报；检查一下模型和 effort 设置（默认全部用 Opus，线程是 high effort，协调者是 low effort）；先让它"提议后再启动、少量并行"，跑顺了再放开。

### 4.7 成本与限制
- 和其他会话共用同一份计划额度，**消耗更快**。每天最多新开 200 个线程。没有并发上限，"最多跑 N 个"只是偏好。
- 协调者本身也消耗 token。在看 PR 的空闲线程，遇到 CI 失败或新评论时会被唤醒，也会消耗额度。
- 额度用完后线程会等待，额度重置后自动继续。可以随时暂停整个项目。
- 设置 > Usage 里可以按线程和模型查看用量。
- **限制**：
  - 只能在网页、桌面端和手机上用，终端 CLI 和 IDE 插件里没有。
  - 模型只能是 Anthropic 的。
  - 只支持单用户，不能共享。
  - 线程不能在项目之间移动，两个项目也不能合并。
  - 云端沙箱在两轮之间会暂停，恢复失败时可能丢掉没提交的改动。
- **项目级操作**：Pause（全部停下）、Archive（隐藏，线程归档，routine 停止）、Delete（不可恢复，但 GitHub 上的分支和 PR 不受影响）。

### 4.8 排错时体现的设计思路
- **"线程瞎猜或卡住，而不是来问我"**，通常是项目设置有缺口，不是单个任务的问题。处理方法：
  1. 让协调者列出每个线程的任务、它做了哪些假设、缺什么。
  2. 标记做错方向的线程为已解决。
  3. 在指令或环境里一次性补上缺口。
  4. 先发一个线程验证，再重发其余任务。
- "Claude hasn't responded" 时可以重启协调者，线程不受影响。

### 4.9 与其他并行方案的关系（官方对比）
| 方案 | 谁来协调 | 适用场景 |
|---|---|---|
| Subagents | Claude 在一个会话里委派 | 支线任务会刷屏主对话 |
| Agent view（`claude agents`） | 用户自己派活、看状态 | 几个独立任务，交出去偶尔看一眼 |
| Agent teams | 一个 lead 负责分派和监督（实验性） | 一个大任务拆给多个队友，需要保持同步 |
| Dynamic workflows | 用脚本固定计划 | 规模很大的审计或迁移，需要交叉验证 |
| **Projects** | **Claude 长期协调，跨天甚至跨周** | 任务多、周期长，电脑关了也要继续跑，只想描述一次 |

官方强调："并行本身不是项目的意义。意义在于由 Claude 来启动和跟踪会话，并且每个会话都从同一份指令开始。"

---

## 5. 什么时候用、什么时候不用（官方口径）

**适合用项目的情况**：目标比一个会话活得久，并且会不断产生新任务。
- 一个目标跨多个仓库，比如"让所有服务都用上新的 lint 配置"，每个仓库一个线程、一个 PR。
- 持续往一个领域里喂东西，比如某个服务的 bug、报错栈、review 请求。修完一次后让它记住的坑，会进项目记忆。
- 超过一个会话规模的构建或迁移，比如"按 `docs/spec.md` 把功能做出来""把应用从废弃的 ORM 迁走"。
- 非代码工作，比如合同文件夹、工单导出，反复来问新问题，产出放进 Library。

**不适合**：
- 一个会话就能做完的单个任务 → 直接开一个云端会话。
- 每个任务都要用到本机 → 用本地会话或 agent view。
- 定时执行、不需要对话的任务 → 用 routine。
- 多人一起在 Slack 里派活 → 用 Claude Tag。

---

## 6. 外界评价与风险

- **Futurum 分析师 Mitch Ashley**："Running work in parallel is the easy part to sell. The part that actually earns trust is that every thread still lands as a pull request someone reviews."
- **DevOps.com**：真正的考验是协调者能否在混乱的真实代码库里可靠地判断该委派什么、什么时候合并、什么算完成。这是平台工程问题，不是模型问题。拆错任务会给管理生产分支的团队带来麻烦。
- **VentureBeat**：和 Chat Projects 重名容易混淆。竞品有 OpenAI Codex app（多 agent 工作区）、GitHub 的 agent、Google Jules。差异点在于**持续存在的协调者**。
- **daily.dev**："could drain your plan before lunch"，额度消耗是敏感问题。
- **CellCog**（竞品作者，未实测）：云端运行把代码不能出内网的团队挡在外面；协调者不会阻止多个线程做重叠的工作；建议线程用便宜的模型，协调者用强模型。
  - 它还说"记忆没有查看或导出的方式"。**这与官方文档不符**，官方文档写明可以在 Project settings > Memory 里查看和编辑记忆文件。
- **aireiter**：官方没有公布迁移工具和时间。实测从 Chat 导入 Cowork 时，**积累的对话记忆没有迁移过去**。建议先自行导出，并把决策写成可审查的文件。

---

## 7. 关键洞察（复刻时要抓住的东西）

1. **用户心智从"组织上下文"变成了"委派和收件箱"。** 新版的主视图不是文件列表，而是"什么在等我"。
2. **一个入口加自动路由，取代"先选模式再开始"。** 用户不用决定开不开新会话、交给哪个会话。
3. **信任链是 PR 加 review。** 产出要落到用户已有的审查流程里；非代码工作则落到 Library 里的文件。
4. **记忆是 agent 自己维护的一组文件，但对人可见、可编辑。** 偏好（比如"最多跑两个线程"）也写进记忆。
5. **自主程度用自然语言调，而不是一堆开关**，并有安全的默认做法：第一个项目先只读探索、先提议再执行。
6. **"缺东西就停下来问，不要瞎猜"** 写进了官方指令模板。排错时从项目设置找原因，而不是逐个线程去修。
7. **成本是一等公民**：协调者和线程分别选模型，可以按线程看用量，可以暂停整个项目。
8. **官方当前的空白**：
   - 只能在云端运行，本地线程要依赖 Remote Control。
   - 只支持 Anthropic 模型。
   - 只支持 github.com。
   - 只能单用户。
   - CLI 里用不了。
   - 记忆没有像代码那样的冲突处理机制。

---

## 8. 本次调研的局限

- 没有实际使用过新版 Projects，协调者的路由质量和汇报风格都无法评估。
- 在 Reddit 和 Hacker News 上没搜到一手使用体验，能看到的多是新闻稿改写和厂商博客，其中一部分（CellCog、MemoryLake）是竞品，立场有偏。
- 帮助中心和 Academy 的部分页面比较旧，和新版描述有冲突的地方，以官方文档 code.claude.com 为准。

---

## 9. 对照：当前 tingly-project 复刻了什么

> 本节写于调研时，当时只有旧版的单用户本地复刻。之后协调者、线程、Overview、记忆文件、线程内审批、Goal、仓库和 Library 都已实现，现状以 `../.design/projects.md` §2 为准，下表只保留官方两代产品的差异。

| 能力 | Chat Projects | Redesigned Projects |
|---|---|---|
| 指令 | ✓ | ✓（16k，也发给协调者） |
| 知识库 / Library | 知识库，超量时自动 RAG | Library（文件加产出物） |
| 项目记忆 | 一段记忆，单个对话可关闭 | 多个记忆文件，由 Claude 写、可编辑 |
| 共享和权限 | ✓（Team） | ✗ |
| 协调者对话加路由 | ✗ | ✓ |
| 线程 = agent 会话（分支、PR、CI） | ✗ | ✓ |
| Overview 状态和"等你处理" | ✗ | ✓ |
| 线程内审批 | ✗ | ✓ |
| Suggested threads、Setup recommendations、Routines | ✗ | ✓ |

提示词层面的设计对照（协调者与线程的来源标注、命名、记忆写入、回复风格）见 `../.design/projects.md` §4.5。

---

## 来源

**官方**
- [Let Claude coordinate ongoing work with Projects — Claude Code Docs](https://code.claude.com/docs/en/claude-projects)
- [Run agents in parallel — Claude Code Docs](https://code.claude.com/docs/en/agents)
- [Projects redesigned: from folder to conversation — Claude Blog](https://claude.com/blog/projects-redesigned)
- [Collaborate with Claude on Projects — Anthropic (2024)](https://www.anthropic.com/news/projects)
- [What are projects? — Claude Help Center](https://support.claude.com/en/articles/9517075-what-are-projects)
- [How can I create and manage projects? — Claude Help Center](https://support.claude.com/en/articles/9519177-how-can-i-create-and-manage-projects)
- [Organize work with projects — Cowork Docs](https://claude.com/docs/cowork/guide/projects)
- [Run tasks in the background with Dispatch — Cowork Docs](https://claude.com/docs/cowork/guide/dispatch)
- [Organize your tasks with projects in Claude Cowork — Help Center](https://support.claude.com/en/articles/14116274-organize-your-tasks-with-projects-in-claude-cowork)

**第三方报道和评测**
- [DevOps.com — Anthropic Adds a Coordinator to Claude Projects](https://devops.com/anthropic-adds-a-coordinator-to-claude-projects-for-running-ai-work-in-parallel/)
- [VentureBeat — Anthropic launches Claude Code Projects](https://venturebeat.com/orchestration/anthropic-launches-claude-code-projects-an-always-on-conversation-that-remembers-and-delegates-your-long-running-dev-work)
- [daily.dev — could drain your plan before lunch](https://daily.dev/posts/anthropic-s-new-claude-code-feature-could-drain-your-plan-before-lunch-ebofvbke5)
- [CellCog — Claude Code Projects: One Chat Directs Parallel Threads](https://cellcog.ai/blog/claude-code-projects-redesign/)（竞品）
- [MemoryLake — One Memory, Many Threads](https://www.memorylake.ai/en/blogs/claude-projects-thread-memory)（竞品）
- [aireiter — Claude Projects Redesigned: Cowork vs Claude Code](https://aireiter.com/blog/claude-projects-redesigned-cowork-claude-code-migration)
- [ClickUp — How to Use Claude Projects](https://clickup.com/blog/how-to-use-claude-projects/)
- [Unite.AI](https://www.unite.ai/anthropic-redesigns-claude-code-projects-to-coordinate-agent-threads/) · [MarkTechPost](https://www.marktechpost.com/2026/09/17/anthropic-launches-claude-code-projects-in-beta-parallel-cloud-sessions-that-keep-running-after-you-close-your-laptop/)
