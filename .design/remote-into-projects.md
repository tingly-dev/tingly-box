# Remote 的聊天控制并入 Projects

> 状态：**实施中**（第 2 期能力、第 3 期 a 切换已提 PR）。承接 [projects-im.md](./projects-im.md)（M1–M5 已落地：绑定了的 chat 走 Projects）
> 和 [projects-desk-absorption.md](./projects-desk-absorption.md)（Desk 并入 Projects 的先例，本文沿用它的做法）。

## 目标

YZ（2026-10-21）：Remote 原有的行为迁到 Projects 的**项目**或**文件夹**上，**不再留两套**，但用户原来的用法**照样能用**。

今天同一个 IM chat 有两条完全不同的路：

- **没绑定**：`@tb`（SmartGuide，AFK 运行时）或 `@cc`（`remote/session` + `ClaudeCodeExecutor` 自己起 Claude Code），
  会话、审批、输出渲染都是 Remote 自己的一套。
- **绑定了**（M1–M5）：消息进 Projects 的线程，审批、排队、回复都走 Projects。

迁移之后只剩第二条：**每个 chat 都在 Projects 里有一个落点**，Remote 只剩“接入”（bot 连接、配对、访问控制、通知）。

## 一条规则：Remote 管“谁能进来”，Projects 管“进来之后做什么”

| 留在 Remote（接入层） | 搬到 Projects（会话层） |
|---|---|
| bot 连接与平台凭据、`/bind` 配对、`/join` 群白名单、访问控制（capability） | 会话、排队、审批、停止、回复与汇报 |
| 黑名单、通知/交互 API（Flow A：hook 驱动的 `notify`，见 imbot-output.md） | 工作目录（`/cd`）、会话历史、`/resume` |
| bot 级设置的编辑界面（Remote 页面） | 这些设置**生效**的地方（线程的 profile、权限模式……） |

这条线与 Desk 并入时“通用的进 `projects/`，Tingly Box 独有的经 Host 能力接入”一致：Projects 不需要知道 bot、配对、访问控制。

## 核心模型：`@cc` 的工作目录 = 一个文件夹

今天 `@cc` 的“chat 绑了一个目录（`ProjectPath`），在里面跑 Claude Code，`/clear` 开新会话”，和 Projects 的
“chat 绑在文件夹的线程上，`/new` 开新线程”是同一件事。所以：

- **每个 chat 都有绑定**，没有“未绑定”状态。chat 第一次说话时，若还没有绑定，就在它的目录里开一个文件夹线程并绑上：
  目录 = chat 的 `ProjectPath`（旧数据，或 `/cd` 设的），没有就用 bot 的 `DefaultCwd`（再退到 cwd、home，与今天的解析顺序一致）。
  这一步就是旧数据迁移：`ProjectPath` 在第一次用到时变成文件夹绑定，不需要一次性批量转换。
- **`/cd <路径>`**：把 chat 换到那个文件夹（文件夹不存在就新建为 Projects 的文件夹），下一条消息开新会话（`new_thread`）。
  目录浏览器（按钮选路径）照旧，只是结果落在 Projects。
- **项目（有 coordinator 的）** 仍只在 Web 上绑定（#2184 的决定：项目绑定只有一个入口）。chat 里能改的只有“在哪个文件夹”，
  这本来就是旧用法。这让 chat 里也能换绑文件夹，YZ 确认保留（见“已定”）。
- **会话历史不导入**：旧 `remote/session` 里的对话不搬，新消息从新线程开始（Desk 切换时同样没有导入）。
  `/resume` 仍能接上 Claude Code 自己磁盘上的会话（见下表），所以真正的上下文不丢。

## 逐项去向

### 命令

