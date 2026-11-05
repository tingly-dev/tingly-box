# Projects 消息通道：线程、Coordinator 与用户（IM / Web）

> 状态：提案。**P1 已由用户确认并实现**（见“分阶段计划”里 P1 一节），其余阶段仍是提案、待决定。相关：[coordinator-session.md](./coordinator-session.md)、[../../.design/projects-im.md](../../.design/projects-im.md)（M3 审批进 IM、M4 点名线程）、
> TEP #2412（引用回复直达线程，分支 `tp/file-tep-for-reply-routing-to-th-364d02`，未合并）、[../../agentboot/claude/ref/live-control.md](../../agentboot/claude/ref/live-control.md)。
> 代码引用基于 main `586611a5a`。

## 要解决的三个问题

1. **消息太多**：线程不停往回报，用户的 chat 被刷屏。连“reviewers 在跑，我稍后汇报”这种中间态也会变成一条汇报。
2. **交互被过度转发**：决策卡、审批本来应该直达用户，现在围绕它们的对话被完整转述，一次点击会引出好几条消息。
3. **auto 模式拦截无法批准**：Claude Code 的 auto-mode 分类器拒绝了某条命令（`git push`、`gh pr create`、`gh issue create`），
   线程停住，用户在 chat 里没有任何办法批准；引用用户的批准也解不开。

## 现状（代码事实）

### 一次线程回合结束会产生什么

`Threads.settle`（`internal/project/threads.go`）在**每个**回合结束时发出一个 `Report`（finished / needs_input / failed），没有“这是不是最终结果”的概念：

| 去向 | 钩子 | 结果 |
|---|---|---|
| Web 线程卡片 | `EvReport` 事件、`t.LastReport`、状态 | 卡片更新（这是合适的通道） |
| Coordinator | `Threads.OnReport` → `Coordinator.onReport`：写 `EvThreadUpdate` + 持久输入，唤醒 coordinator 回合 | 每个汇报一次 coordinator 输入；同项目排队中的汇报会合并成一个回合 |
| 绑定在**项目对话**的 chat | `onReport` 里 **立刻** `c.final(ev)` → `Relay.Message` → `foldThreadUpdate` | `[#2 标题] Finished: <首行>`，**先于** coordinator 判断是否值得说 |
| 绑定在**该线程本身**的 chat | `Threads.OnReply` → `Relay.ThreadReply` | 完整回合文本 |
| 同上，之后 | coordinator 回合结束的回复 `OnFinal` → `Relay.Message` | 第二条消息（除非调用了 `no_update`） |

所以一个线程回合结束，项目 chat 至少收到**一条硬编码的折叠行**，再加上 coordinator 通常还会补一条评论。`no_update` 只能压掉第二条，压不掉第一条。

“中间态也汇报”的原因：

- 工作线程（非文件夹会话）在 `settle` 里，只要回合结束就记为 `idle/ready` 并汇报。文件夹会话有 `len(m.engine.BackgroundTasks(...)) > 0 → 保持 working` 的特判，**工作线程没有**。线程让 reviewer 子 agent 在后台跑、自己结束回合，就产生一条“finished”；后台任务完成后 Claude Code 自起一个 unsolicited 回合，又产生一条。
- 线程 system prompt 只要求“每个回合以简短汇报结束”（`prompts.go`），鼓励每回合都收尾。
- `stalled` 汇报已经只给 coordinator、不进 chat，是正确的分级的例子。

### 交互（审批、提问、决策）

- 线程审批/提问：`threadPrompter.wait` → `OnWait` → `Relay.NeedsUser`，直接发给绑定在线程**及其祖先**上的 chat，审批带“允许 / 本线程都允许 / 拒绝”按钮，先答为准，`Settled` 把按钮消息改写成结果。这条路径**已经是直达**，没有 coordinator 参与。
- 但周边仍被转述：`NEEDS_INPUT:` 行结束的回合走普通汇报路径（折叠成 “Needs you … Reply with @N”，再加 coordinator 评论）；审批被回答后，线程继续跑，回合结束又是一条汇报。
- 决策卡（`request_decision`，`decision.go`）：`ask()` 写 `EvDecision` 并调 `c.final`。**原先 `Relay.Message` 的 switch 只处理 assistant / error / thread_update，`EvDecision` 到不了 IM**（已用测试复现：绑定了项目的 chat 一条消息也收不到，Web 才有卡片）——所以权限请求能到钉钉、决策卡到不了。已修，见“决策卡进 IM（已实现）”。用户在 Web 点选项后，`Choose` 把选项文字作为**用户消息**发给 coordinator，触发完整回合：coordinator 必须回复（用户消息回合不允许 `no_update`），随后 `message_thread(cite)` 通知线程，线程回合结束又是汇报——一次点击产生 3–4 条聊天消息（回声问题仍待“设计二”解决）。

