# Remote 的聊天控制并入 Projects

> 状态：**设计讨论**（未实施）。承接 [projects-im.md](./projects-im.md)（M1–M5 已落地：绑定了的 chat 走 Projects）
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
  这本来就是旧用法。⚠️ 这让 chat 里也能换绑（文件夹），见“待决定 1”。
- **会话历史不导入**：旧 `remote/session` 里的对话不搬，新消息从新线程开始（Desk 切换时同样没有导入）。
  `/resume` 仍能接上 Claude Code 自己磁盘上的会话（见下表），所以真正的上下文不丢。

## 逐项去向

### 命令

| 命令 | 今天（`@cc` / `@tb`） | 迁移后 | 差异 |
|---|---|---|---|
| 普通消息 | 交给当前 agent（新用户默认 `@tb`） | 交给 chat 绑定的线程；没有绑定时按上节自动开文件夹线程 | `@tb` 的去向见下 |
| `@cc …` / `/cc …` | 切到 Claude Code 并发送 | 前缀被去掉，后面的话照常发送 | 无（已经是 Claude Code） |
| `@tb …` / `/tb …` | 切到 SmartGuide | 见“`@tb` 的去向” | 待决定 2 |
| `/cd <路径>`、目录浏览器 | 设 `ProjectPath` | 换到该文件夹，下一条开新会话 | 无 |
| `/project`（`/p`） | 列本 chat 用过的目录，点选切换 | 列本 chat 用过的**文件夹**（Projects 里的文件夹线程），点选 = `/cd` | 只列文件夹，不列项目 |
| `/clear`、`/stop`、`/interrupt` | 清会话 / 停 | 已在 M2b 接管：`/clear` = 停 + 下一条开新会话 | 无 |
| `/new` | 无 | M2b | 新增 |
| `/status` | 当前 agent、会话、目录 | 绑在哪（项目或文件夹）、线程 `#N`、状态、模型、权限模式 | 内容换成 Projects 的 |
| `/resume [n]` | 列 Claude Code 磁盘会话，选一个续上 | 同样列出；选中后开一个文件夹线程，`SessionID` 用选中的那个（线程本来就用 `--resume <SessionID>` 续接） | 无 |
| `/yolo` | 切换本 chat 自动批准 | 切换绑定线程的权限模式（`bypassPermissions` ↔ 文件夹默认） | 作用在线程上，Web 上看得见 |
| `/verbose`、`/quiet` | 中间消息开关（默认开） | 同名，按 chat 记；见“输出” | 待决定 3 |
| `/bash <cmd>` | 在 chat 目录跑白名单命令 | 在绑定线程的目录跑，白名单照旧（`BashAllowlist`） | 无 |
| `/help` | 旧命令 | M5 的 Projects 帮助加上上面这些 | 一份帮助 |
| `/bind <code>`、`/join` | 配对、群白名单 | 不变（接入层） | 无 |
| `/mock` | 测试用假 agent | 删除 | 只用于测试 |
| `/threads`、`/focus`、`/back`、`@N` | 无 | M4，文件夹里同样可用 | 新增 |

### 行为

| 行为 | 今天 | 迁移后 | 差异 |
|---|---|---|---|
| 审批与提问 | `IMPrompter`：按钮或文字回复，超时自动处理，全局“总是允许” | M3 的 Projects 按钮；没有按钮的平台今天只发一行“去 Web 处理” | **缺文字回答**，必须补（下面“要补的能力”①） |
| 运行中再发消息 | 折进正在跑的回合（steer，`↪ Added to the running task`） | Projects 排队，本回合结束后再处理 | 待决定 4 |
| 中间输出 | verbose 时逐条发工具调用（聚合）、处理中、`[RESULT]`、Task done | Relay 只发最终回复和“需要你” | 要补②（verbose 时发聚合的工具行） |
| 图片和文件（入） | 下载到 `<目录>/.download`，消息里带 `<upload_file>` | 同样下载，作为线程消息的附件（`Send` 已支持附件） | 要补③ |
| 发文件（出） | `@tb` 的 `send_file` 工具 | 随 `@tb` 的决定 | 待决定 2 |
| 常驻进程 | bot 级 `PersistentSession` 开关 | Projects 线程本来就常驻（引擎缓存进程，resolve 时回收） | 开关作废 |

### bot 级设置

| 设置 | 今天作用于 | 迁移后 |
|---|---|---|
| `DefaultAgent`（Claude Code profile） | 该 bot 所有 `@cc` | 该 bot 的 chat **新开**的文件夹线程用这个 profile（`FolderThreadSpec.Profile`）；已有线程不变，Web 上可改 |
| `DefaultCwd` | 没绑定目录时的 `@cc` 目录 | 没有绑定的 chat 第一次说话时的文件夹 |
| `Verbose` | 默认中间消息开关 | 同（chat 级 `/verbose` 覆盖） |
| `BashAllowlist` | `/bash` | 同 |
| `SmartGuideProvider/Model` | `@tb` 的模型 | 随 `@tb` 的决定 |
| `PersistentSession` | `@cc` 进程是否常驻 | 作废（Projects 总是常驻） |
| `RequirePairing`、访问控制、平台凭据 | 接入 | 不变 |