| 命令 | 今天（`@cc` / `@tb`） | 迁移后 | 差异 |
|---|---|---|---|
| 普通消息 | 交给当前 agent（新用户默认 `@tb`） | 交给 chat 绑定的线程；没有绑定时按上节自动开文件夹线程 | `@tb` 的去向见下 |
| `@cc …` / `/cc …` | 切到 Claude Code 并发送 | 前缀被去掉，后面的话照常发送 | 无（已经是 Claude Code） |
| `@tb …` / `/tb …` | 切到 SmartGuide | 前缀去掉，后面的话照常发给绑定的线程（`@tb` 下线） | 无向导 |
| `/cd <路径>`、目录浏览器 | 设 `ProjectPath` | 换到该文件夹，下一条开新会话 | 无 |
| `/project`（`/p`） | 列本 chat 用过的目录，点选切换 | 改名 `/dir`（YZ：project、folders 都不是好词，命令名不用复数）：不带参数列出本 chat 用过的目录，`/dir N` 或 `/dir <路径>` 切过去（= `/cd`）；`/project`、`/p` 留作别名 | 只列文件夹，不列项目 |
| `/clear`、`/stop`、`/interrupt` | 清会话 / 停 | 已在 M2b 接管：`/clear` = 停 + 下一条开新会话 | 无 |
| `/new` | 无 | M2b | 新增 |
| `/status` | 当前 agent、会话、目录 | 绑在哪（项目或文件夹）、线程 `#N`、状态、模型、权限模式 | 内容换成 Projects 的 |
| `/resume [n]` | 列 Claude Code 磁盘会话，选一个续上 | 同样列出；选中后开一个文件夹线程，`SessionID` 用选中的那个（线程本来就用 `--resume <SessionID>` 续接） | 无 |
| `/yolo` | 切换本 chat 自动批准 | 切换绑定线程的权限模式（`bypassPermissions` ↔ 文件夹默认） | 作用在线程上，Web 上看得见 |
| `/verbose`、`/quiet` | 中间消息开关（默认开） | 同名，按 chat 记，**默认关**；见“输出” | 默认值变了 |
| `/bash <cmd>` | 在 chat 目录跑白名单命令 | 在绑定线程的目录跑，白名单照旧（`BashAllowlist`） | 无 |
| `/help` | 旧命令 | M5 的 Projects 帮助加上上面这些 | 一份帮助 |
| `/bind <code>`、`/join` | 配对、群白名单 | 不变（接入层） | 无 |
| `/mock` | 测试用假 agent | 删除 | 只用于测试 |
| `/threads`、`/focus`、`/back`、`@N` | 无 | M4，文件夹里同样可用；`/threads` 改名 `/session`：不带参数列出本项目或本目录的会话，`/session N` 切到 #N（替代 `/focus N`）；`/threads`、`/focus` 留作别名 | 新增 |

### 行为

| 行为 | 今天 | 迁移后 | 差异 |
|---|---|---|---|
| 审批与提问 | `IMPrompter`：按钮或文字回复，超时自动处理，全局“总是允许” | 权限审批用按钮，提问在聊天里直接回（下面“要补的能力”①） | 今天没有按钮的平台只发一行“去 Web 处理”，必须补 |
| 运行中再发消息 | 折进正在跑的回合（steer，`↪ Added to the running task`） | Projects 排队，本回合结束后再处理 | 已定：排队 |
| 中间输出 | verbose 时逐条发工具调用（聚合）、处理中、`[RESULT]`、Task done | Relay 只发最终回复和“需要你” | 要补②（verbose 时发聚合的工具行） |
| 图片和文件（入） | 下载到 `<目录>/.download`，消息里带 `<upload_file>` | 同样下载，作为线程消息的附件（`Send` 已支持附件） | 要补③ |
| 发文件（出） | `@tb` 的 `send_file` 工具 | `@tb` 下线后没有；以后需要再做“线程产出的文件经 Relay 发回” | 暂缺 |
| 常驻进程 | bot 级 `PersistentSession` 开关 | Projects 线程本来就常驻（引擎缓存进程，resolve 时回收） | 开关作废 |

### bot 级设置

| 设置 | 今天作用于 | 迁移后 |
|---|---|---|
| `DefaultAgent`（Claude Code profile） | 该 bot 所有 `@cc` | 该 bot 的 chat **新开**的文件夹线程用这个 profile（`FolderThreadSpec.Profile`）；已有线程不变，Web 上可改 |
| `DefaultCwd` | 没绑定目录时的 `@cc` 目录 | 没有绑定的 chat 第一次说话时的文件夹 |
| `Verbose` | 默认中间消息开关（默认开） | 同，但默认关（chat 级 `/verbose` 覆盖） |
| `BashAllowlist` | `/bash` | 同 |
| `SmartGuideProvider/Model` | `@tb` 的模型 | 作废 |
| `PersistentSession` | `@cc` 进程是否常驻 | 作废（Projects 总是常驻） |
| `RequirePairing`、访问控制、平台凭据 | 接入 | 不变 |

## `@tb` 的去向（已定：A）

`@tb` 是给“还不知道在哪个目录干活”的人用的向导：读文件、跑白名单命令、`change_workdir`、`send_file`、技能。三个做法：