### 权限与 auto 模式

- 线程默认权限模式 `acceptEdits`（`DefaultPermissionMode`），可按项目 / 线程改成 `auto`。agentboot 启动带 `--permission-mode <mode>` 和 `--permission-prompt-tool stdio`；Claude Code 先按本地规则判定，只有结果为 **ask** 的调用才会产生 `can_use_tool` 控制请求，进入 `threadPrompter.OnApproval`（`agentboot/claude/settings_ask.go` 同一机制）。
- 线程设置文件（`workerSettings` / `sessionSettings` / `writeSettings`）已经会写 `permissions.additionalDirectories`、`permissions.allow`（Read/Edit 项目 library）和 `autoMode.{environment,allow}`（`library_access.go` 的 `grantLibrary`，用来让分类器放行写 library）。**这就是已有的“预批准”模式**，可以复用。
- “允许本线程”目前按**工具名**记忆（`Threads.allow(threadID, req.ToolName)`）：对 Bash 点一次“本线程都允许”，等于放行该线程所有 Bash。这在默认模式下就是个过宽的授权，与下文的收紧方案有关。
- agentboot 已有：`SetPermissionMode`（运行中切模式，`set_permission_mode` 控制请求）、`can_use_tool` 回复里的 `updatedPermissions`（目前只实现了 `addDirectories`）、`ResultMessage.PermissionDenials`（结构体字段是 `request_id/reason`，与 Claude Code 实际的 `tool_name/tool_use_id/tool_input` 不符，且 Projects 没有读取它）。

### Claude Code 对分类器拒绝的行为（官方文档，读自 code.claude.com 的 permission-modes / auto-mode-config / hooks / permissions / agent-sdk 页）

| 事实 | 对设计的影响 |
|---|---|
| 分类器是“其余所有情况”的最后一步；**拒绝不会产生 `can_use_tool`**，只有 ask 结果才会回到宿主 | 现有 IM 审批管线看不到分类器拒绝，这就是问题 3 的根因 |
| 拒绝后 Claude 收到拒绝理由并改试别的做法；连续 3 次或累计 20 次拒绝为阈值；`-p` 下无提示宿主时动作不执行、会话继续（不会中止）；有宿主时阈值回退是否路由给宿主**文档未明说** | 线程“停住”其实是模型放弃或阈值后卡住；不能指望自动回退到审批 |
| 匹配的 `permissions.allow` / `ask` / `deny` 规则**先于分类器**解析；`Bash(git push *)` 作为 allow 规则不经分类器 | **预批准规则**可行（项目/线程级，窄规则） |
| `permissions.ask` 规则在 auto 模式下**总会提示**，分类器不能自动放行它；匹配的是命令原文，`git -C dir push`、`git -c k=v push` 这类写法会绕过，需要 `PreToolUse` hook 返回 `ask` 做全文匹配 | 可把“危险但可批准”的命令强制变成 ask，复用现有审批卡 |
| 进入 auto 模式时会丢弃过宽的 allow 规则（`Bash(*)`、`Bash(python*)`、包管理器运行命令、`Agent` 等）；窄规则保留；`classifyAllShell: true` 会让所有 Bash allow 规则失效 | 预批准规则必须窄（精确命令或有限前缀），不能写成 `Bash(*)` |
| `PermissionDenied` hook（auto 模式专用，v2.1.89+）：输入含 `tool_name`、`tool_input`、`tool_use_id`、`reason`；输出只有 `retry: true`（告诉模型可以重试，**不能**放行，重试仍过分类器）；仅 command hook | 是**检测**拒绝的干净信号，不是授权手段 |
| `PermissionRequest` hook：在要提示（或不能提示而要自动拒绝）时触发，可返回 allow（带 `updatedPermissions`，目的地 `session` 为内存级）/ deny；deny/ask 规则仍然更强 | 只覆盖 ask 结果，不覆盖分类器拒绝 |
| `PreToolUse` hook 可返回 `allow/deny/ask/defer`；文档**没有**说 hook 的 `allow` 能否覆盖分类器拒绝；deny/ask 规则无论 hook 结果都生效 | 需要实验确认 |
| 分类器会读取“明确点名该动作的用户意图”，可解除 `soft_deny`，**解不开 `hard_deny`**；“批准”不会生成规则，重试仍重新判定 | 来自**真实用户回合**的批准有机会解开 soft 拒绝；coordinator 转述的不行（我们自己在转述前加了“不代表用户批准”的前缀） |
| `set_permission_mode` 可在流式会话中途切换，立即生效；不能中途进入 `bypassPermissions`（除非启动时开了）；`auto` 在某些模型上不可用 | 可作为“一次性放宽”的手段，但粒度粗 |

