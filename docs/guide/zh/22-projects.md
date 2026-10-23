# Projects

路径：`/projects`（你的项目）· `/projects/:id`（某个项目）· `/projects/folder`（工作文件夹及其会话）· `/projects/im`（IM 聊天及其绑定位置）

![Projects](../images/projects.png)

**Projects** 通过 Tingly-Box 在网页——以及你的 IM 聊天——里运行 Claude Code。它取代了原来的 *Desk*：普通的工作文件夹现在是一等公民，项目则是更大、更长期的形态。一切都经由网关的 `claude_code` 场景，所以你的[路由规则、配额和用量](./03-scenario-claude-code.md)对它的作用与对 CLI 完全一样。

> **默认关闭。** Projects 会在主机上运行本地编码 Agent，所以它是一个 **Beta** 的 [Power-up](./19-experimental.md#projects)：在 Power-ups 菜单（或 **System → Experimental**）里打开 *Projects*。同一个开关还会打开 **IM** 导航项，并让 IM 聊天可以与 Claude Code 对话（见 [Remote](./12-remote-control.md)）。能登录这个 Tingly-Box 的人，或者可以通过它的机器人使用 Remote Control 的人，随后就可以在这台机器上、在他们选择的文件夹和仓库里运行 Claude Code，并批准它的工具调用。机器上必须装有 Claude Code；没有的话，项目的 **Environment** 标签页会提示并给出安装命令。

两种形态放在同一个导航项（**Projects**，有东西在等你时带橙色**圆点**）之下：

| | 是什么 | 谁和 Claude 对话 |
|---|---|---|
| **Working folder（工作文件夹）** | 机器上一个已有的文件夹。会话*直接在这个文件夹里*运行，不用 worktree | 你本人——一个任务一个会话 |
| **Project（项目）** | 超出单次会话的目标：一个**协调者（coordinator）**对话，加上若干**线程（thread）**，每个线程是在自己的 git worktree 和分支里的一次 Claude Code 会话 | 你和协调者对话；它为每个任务开一个线程，并告诉你哪里需要你 |

侧边栏同时列出两者：**Projects**（带 **+**）和 **Working folders**（带 **+**），然后是一行 **IM**（`3 bots`），打开 IM 页面。项目行显示有多少线程*在等你*（`2 threads wait on you`）；加星标的项目排在最前；文件夹行显示它的路径和等待中的会话。

---

## Working folders（工作文件夹）

![工作文件夹](../images/projects-folder.png)

用 *Working folders* 旁的 **+** 添加（*Enter an existing folder on the machine running this app. Sessions you start there work in the folder itself, with no worktree.*）。点击侧边栏里的文件夹查看它的会话；`/projects/folder` 把所有文件夹放在一起。文件夹可以从侧边栏移除（**Remove from the sidebar**——其中运行中的会话会停止，过去的会话保留）。

**开始会话**——页面以 *What should Claude work on?* 和一个消息框开头。消息框下面是几个安静的选择器：**工作文件夹**、**Claude Code configuration profile**（*Default configuration profile* = 主 `claude_code` 场景，或你的某个 [Profile](./03-scenario-claude-code.md#profile-管理)）、**模型**和**权限**：*Ask for everything · Accept edits · Auto · Plan only · Don't ask · Never ask*（默认跟随项目/连接）。**Attach** 添加文件（需要先选文件夹）。右侧的 **Sessions** 面板把会话分成 **Waiting on you**、**Working**、**Done** 和 **Archived**；**New session** 再开一个。

### 一次会话

![工作文件夹里的会话](../images/projects-folder-session.png)

对话使用共享的转录视图：你的消息、Claude 的文字，以及**每次工具调用一行**（*Read src/…*、*Ran 1 command, edited 1 file · 1m 0s*），可展开看详情；子 Agent 的运行显示为单独的行，可以点开阅读它们的步骤。

- **审批**——*Allow Bash? `npm test`*，带 **Allow**、**Always allow Bash here** 和 **Deny**；卡片会说明这次审批是为了什么。等待中的会话显示橙色的 *Waiting on you* 状态并点亮导航栏圆点
- **提问**——Claude 提问时，选项会显示在一张卡片上（见下）。可以选一个、写自己的话，或 **Decline**
- **消息框上方的信息面板**——*This session*：**Folder**（复制 `cd` 命令）、**Branch**（复制分支名）和 **Auto-answer**（见下）。可折叠
- **消息框**——Claude 忙碌时你发的内容会排队：*Message this session — it reads it after its current turn…*，**停止**按钮结束当前回合
- **头部**——后台任务的 **running** 计数（点开可查看 Claude 在后台运行的命令和子 Agent，并可停止它们）、**Archive session**（对勾图标；**Reopen session** 可恢复）以及 ⋮：*Show task*、*Show trajectory*（逐步的回顾视图）、*Expand all tool calls*、**Continue in terminal**（见下）
- 消息框下方的**路由状态行**：哪些规则在路由这个会话（`cc → claude-sonnet-4-5 @ Anthropic (team)`）、已用上下文（`29% · 57.5k in / 1.4k out · cache 84%`）以及背后 Provider 的配额（`session 14% left · weekly 81% left`）；Provider 配额耗尽时会提示换一个 Profile
- **Continue in terminal**——给出在这台机器的终端里恢复同一个 Claude Code 会话的命令。同一时间只用一个地方：在这里发消息会再次启动会话；后台任务会随会话的进程一起停止（对话框会说明）

**Auto-answer**（信息面板）——当问题带有建议答案时怎么做：**Off**（每个问题都等你）、**After 10 min**（十分钟内你没回答，线程就按建议继续工作）或 **Right away**（立刻按建议继续；你仍可以用一条消息来纠正）。提问卡片会写明时间：*Goes with "Exponential with jitter" at 08:55 AM if you don't answer*。

---

## Projects（项目）

![项目列表](../images/projects.png)

`/projects` 以卡片列出你的项目——名称、目标、状态计数（*2 Waiting on you · 1 Ready for review · 1 Working*）、仓库以及最近活跃时间；收藏的带星标，暂停的带暂停图标。标签页：**Your projects** 和 **Archived**。**New project** 打开：

![新建项目](../images/projects-new.png)

- **Name**：*Scope it to one stream of work you will keep adding to*（范围限定为你会持续添加的一条工作流）
- **Goal (optional)**：协调者朝着它工作的一行目标
- **Repositories (optional)**：线程工作的本地 git 仓库。每个线程有自己的 worktree 和分支，从所示分支开始。**没有仓库时，线程把文件交付到 Library**

当工作有一个超出单次会话、并会不断产生任务的目标时，项目就值得——例如跨多个仓库的一次改动、你会持续丢 bug 进去的一块区域，或比一次会话更大的迁移。

### 一个项目

![一个项目](../images/projects-project.png)

页头是 `Projects / <名称>`，带目标、**已绑定的 IM 聊天**（每个聊天一个标签，加 **Bind chat**，见下）、**Layout** 菜单、**星标**、**Project settings**（齿轮）和 **Overview** 开关。左侧是与协调者的**对话**；右侧是 **Overview**，有 **Threads**（`· 2 waiting`）和 **Library** 两个标签页。

- **与协调者对话**：*Describe work, paste a bug report or stack trace, or ask a question…*——一个任务、一个清单、一份 bug 报告。它为每个任务开一个线程，并以每线程一行的方式汇报：标题、分支（`projects/t-sdk`）、`N commits`，以及有东西可合并时的 **Review changes**。线程汇报可以隐藏（**Hide thread reports**）。空项目会给出三个一键示例
- 消息框下方**选模型**：**Thread model** 和 **Coordinator model**（留空 = 连接默认值；线程用更便宜的模型最省钱），以及线程运行时的**权限模式**
- **Threads** 分组：**Waiting on you · Ready for review · Working · Idle · Resolved**，每个带最新一行和时间
- **Library**：你添加的文件（所有线程都可读）；没有仓库的线程把成果交付到这里
- 项目被**暂停**（线程停止、不接新工作）或**归档**（只读，解绑聊天）时，会有横幅说明，并带撤销按钮

### 一个线程

![等待提问的线程](../images/projects-thread.png)

点击线程会在对话旁边打开它（或点汇报里的 `Open thread`）。它与会话是同一种转录视图，另外还有：

- **提问卡片**——*用选项回答 Claude 的提问*：标题写明主题，每个选项有标题和说明（推荐项有标记），**Or answer in your own words** 接受自由文本；**Answer** 或 **Decline**
- 标题下的**状态**（`Waiting on you`）、标记自上次查看以来新内容的 **New**，以及运行时的**计时器**
- **Mark resolved**（对勾）——线程完成时一键标记；**Reopen** 可再给它发工作。⋮ 菜单还有 *Show task*、*Show trajectory*、*Expand all tool calls* 以及后台任务 / 子 Agent
- **Review changes**（待评审的线程）——线程提交相对基础分支的 diff、**Merge it**（检出必须在基础分支上且没有本地改动；worktree 中未提交的改动会给出警告且不属于合并内容）以及之后的 *Merged into main* 标记
- 消息框上方的**信息面板**：**Folder**（复制到 worktree 的 `cd`）、**Branch**（`projects/t-backoff` *from main*）、**Auto-answer**（见上）
- **消息框**——直接给这个线程发消息；它在当前回合结束后读取

![信息面板（展开）](../images/projects-thread-info.png)

### Layout（布局）

![Layout 菜单](../images/projects-layout-menu.png)

**Layout** 菜单选择各个面板在哪里打开——保存在当前浏览器，窄屏上隐藏（窄屏使用单个抽屉）：

| | 选项 |
|---|---|
| **Overview sits in** | *Side pane*（默认）· *Window*（可拖动、可缩放的浮动窗口，最小化为一个药丸，有东西等你时带圆点）· *Top bar*（每个状态一列，最需要你的排前面；折叠后是一行计数） |
| **Threads open in** | *Side pane*（默认）· *Floating window* · *Floating, from the Overview*（从浮动的 Overview 里打开的线程也在自己的窗口里打开）。当 Overview 在顶部时线程在它下方打开，所以这个选项会置灰并说明原因 |

### 项目设置

![项目设置](../images/projects-settings.png)

- **General**：名称和目标；**Coordinator model** 和 **Thread model**（留空 = *Connection default*）；**Thread permissions**：*edits run, other actions ask you* · *everything asks you* · *Claude Code decides*（需要支持 Claude Code auto 模式的模型）· *never ask*（仅用于一次性环境）；**Pause project**（立即停止所有运行中的线程，恢复前不接新工作）和 **Archive project**（隐藏项目、停止它的线程并解绑它的 IM 聊天；一切仍可阅读）
- **Memory**：**Project instructions**（每个新线程和协调者开始时依据的简报：项目的用途、工作发生在哪里、线程如何自检、哪些需要你点头）和 **Memory files**（Claude 记下的笔记——需求、决定、坑；可以在对话里让它记住或忘记某事，也可以在这里编辑；`MEMORY.md` 是每个线程都会读的索引）
- **Environment**：项目的 **Repositories**（新线程使用这些；已有线程保留自己的 worktree）和 **Thread runtime**：找到的 Claude Code CLI 位置，以及线程把模型调用发往何处（当它的连接是 Anthropic 风格时是网关；否则线程使用 Claude Code 自己的登录）
- **Usage**：线程上报的 Token 使用量，按模型和按线程列出（每个线程都是完整的 Claude Code 会话；并行线程消耗 Token 更快）

---

## IM

![IM 页面](../images/projects-im.png)

`/projects/im`（Projects 侧边栏底部的 **IM** 行）列出每个已连接的机器人——平台、`Connected` / `Not running`、**Access control**——以及它的聊天：**Chat**（Private chat / Group）、**Bound to**、**Status**（*Ready · Disabled · Waiting to pair*）和一个操作：**Bind** 或 **Move**。一个聊天**同一时间只绑定一个位置**——项目或工作文件夹；移动它会保留旧线程，不会删除任何东西。从未说过话的聊天只有一个文件夹，不会在这里列出。

在项目或文件夹页面上，页头的标签显示已绑定的聊天（点 **×** 解绑），**Bind chat** 打开选择器：会被移动的聊天标有 *Will move*，在访问控制中被禁用的聊天置灰。*Send the bot a message from the chat you want, and it shows up here.* 来自 IM 的消息在转录里带有平台标记（*From Telegram · 123456789*）。

聊天的行为——命令、聊天里的审批、Profile——见 [Remote](./12-remote-control.md#在聊天里与-claude-code-对话)。

---

## 相关页面

- [Remote](./12-remote-control.md)
- [Claude Code](./03-scenario-claude-code.md)
- [实验性功能 · Projects](./19-experimental.md#projects)
- [场景总览 · Power-ups](./02-scenario-overview.md#power-ups齿轮背后的菜单)