- **A. 下线 `@tb`（推荐）**：没有绑定的 chat 直接进 `DefaultCwd` 的文件夹线程，由 Claude Code 回答；
  `change_workdir` 由 `/cd`、`/dir` 承担。Claude Code 本身就能探索、读文件、回答问题，`@tb` 的提示词也写着“重活交给 `@cc`”。
  代价：`afk/` 失去唯一的使用者（是否保留由它自己的路线决定）；`send_file` 需要另找出路（例如线程产出的文件经 Relay 发回）；
  SmartGuide 的模型设置作废。
- **B. `@tb` 变成文件夹的“对话线程”**：给文件夹也配一个 `agent=llm` 的主线程，跑在 AFK 上，等于“没有 worktree 的 coordinator”。
  保留了 `@tb`，但 Projects 要多一种 agent 运行时和一种文件夹形态，用户要分清“跟向导说”和“跟 Claude Code 说”。
- **C. 保留 `@tb` 在 Remote**：与“不再留两套”相悖，只作为过渡。

## 开关（已定：合并）

YZ（2026-10-21）：两个开关合成一个，一起开、一起关。今天 Remote 只是 Power-ups 里一个**隐藏侧栏图标**的开关（bot 照常跑），
Projects 是后端实验开关（`projects`）。合并后只留 `projects` 这一个后端开关，Power-ups 里一行：

- **开**：Projects 与 IM（原 Remote）两个侧栏项都出现；所有 IM 聊天走 Projects。
- **关**：两个侧栏项都隐藏；chat 里的聊天控制关闭，回一句去设置里打开；bot 仍在运行，hook 驱动的通知（Flow A）照常，
  因为通知与聊天控制本来正交（bot-arch.md）。
- 名字（建议，第 4 期定）：开关叫 Projects，说明“在 Web 和 IM 里使用 Claude Code”；侧栏的 Remote 改名 IM（只剩 bot 连接与访问控制）。

## 要补的能力

① **聊天式回答**（YZ，2026-10-21）：只有**权限审批**（允许 / 本线程都允许 / 拒绝）用按钮；提问（AskUserQuestion）
和其他需要用户输入的，一律在聊天里直接回：提示列出编号选项，chat 回编号或自己的话，就是答案（多问题、多选、自由回答都能这样答，
不再受 M3“单选、最多 8 个选项才有按钮”的限制）。没有按钮的平台，权限审批也用文字回 `y` / `n` / `always`。
等待回答时，chat 的下一条普通消息就是答案，而不是发给线程的新消息；两条入口都交给 `Service.AnswerFromChannel` / `Threads.Respond`。
   **落地**（第 2 期）：Relay 给每个被问到的 chat 记下待答请求（内存，按 chat 排队，最早的先答；请求结束时清掉）。
   `PostFromChannel` 里 `@N` 优先，其次是待答请求，再其次才是普通消息。审批：`y`/`always`/`n`（含中文“是/总是/否”），
   **其他话一律视为拒绝并把这句话转给线程**（Claude Code 收到“用户说：……”，和 Web 上拒绝时附言一样）。
   提问：一个问题时整条回复就是答案；多个问题时一行一个；数字取选项（多选用逗号），否则原话作答。
   有按钮的审批消息正文不带回复提示（按钮自明），但照样能回 `y`；按钮消息结束后照旧改写。线程汇报里的 “Needs you” 改为提示 `@N 回答`。
② **verbose 中间输出**：Relay 在 chat 开了 verbose 时，把线程的工具调用按 `stream.go` 的聚合规则发出去（复用 `remote/control/render`）。
   **落地（第 2 期）**：开关存在绑定上（`ChannelBinding.Verbose`，默认关），`/verbose on|off` 设置，`/quiet` 等于 `/verbose off`，换线程（`/focus`）不变，解绑即清。
   只发给绑在这个线程本身的 chat。每次工具调用发一条：前面那段话（如果有）+ `→ ` 加 `describeTool` 的一行（和审批同一种说法）；
   最后一段话后面没有工具调用，就是回复，照旧由 `ThreadReply` 发给所有 chat，所以开 verbose 也不会重复。
   没有复用 `remote/control/render`：Projects 不能 import 根模块，而且一行一条已经够读，以后真嫌吵再做聚合。