没跑实验的部分：上面全部来自文档；`--permission-prompt-tool stdio` 与 ask 规则同用的行为文档只是暗示，`PreToolUse allow` 能否越过分类器文档没写。这些是 P0 要用真实 CLI 验证的东西。

## 设计一：消息分级

核心原则：**线程的“说话”和“被看见”是两件事**。线程卡片（状态、一行进度、最后汇报）承载进度；chat 只承载需要人反应或值得人记住的事；coordinator 看见全部但**不要求**每条都转述。

### 分级

| 级别 | 内容 | 给 coordinator | 给用户 chat | 给 Web |
|---|---|---|---|---|
| **T0 立刻、可操作** | 审批、提问（含 `NEEDS_INPUT`）、决策卡、分类器拒绝（见设计三）、线程失败 | 以一条 system 备注告知，**不触发回合** | 带按钮/可回复的一条消息，直达，不经 coordinator | 卡片 + 等待信号 |
| **T1 结果** | 线程最终完成（有产物：提交、PR、文件）；一批线程全部完成 | 作为输入触发回合 | **每个线程结果一条**，或汇总成一条（见摘要）；coordinator 可补充，但**这是唯一的一条** | 卡片 + 汇报事件 |
| **T2 仅 coordinator** | 中间态回合（后台任务仍在跑、“稍后汇报”）、`stalled`、check-in、设置变更 | 作为输入；默认 `no_update` | 不发 | 卡片状态 |
| **T3 仅卡片** | 逐步进展：工具调用、线程自述的阶段 | 不发 | 不发（`/verbose` 例外，已有） | 卡片一行进度、转录 |

### 怎么判定“中间态”（不能只靠 LLM 自觉）

1. **结构信号（主）**：工作线程回合结束时若 `engine.BackgroundTasks(session)` 非空，沿用文件夹会话的特判：状态保持 `working`，**不发汇报**；后台任务结束触发的 unsolicited 回合才是最终汇报。这一条就能消掉问题里“reviewers 在跑”的案例，改动只在 `settle`。
2. **线程自声明（辅）**：线程 MCP（现有 `tingly-projects`，目前只有 `send_file`）加 `set_progress(text)`：写卡片的一行进度（更新 `Thread` 的进度字段、发 SSE），**不进 chat、不触发 coordinator**。线程 prompt 改为：中途想让人知道进度用 `set_progress`；等后台任务就等，不要结束回合来“稍后汇报”；回合结束汇报只给最终结果。
3. **兜底（coordinator）**：T2 之外的仍由 coordinator 判断；但**折叠行不能再先于 coordinator 发出**（见下）。

### 谁发那一条 chat 消息

问题 1 的直接根因是两份输出（硬编码折叠行 + coordinator 评论）。改为**单一出口**：

- `Coordinator.onReport` 不再 `c.final(ev)`。chat 里的线程汇报由 coordinator 回合的结果决定：它回复了（或调用新工具 `announce`/沿用回复）就发那一条；`no_update` 就不发。
- coordinator 不可用（暂停/归档/出错，`onReport` 里已有的分支）时才回退到折叠行，保证用户不会漏掉完成事件。
- T0 不受影响：直达，不依赖 coordinator 是否在跑。

### 批处理 / 摘要

`Coordinator.work` 已经把排队中的汇报合并进同一回合；在此之上：

- **出站摘要窗口**：同一 chat 在 W 秒内（建议 30–60s，可配）产生的 T1 合并为一条：`3 threads finished: #2 修登录 (PR #…) · #3 …; 1 needs you: #4`。窗口由 Relay 的队列实现（它已经是按序单队列），T0 不进窗口，立即发并顺带冲掉窗口。
- **去重**：同一线程在窗口内的多次 T1 只留最新。
- **节奏上限**（防 coordinator 评论泛滥）：coordinator 提示里已有“replies_since_user ≥ 2 就 no_update”的软规则；改成硬规则——用户未发言期间，非 T0/T1 的 coordinator 评论一律不出站。

### 线程卡片承载进度

Web 已有卡片的状态和 `LastReport`。增加：

- `Thread.Progress`（一行，`set_progress` 或最近一次工具调用的 `describeTool` 自动派生）；卡片显示，不产生转录事件（或只覆盖最后一条，避免转录膨胀）。
- IM 里用 `/threads`（M4 已有）按需拉取，而不是推送。

## 设计二：交互直达（pass-through）

原则：**交互项是一等对象，不是文本消息**。它有类型、所属线程、答案路由，用户回答后只发生一件事：答案送到所属线程，卡片就地改写成结果。coordinator 只收到一条不触发回合的备注。

### 交互类型与路由