## `@tb` 的去向（待决定 2）

`@tb` 是给“还不知道在哪个目录干活”的人用的向导：读文件、跑白名单命令、`change_workdir`、`send_file`、技能。三个做法：

- **A. 下线 `@tb`（推荐）**：没有绑定的 chat 直接进 `DefaultCwd` 的文件夹线程，由 Claude Code 回答；
  `change_workdir` 由 `/cd`、`/project` 承担。Claude Code 本身就能探索、读文件、回答问题，`@tb` 的提示词也写着“重活交给 `@cc`”。
  代价：`afk/` 失去唯一的使用者（是否保留由它自己的路线决定）；`send_file` 需要另找出路（例如线程产出的文件经 Relay 发回）；
  SmartGuide 的模型设置作废。
- **B. `@tb` 变成文件夹的“对话线程”**：给文件夹也配一个 `agent=llm` 的主线程，跑在 AFK 上，等于“没有 worktree 的 coordinator”。
  保留了 `@tb`，但 Projects 要多一种 agent 运行时和一种文件夹形态，用户要分清“跟向导说”和“跟 Claude Code 说”。
- **C. 保留 `@tb` 在 Remote**：与“不再留两套”相悖，只作为过渡。

## 开关

Projects 今天是实验开关（`projects`，默认关）。迁移后 IM 的会话层就是 Projects，所以：

- **Remote 开着，Projects 的运行时就要跑**（文件夹线程、Relay），不论 Projects 页面是否显示。没开 Remote 就没有 IM，与 YZ 的
  “没开 remote 就不会有 projects remote 的功能”一致。
- Projects 页面关着时，用户在 Web 上看不到这些线程，但 IM 里一切可用；打开 Projects 就能看到并接着用。（待决定 5：是否就此把 Projects 默认打开。）

## 要补的能力

① **没有按钮的平台用文字回答审批**（微信、企业微信、钉钉、Slack……今天 `@cc` 能回 `y`/`n`，Projects 只说“去 Web”）。
做法：提示里带编号，chat 回 `y`、`n`、`always` 或选项编号，交给 `Service.AnswerFromChannel`（与按钮同一个入口）。
② **verbose 中间输出**：Relay 在 chat 开了 verbose 时，把线程的工具调用按 `stream.go` 的聚合规则发出去（复用 `remote/control/render`）。
③ **附件**：IM 的图片和文件下载到文件夹后作为 `Send` 的附件。
④ **`/status`、`/resume`、`/yolo`、`/bash`、`/project`、`/cd`** 改为作用在绑定上（`ChannelCommand` 加命令）。

## 删除清单（迁移完成后）

`remote/session`、`remoteagent` 的 `ClaudeCodeExecutor` / `AgentRouter` / `stream.go` 的执行部分 / `exec_registry`、
`internal/db/remote_session_store.go` 与 `remote_import.go`、`IMPrompter` 在 chat 控制上的用法（通知路径若仍用则保留）；
选 A 时再加 `executor_smartguide.go`、`remote/control/smart_guide/`、SmartGuide 配置与模型对话框。
Remote 页面保留，只去掉 SmartGuide 模型和“常驻会话”这类作废项。

## 分期（每期一个 PR）

1. **补能力**：①文字回答审批、②verbose 输出、③附件。只影响已绑定的 chat，可以先上。
2. **切换**：所有 chat 都走 Projects（自动文件夹绑定、`/cd`、`/project`、`/status`、`/resume`、`/yolo`、`/bash`、`@cc` 前缀、`@tb` 按决定），
   bot 级设置接到线程上。旧执行器还在但不再被调用。
3. **删除**：上面的删除清单。

## 待决定

1. **chat 里能换文件夹**（`/cd`、`/project`）：这是旧用法，保留就意味着文件夹绑定有 chat 和 Web 两个入口；项目仍只在 Web。
   推荐保留（不留会让旧用户最常用的命令失效），并在 Web 的 IM 弹窗里显示“由 chat 的 /cd 设定”。
2. **`@tb`**：A 下线（推荐）/ B 变成文件夹对话线程 / C 暂留。
3. **verbose 默认值**：今天默认开（刷屏，imbot-output.md 记了噪音问题）；Projects 默认只发最终回复。推荐默认关，`/verbose on` 打开。
4. **运行中再发消息**：排队（Projects 现状，推荐，Web 也是这样）还是折进当前回合（`@cc` 现状）。
5. **Projects 开关**：只让 Remote 带起运行时（推荐，改动小），还是借此把 Projects 默认打开。