③ **附件**：IM 的图片和文件下载到文件夹后作为 `Send` 的附件。
   **落地（第 2 期）**：Remote 照旧按 `FileStore` 的类型和大小限制下载，但放到临时目录，通过 `ProjectInbound.Files` 交给 Projects，返回后删掉。
   Projects 把文件复制进上传区（和 Web 附件同一个地方：项目的 library，或者会话自己的区；`/new` 后的第一条先放草稿区，开线程时搬过去），
   不往用户的文件夹里写，标题（caption）就是消息正文。带文件的消息不当作对待答请求的回复。
④ **`/status`、`/resume`、`/yolo`、`/bash`、`/dir`、`/cd`** 改为作用在绑定上（`ChannelCommand` 加命令）。

## 删除清单（迁移完成后）

`remote/session`、`remoteagent` 的 `ClaudeCodeExecutor` / `AgentRouter` / `stream.go` 的执行部分 / `exec_registry`、
`internal/db/remote_session_store.go` 与 `remote_import.go`、`IMPrompter` 在 chat 控制上的用法（通知路径若仍用则保留）；
选 A 时再加 `executor_smartguide.go`、`remote/control/smart_guide/`、SmartGuide 配置与模型对话框。
Remote 页面保留，只去掉 SmartGuide 模型和“常驻会话”这类作废项。

### 第 4 期 a 的落地：删掉旧的聊天执行路径

- **删了**：`remote/session`、`remote/control/smart_guide`、`remote/control/render`（只有 `stream.go` 用）、
  `remote/control/service.go`（`Core`）、`remoteagent` 的执行器 / `AgentRouter` / `stream.go` / `handoff.go` /
  `exec_registry` / 常驻会话池接线、`internal/db` 的 `RemoteSessionStore` 和旧 `bot_sessions.json` 导入
  （表 `remote_sessions` 不再 AutoMigrate，旧数据留在库里不动；`bot_chats.json` 的导入保留，chat 的目录和配对还在用）。
- **`remoteagent` 现在只做三件事**：接入（配对、群白名单、`/bind`、`/join`）、选目录（`/cd`、`/dir`、目录浏览器）、
  其余一律交给 Projects（`ProjectRouter`）。没有 router 或开关关着，chat 只有接入和通知。
- **命令注册表就是这 9 个命令加 `/help`**（`/bind`、`/join` 隐藏）：平台的命令菜单（Telegram 的 `/` 菜单）从注册表生成，
  所以菜单里不再有旧命令；会话类命令的处理函数直接转给 Projects。
- **`/cd` 只回一句**：`📁 <路径>` + “下一条消息在这里开新会话”。先在 Projects 里换目录，成功了才记进 chat 的目录历史（`/dir` 列的就是它）。
- **命令行改名 `tingly-box bot`**（YZ 2026-10-22：它管的是 bot，不是“remote”；不留 `remote` 别名）。
  `start` 删掉：它不经过服务端，跑的是旧路径，而 Projects 只在服务里运行；bot 随服务（`tingly-box start`）运行。
  `list`、`add`、`pair` 保留，`add` 不再问 SmartGuide 模型。`config` 保留但换了内容：原来只设 SmartGuide 模型，
  现在不带参数显示 bot 的设置（名称、平台、启用、配对、默认目录），`--name`、`--default-cwd`、`--enable`/`--disable` 修改，
  改完通知正在跑的服务重载；bot 可以用 UUID 或 `bot list` 里的序号指定。
- **没动（第 4 期 b）**：bot 设置里作废的字段（`SmartGuideProvider/Model`、`PersistentSession`、`BashAllowlist`、`Verbose`）
  和它们的 API、前端对话框、SmartGuide 路由规则（`tbclient`）；`ChatStore` 里不再用的列（当前 agent、bash cwd、verbose）；
  文案（Projects 开关说明、Remote 侧栏改名）；`DefaultAgent` 接到线程 profile。

## 分期（每期一个 PR）

1. **补能力**：①文字回答审批、②verbose 输出、③附件。只影响已绑定的 chat，可以先上。
2. **切换**：所有 chat 都走 Projects（自动文件夹绑定、`/cd`、`/dir`、`/status`、`/resume`、`/yolo`、`/bash`、`@cc` 前缀、`@tb` 下线），
   bot 级设置接到线程上。旧执行器还在但不再被调用。
3. **删除**：上面的删除清单。

### 第 3 期 a 的落地：每个 chat 都有“落点”