| 类型 | 来源 | 现状 | 目标 |
|---|---|---|---|
| 审批（工具权限） | 线程 `can_use_tool` | 已直达，有按钮 | 保留；改写时附上“线程已继续”；之后回合的普通汇报按分级处理 |
| 提问（AskUserQuestion） | 线程 | 已直达；单选 ≤8 项有按钮 | 保留 |
| `NEEDS_INPUT:` 行 | 线程回合文本 | 走汇报 + coordinator | 升级为 T0 交互项：卡片/消息带“回复即作答”，答案**直接**成为用户对线程的消息（`source=im`，无 coordinator 前缀），和 TEP #2412 的引用回复路径一致；coordinator 得到备注 |
| 决策卡 | coordinator | ~~Web 才有~~ 已进 IM（按钮或编号文字）；点击变成用户消息触发整回合 | IM 也渲染按钮；点击后：记录选择、卡片就地改写 `Chosen: X`，coordinator 收到**动作型**输入（见下） |
| 分类器拒绝 | 线程（auto 模式） | 无 | 设计三：转成审批项 |

### 去掉回声

- **决策点击**：`Choose` 目前 `Post(label)` 成普通用户消息。改为带标记的“决策回答”输入：coordinator 必须据此行动（`message_thread`、`start_thread`…），但**它的文字回复被抑制**，除非内容是新的（例如发现冲突、需要追问）。卡片改写本身就是对用户的确认。实现上是允许这类回合调用 `no_update`，并在 coordinator 提示里写明。
- **审批回答之后**：线程继续，用户已经通过卡片改写看到了结果，不再发“已继续”类消息。
- **coordinator 备注代替转述**：用户直接对线程的回答（按钮、`@N`、TEP #2412 的引用回复）都只给 coordinator 一条 system 备注（#2412 已为引用回复实现“项目对话里加 system 事件、不触发回合”），统一复用这个机制。
- **回复路由**：保持 #2412 的优先级（行首 `@N` > 引用回复 > 绑定）。交互项消息要在 `channel_messages` 里记平台消息 id，这样对一张提问卡的**引用回复**就是答案，不需要“chat 的下一条消息”这种隐式状态（`Relay.awaiting` 现在靠“oldest unsettled”，多线程并发时容易答错）。可以作为 #2412 之后的第二步。

### 信任

- 只有来自**授权 chat / Web 会话的原生用户操作**才算批准（`remote_control.approve` 权限，已有）。coordinator 转述、线程文本、汇报内容都不是批准（现状：`coordinatorNote`、`cite` 机制已保证这一点，保持）。
- 交互项的展示内容由**宿主**从真实 `tool_input` 生成（命令原文、文件路径），不用模型写的描述。

## 设计三：批准 auto 模式的拦截

根因：分类器拒绝不是 ask，宿主看不到；用户的批准只能以文字形式经 coordinator 转述，而分类器不把转述当作用户意图。目标是让用户在 chat 里点一下就能批准，同时不削弱“只有本人批准才算”。

### 方案

**A. 把高影响命令改成 ask（预防，复用现有审批卡）**

线程设置文件里（`grantProjectAccess` 同一个地方）为 auto 模式追加 `permissions.ask`，如 `Bash(gh pr create *)`、`Bash(gh issue create *)`、`Bash(gh pr merge *)`。（**不能**再加 `Bash(git push *)`：规则按 deny、ask、allow 的顺序取第一个命中，笼统的 push ask 会连 B 里预批准的本分支 push 一起拦住；实现时 push 只按形状 ask，见 P1。）文档说明 ask 规则在 auto 模式下总会提示、分类器不能自动放行；`-p` 下提示走 `--permission-prompt-tool`，也就是现有的 `threadPrompter` → IM/Web 审批卡，用户点“允许”即可。
- 优点：几乎不改管线；批准对象就是那一次具体的调用，展示的是真实命令。
- 局限：规则匹配命令原文，`git -C dir push`、`git -c k=v push` 之类写法会漏过去落到分类器；补一个 `PreToolUse` command hook 做全文匹配并返回 `ask` 可以堵上（hook 由宿主写进同一份 settings）。

**B. 窄范围预批准（免打扰，用户事先授权）**

项目/线程设置里加“预批准命令”，写进 `permissions.allow`，先于分类器生效，沿用 `grantLibrary` 的模式。默认只给线程自己的分支：`Bash(git push origin <thread.branch>)`、`Bash(git push -u origin <thread.branch>)`（分支名在写 settings 时已知），可选 `Bash(gh pr create *)`。线程 prompt 本来就允许“推自己的分支、按要求开 PR”，这只是让规则和约定一致。
- 必须是窄规则：宽通配符进入 auto 模式时会被丢掉，也会带来风险。
- 来源必须是主人：项目设置页由用户开启，或审批卡上的“此项目总是允许这条”由用户点击后写入；模型、coordinator、线程都不能写这份设置。
- 规则在设置页可见、可撤销，每次生效写一条事件（审计）。