- **绑定可以只有文件夹**：`ChannelBinding.Folder`（路径）。有它时，chat 的下一条消息在这个文件夹里开新线程并绑上（文件夹不在
  Projects 里就 `OpenFolder` 建一个），然后清掉。从没说过话的 chat 绑定行只有 `Folder`、没有 `ThreadID`；
  这种行不在 Web 的绑定列表里出现（还没有线程可看）。`/cd` 是同一件事：线程还绑着，`Folder` 指向新目录，
  `/stop`、`/clear` 停的是旧线程，`/new <话>` 直接在新目录开，`/session`（`/focus`）换线程会清掉 `Folder`。
- **Remote 侧只做一件事**：开关开着时，任何消息先 `ToFolder(ifNew)`：chat 自己的 `ProjectPath`（旧数据或 `/cd`），
  不能用就退到 bot 的 `DefaultCwd`。之后整条路径就是“已绑定的 chat”，M1–M5 的路由不变。`/cd`、目录浏览器、`/project` 选择
  最后都落到 `chatStore.BindProject`，那里再调一次 `ToFolder`。
- **`@cc` / `@tb` / `/cc` / `/tb` 前缀**直接去掉；只有前缀没有话时回一句“直接发消息就行”。
- **开关关着**：除了 `/bind`、`/join`（接入层）之外，chat 收到一句“去打开 Projects”。没有 router 的 BotHandler（独立 CLI、测试 harness）完全不变。
- **升级迁移**：`projects` 开关从没被设置过、且有已启用的 bot，就在启动时打开一次；设过（开或关）就不动。
  否则在用 Remote 的人升级后会被切断，与“原来的用法照样能用”冲突。
- 前端：Power-ups 里 Remote 那一行去掉，Projects 那一行显示 bot 数；Remote 侧栏跟着 Projects 开关出现。
- 本期**没做**：`/status`、`/resume`、`/yolo`、`/bash`、`/dir`、`/session` 仍是旧实现（第 3 期 b，见“命令精简”）；`DefaultAgent` 还没接到新线程的 profile 上
  （bot 设置里存的是 Tingly Box 的 Claude Code profile，和 Projects 线程的 profile 是不是同一套 id 要先确认，放到第 4 期）。

## 命令精简（已定，YZ 2026-10-22）

新模式下聊天只做三件事：选在哪干（目录、会话）、控制当前会话、看状态。`/help` 只列这 9 个，**旧名字一律不留**（不做隐藏别名，用到了回“Unknown command”）：

| 命令 | 由谁执行 | 说明 |
|---|---|---|
| `/cd <路径>` | Remote → `ToFolder` | 换文件夹，下一条开新会话 |
| `/dir [N\|路径]` | Remote → `ToFolder` | 不带参数列出本 chat 用过的目录（原 `/project`），带参数 = `/cd` |
| `/session [N]` | Projects | 不带参数列出会话（项目的对话是 `#0`），带参数切过去；替代 `/threads`、`/focus`、`/back`、`/resume` |
| `/new [话]` | Projects | 下一条（或这句话）开新会话，正在跑的不受影响 |
| `/stop [#N]` | Projects | 停 |
| `/clear` | Projects | 停 + 下一条开新会话（YZ：保留） |
| `/status` | Projects | 文件夹或项目、会话 `#N`、状态、模型、权限模式、verbose |
| `/verbose on\|off` | Projects | 见“要补的能力”② |
| `/yolo` | Projects | 当前会话在 `bypassPermissions` 和继承的模式之间切换（YZ：保留）；`/new`、`/cd` 开的新会话沿用它 |

`@N <话>` 保留（不是命令）。`/bind`、`/join`（接入层）照旧，不进帮助。删掉：`/bash`（Claude Code 自己能跑命令）、`/mock`、
`/resume`（并入 `/session`；在 Projects 外开的 CLI 会话不列）、`/project` `/p`、`/threads`、`/focus`、`/back`、`/interrupt` `/s` `/i`、
`/quiet` `/noverbose`、`/h`，以及裸词 `stop`（现在是普通消息）。

## 已定（YZ，2026-10-21）

1. 命令名：`/cd` 保留；`/dir`（替代 `/project`）和 `/session`（替代 `/threads`、`/focus`）。规则是**名词命令不带参数就列出，带参数就切换**，
   所以只记两个词。旧名都留作别名。命令名不用 project、folder，也不用复数；`/list`、`/ls` 不用（不说列什么，`/ls` 会被当成列文件）。项目仍只在 Web 绑。
2. `@tb` 下线（做法 A）。
3. 中间输出默认关，`/verbose on` 打开。
4. 运行中再发消息：排队（Projects 现状）。
5. 审批与提问：只有权限审批是按钮，其余都在聊天里回答（见“要补的能力”①）。