**C. 拒绝 → 审批卡（反应式，覆盖 A/B 之外的所有拦截）**

在线程设置里加一个 `PermissionDenied` command hook（auto 模式专用，v2.1.89+），hook 拿到 `tool_name`、`tool_input`、`tool_use_id`、`reason`，通过只监听 loopback 的 capability URL（和 Projects MCP 同一种做法）通知宿主。宿主创建一个“被拦截”交互项发给用户：“Auto mode blocked: Run `git push origin tp/x`” + 按钮 [Allow once] [Allow for this project] [Deny]。批准之后怎么让命令真的跑起来，有三种做法，按可靠性排序：

1. **宿主代跑（approve-and-run）**：批准后宿主在该线程的 cwd 用线程的环境执行**那一条原样命令**，把退出码和输出作为用户批准的结果送回线程（“用户批准并已由宿主执行，输出如下，请继续”）。不依赖分类器是否肯放行，最可靠，适用于 `git push`、`gh pr create`、`gh issue create` 这类一次性、有明确副作用的命令。风险：宿主执行的环境与线程的 sandbox/环境变量可能不同；只限 Bash、只限与被拒绝完全一致的命令串、一次性、绑定 `tool_use_id`、结果写入线程转录作为可见事件。不适用于需要交互或依赖线程内状态（shell 变量、管道前序）的命令。
2. **真实用户回合 + 重试**：批准后宿主向线程发一条**不带 coordinator 前缀**的用户消息（`source=im`），点名该动作（“我批准运行 `git push origin tp/x`，请重试”），并让 hook 返回 `retry: true`。按文档，分类器会把点名具体动作的用户意图当作信号解除 soft deny，hard deny 解不开。依赖模型行为与分类器判断，需要实验确认；好处是不改变执行位置。
3. **一次性放宽模式**：`SetPermissionMode(default)`（`set_permission_mode`）后让命令走 ask → 现有审批卡（用户已经点过一次时可自动应答）→ 命令执行 → 切回 `auto`。粒度粗，窗口内其他调用也会被放宽，**不推荐**，除非其他方案都不可行。

> 不要用 `PermissionRequest`/`canUseTool` 的 `updatedPermissions` 去“放行分类器拒绝”：文档说分类器拒绝不会到达这里。`updatedPermissions`（`addRules`，目的地 `session`）用在“Allow for this thread”的范围收紧上（见下）。

**D. 结构性修正：收紧“允许本线程”**

现在对 Bash 点“本线程都允许”会记 `allowed[thread]["Bash"] = true`，等于放行线程的全部 Bash。改成用审批请求自带的 `permission_suggestions`（或宿主从命令推出的前缀，如 `git push origin tp/x`）作为范围，回复 `updatedPermissions: addRules / allow / session`；没有可用建议时，“本线程都允许”在 Bash 上不显示。需要在 `agentboot/claude/transport.go` 的 `buildPermissionResponse` 里实现 `addRules`（目前只有 `addDirectories`）。

**E. 兜底检测**

不装 hook 的路径：读取 `ResultMessage.permission_denials`（修正结构体为 `tool_name/tool_use_id/tool_input`），回合结束时若存在拒绝，把线程置为 `waiting`，T0 交互项“上一步被 auto 模式拦截”，用户批准后按 C 的方式处理（此时线程已经停下，用 C-1 宿主代跑或 C-2 的用户消息继续）。文档说明有宿主提示工具时 `permission_denied` 系统事件可能不发，但结果里的 `permission_denials` 是权威记录。

### 推荐组合

先 **A + B + D**（不需要新机制，直接复用审批管线与 settings 写入），再 **C-1 + E** 覆盖其余拦截；C-2 作为 C-1 不适用时的后备，需先验证。

### 安全与信任

- **只有主人的原生操作算批准**：授权 chat 的按钮 / Web 点击，要求 `remote_control.approve`。coordinator 的转述、线程输出、汇报、文件内容一律不算（沿用 `cite`、`coordinatorNote`）。
- **批准绑定具体调用**：卡片展示宿主从 `tool_input` 取出的命令原文（不是模型的描述）；批准只对应这个 `tool_use_id` 和这条原文，一次有效，回合结束即失效；“总是允许”写入的是窄规则，不是工具名。
- **提示注入**：线程读到恶意内容后诱导自己 push，用户在卡片上看到的是真实命令，是主要防线；分类器是第二道，不能因为这个功能把它整体关掉（不使用 `bypassPermissions`，也不把 `Bash(*)` 之类宽规则写进 allow）。
- **审计**：每次批准、拒绝、代跑结果都写线程事件（谁在哪里批的、命令、结果）。
- **组织策略**：如果托管设置禁用了 auto 模式或 bypass，我们的规则写入不能绕过；`deny` 规则永远比 `allow` 强，我们不碰。
- **宿主代跑的额外约束**：不通过 shell 展开用户未见的内容；不在 deny 规则命中时执行；超时与输出截断；不对 `rm`、权限变更类命令提供“代跑”。

## 方案对比

| | 解决 | 改动面 | 风险 | 备注 |
|---|---|---|---|---|
| 消息：折叠行让位给 coordinator 的单一出口 | 1 | `Coordinator.onReport`、`Relay.Message` | 低；coordinator 挂掉时要有回退 | 单点改动，收益最大 |
| 消息：后台任务 → 不汇报 | 1 | `Threads.settle` | 低 | 文件夹会话已有同样逻辑 |
| 消息：摘要窗口 | 1 | `Relay` 队列 | 中：延迟 vs 及时 | T0 绕过窗口 |
| 消息：`set_progress` + 卡片进度行 | 1 | 线程 MCP、`Thread` 字段、前端卡片 | 中：线程是否肯用 | 与结构信号互补 |
| 交互：决策卡入 IM + 动作型输入 | 2 | `Relay.Message`、`Choose`、coordinator 提示 | 中 | 需要 `no_update` 在该回合可用 |
| 交互：`NEEDS_INPUT` 升级为直达 | 2 | `settle`、`Relay` | 中 | 依赖 #2412 的路由与消息 id 记录 |
| 拦截：A ask 规则 | 3 | settings 写入 | 低；漏过写法靠 hook | 复用现有审批卡 |
| 拦截：B 窄预批准 | 3 | settings 写入、项目设置 UI | 低；规则必须窄 | 免打扰 |
| 拦截：C-1 hook + 宿主代跑 | 3 | hook、loopback 接口、执行器、卡片 | 中高：宿主执行命令 | 最可靠，需明确用户同意这一语义 |
| 拦截：C-2 用户消息 + retry | 3 | hook、消息 | 不确定，依赖分类器 | 先实验 |
| 拦截：C-3 切模式 | 3 | 控制请求 | 高：窗口内放宽 | 不推荐 |

## 分阶段计划

**P0 实验（半天到一天，不改产品代码）**——用真实 Claude Code（≥2.1.259，隔离 HOME 与测试仓库）验证：
1. auto 模式 + `permissions.ask: ["Bash(git push *)"]` + `--permission-prompt-tool stdio`：是否产生 `can_use_tool`；`git -C x push` 是否漏过；
2. `PermissionDenied` hook 在 `-p`/stream-json 下是否触发、拿到什么字段；`PreToolUse` 返回 `allow` 能否越过分类器拒绝；
3. 来自真实用户回合的“批准”能否解除一次 soft deny，而经 `coordinatorNote` 转述的不能；
4. `permission_denials` 在结果里的真实形状；有宿主提示工具时达到 3/20 阈值的行为；
5. 窄 allow 规则 `Bash(git push origin tp/x)` 在 auto 模式下是否保留且免分类。

**P1 快赢（低风险，各自独立可合并）——用户已确认（“p1 小改动可以先做，甚至堆叠”），除第 3 项外已实现，每项一个提交，可单独评审或丢弃**

1. **已实现**：`settle`：工作线程回合结束时有后台任务 → 保持 working、汇报先扣下（`background.go`）。接续回合（Claude 在后台任务结束后自起的回合）的汇报取代它；没有接续回合时，最后一个任务结束 15 秒后放出（`Threads.HoldGrace`），任务一直不结束则 10 分钟后放出（`HoldMax`，与常驻进程 10 分钟空闲回收对齐）。`NEEDS_INPUT` 和失败不扣。
2. **已实现**：`Coordinator.onReport` 不再先发折叠的 `[#N] Finished: …`：coordinator 在线时，finished 汇报只经 coordinator 的回复进 chat（`no_update` 即不发）。与提案的差别：失败和 `NEEDS_INPUT` 仍先发折叠行（它们需要人处理，且带 `@N` 回复提示）；coordinator 不可用（暂停、归档、出错）时 finished 也回退到折叠行。coordinator 提示加了一句：用户的 chat 不会再收到别的关于完成线程的消息，有结果（PR、文件、答案）就说一两句。
3. **未做**（不在确认范围）：线程 prompt 改为不要为“稍后汇报”结束回合、回合汇报只写最终结果。
4. **已实现**：auto 模式线程的 settings（`git_access.go`）：allow `git push [-u|--set-upstream] origin <本线程分支>`；ask `gh pr create|merge`、`gh issue create`、推到 base/main/master、`--force/-f/--force-with-lease`、`--delete`。只在线程的有效权限模式为 auto 时写；settings 文件现在每回合重写，所以事后切到 auto 也生效。其余形状的 push 仍交给分类器（笼统的 push ask 会盖过 allow），要靠 P3 的 hook 接住。分支名含括号、星号、空白时不写 allow 规则。
5. **已实现**：D 的简化版：Bash 的“本线程都允许”只记**那一条命令原文**（去掉首尾空白后逐字匹配），其他工具仍按工具名。没有 `permission_suggestions` 的前缀规则、也没用 `updatedPermissions`（`addRules` 留给以后）。IM 里的按钮对 Bash 改叫“Allow this command for this thread”。**网页端按钮文案仍是 “Always allow {{tool}} here”（`thread.allowThread`），对 Bash 已不准确，需要前端跟进**（本次没改前端）。

**设置怎么应用（常驻进程不重启；用户决定：“不应该重启，应该通过 stream 消息注入或者等待下一轮激活”）**

线程的设置文件每回合开始前重写（只在内容变化时，经 rename 原子替换）。`claude` 进程只在**启动时**读 `--settings` 文件，之后改文件对它没有任何作用（实测，见 `agentboot/claude/ref/live-control.md` 的 “Settings”）。所以每一种变化走哪条路：

| 变化 | 怎么应用 | 进程重启？ |
| --- | --- | --- |
| 权限模式（default / acceptEdits / auto / plan …） | `set_permission_mode` 控制请求（`applyLive`，原有） | 否；进程拒绝时才回退到重启（原有行为） |
| 模型 | `set_model` 控制请求（原有） | 否；同上 |
| **权限规则**：`permissions.allow/ask/deny/additionalDirectories`、`autoMode.*`（含 P1 的 auto 模式 git 规则、library 授权） | 下一轮开始前 `apply_flag_settings` 控制请求，只发变化的顶层键；已验证实际生效：加规则后同一命令不再提示，别的命令仍提示，清空后又提示 | **否**；后台任务不受影响 |
| 规则被进程拒绝（太旧、不支持的 CLI） | 不推送，规则等进程下次自然启动（空闲 10 分钟回收后用 `--resume` 重开）再生效；下一轮再试一次 | 否 |
| 环境变量、网关路由、profile 的其余设置、MCP 配置、effort、entrypoint | 只能在启动时读 | 是（原有；变了才重启，且这类变化在 UI 上本来就是显式改动） |

要点和限制：

- **只能加不能减**：推送的层叠在进程启动时读到的文件内容之上，数组取并集。启动时文件里已有的规则（例如先在 default 模式启动、没有 git 规则）后来被删掉，要等下次启动才真正消失。反过来，启动时没有、后来才加的规则（例如中途切到 auto 后新增的 git 规则）当轮就生效。对这里的规则集这是无害的方向：多出来的 ask 只会多问一次，多出来的 allow 只限本线程分支。
- **启动签名不含规则**：`launchSignature` 里设置文件的哈希去掉 `permissions` 和 `autoMode` 两个键再算，所以改规则不会让进程被驱逐（这正是之前“每回合重写 settings 会重启并带走后台任务”的风险）。文件解析不了时仍按整个内容哈希。
- **何时生效**：规则在回合之间推送（引擎在启动一轮前对比并发送），所以进程正在跑后台任务时空闲着也可以推；正在回合中的进程要等它下一轮。
- **验证**：`agentboot/claude/ref/live-settings-spike.py` 用真实 CLI 2.1.259 加本地脚本模型验证了以上语义；`agentboot/engine` 的 `TestEngine_PermissionRulesApplyLiveWithoutRestart` 用替身进程验证“改规则不重启、模式切换仍走 `set_permission_mode`、被拒绝不重启且下轮重试、改 env 仍重启”。

P1 没有用真实 Claude Code 验证：规则是否真的在 auto 模式下保留并免分类（P0 第 5 条）仍待实验。

**P2 分级与交互**
1. 分级模型落到 `Report`（带 tier），`set_progress` 与卡片进度行；
2. 摘要窗口；
3. 决策卡 IM 渲染；`Choose` 改为动作型输入并抑制回声；
4. `NEEDS_INPUT` 升级为直达交互项（依赖 #2412 合并及其消息 id 记录）；
5. 前端：卡片进度行、被拦截卡片的样式。

**P3 拦截闭环**
1. `PermissionDenied` hook + loopback 接口 → “被拦截”交互项；
2. 宿主代跑（C-1），或按 P0 结果选 C-2；
3. 项目设置页“预批准命令”，卡片上的“此项目总是允许”；
4. 修正 `PermissionDenial` 结构并用 E 兜底。

## 待用户决定

1. **代跑语义**：批准后由宿主执行那条命令（C-1），还是只放行让线程自己重试（C-2，可能仍被拦）？前者可靠但意味着宿主以线程的身份执行命令。
2. **默认预批准范围**：线程自己分支的 `git push` 是否默认免问？`gh pr create`、`gh issue create` 呢？合并（`gh pr merge`）是否永远要问？
3. **单一出口**：chat 里线程完成的消息是否**只由 coordinator 的回复**产生（coordinator 说 `no_update` 就不发）？还是线程的最终汇报一律至少发一行？
4. **摘要窗口**：默认 30–60s 合并 T1，T0 立即发——可以接受吗？是否需要按 chat 设置（`/verbose`、`/quiet`）？
5. **决策卡点击后 coordinator 是否完全静默**（只让卡片改写确认），还是仍给一句确认？
6. **auto 模式的默认**：是否要在 UI 里提示“auto 模式下 push/PR 会被分类器拦截，建议开启预批准”，而不是静默？
7. 是否接受先做 P0 实验再定 C 方案？实验需要用到真实 Claude 凭据与 auto 模式可用的模型。

## 待核实（本文未验证）

- 决策卡进 IM 已核实并修复（见“决策卡进 IM（已实现）”）；未在真实钉钉/飞书/Telegram 上验证。
- 所有关于 Claude Code 行为的结论来自官方文档，未在真实 CLI 上跑过；`--permission-prompt-tool stdio` 的行为文档只有暗示（P0）。
- `PermissionDenial` 的真实 JSON 形状按我对 SDK 的了解写成 `tool_name/tool_use_id/tool_input`，需要实测。
- 本文未改动任何产品代码；TEP #2412 分支尚未合并，涉及它的部分以其提交说明和 `.design/projects-im.md` 里的描述为准。

## 决策卡进 IM（已实现）

根因：`Relay.Message`（`internal/project/relay.go`）按事件种类分发，没有 `EvDecision` 分支，而 `Coordinator.ask` 只经 `OnFinal → Relay.Message` 出站；权限请求走另一条路（`OnWait → Relay.NeedsUser`），所以只有它到 IM。

现在：

- `Relay.decision` 把卡片当作一个**请求**发给绑定在该项目主线程上的 chat：请求 id 是卡片事件 id，`pendingRequest{主线程, 事件 id}`。支持按钮的平台（`ChannelPrompter` 且平台能力含 interaction：Telegram、Feishu/Lark 等）每个选项一个按钮，值为 `o<N>`，推荐项标 `(recommended)`；不支持按钮的平台（`PlatformCapabilities.SupportsInteraction()` 为假：钉钉、Discord、Slack、WhatsApp 等，`ErrNoButtons`）和没有 prompter 的宿主，收到带编号的文字卡 + “Reply 1 or 2 to choose.”。文字含上下文、每项后果、推荐理由。
- 回答有两条路，都落到同一个 `Coordinator.Choose`：① 按钮回调 `pj` → `RouteAnswer` → `Service.AnswerFromChannel`（要求该 chat 绑定在卡片所属的主线程或其祖先上，否则 `ErrNotThisChat`；已答返回 “This was already answered.”）；② 文字回复：`PostFromChannel` 里该 chat 的待答请求依次尝试，卡片只认选项的**编号或标签**（忽略大小写和句末标点），其它文字是普通消息、卡片继续开着，也不会挡住同一 chat 里之后的审批/提问的文字回答。
- 答案作为**用户消息**（`source=im`，只有标签本身，不带 coordinator 措辞）交给 coordinator，与 Web 点击一致；`Coordinator.OnDecided → Relay.Decided` 把各 chat 里的卡片改写为 “Answered in a chat/on the web: <标签>”，并忘掉待答记录。Web 先答同样会改写 IM 里的卡片。
- 信任：能答卡片的只有绑定了该会话的 chat（按钮路径在 `AnswerFromChannel` 校验，文字路径经 chat 的配对/白名单门）。已知缺口（未改，影响审批按钮同理）：按钮回调不经 `handleInboundGate`，在群里是“chat 已绑定”这一级，不区分群成员。

测试：`internal/project/decision_im_test.go`（按钮、纯文字、无 prompter 宿主、点按钮答复、越权、文字编号/标签、Web 先答、未答卡片不挡审批）；`internal/server/module/projects/tingly_decision_test.go`（`TestTinglyDecision*`：真实 MCP `request_decision` → bot → Tingly 传输；按钮、用 `buttonless` 包装模拟钉钉式无按钮平台、他人 chat 越权）；`imchannel` 里断言 dingtalk/weixin 的 `SendActions` 为 `ErrNoButtons`。
