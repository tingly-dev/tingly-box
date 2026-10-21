# Projects（协调者 + 线程）设计

> 调研：[`.research/claude-projects.md`](../.research/claude-projects.md)。本文是 tingly-project 对**新版 Claude Projects**（2026-09 重新设计）的复刻设计。
> 旧版"文件夹式 Projects"（知识库、项目对话）的实现被本设计取代并移除。

## 1. 产品定位

一个本地运行的"幕僚长"：用户把一串持续冒出来的工作丢进**一个对话**，协调者把它们拆成并行的**线程**（Claude Code 会话），
每个线程在自己的 git worktree 分支上干活，做完向协调者汇报。用户走开，回来只看 **Overview 里"等你处理"** 的部分，并在本地 review diff、合并。

和官方的差异定位（官方当前的空白，见调研 §7.8）：

| | 官方 | tingly-project |
|---|---|---|
| 线程跑在哪 | 云端（本地需 Remote Control） | **本机**，每个线程一个 git worktree |
| 模型 | 仅 Anthropic | 任意：通过 Tingly Box 网关路由，协调者和线程可分别选模型 |
| 代码托管 | 仅 github.com | 任意本地 git 仓库；有 GitHub remote 时可推送开 PR（P2） |
| 入口 | 网页、桌面、手机 | 本地网页（P2：IM） |

## 2. 体验要素（必须复刻）与 MVP 取舍

| 体验要素（调研 §4） | MVP | 说明 |
|---|---|---|
| 一个对话入口 + 协调者路由（就地回答 / 新线程 / 交给已有线程 / 一条多任务拆多线程） | ✅ | 协调者 = LLM + 工具 |
| 线程卡片出现在对话里（标题 + 状态，可点开） | ✅ | |
| Suggested threads（提议后由用户启动，可全部启动） | ✅ | `propose_threads` 工具 |
| 线程 = 完整 Claude Code 会话，独立分支 | ✅ | agentboot + git worktree |
| Overview：按状态分组，"Waiting on you" 为核心，按钮带小红点 | ✅ | |
| 打开线程：看过程、直接发消息、回答审批、停止 | ✅ | |
| 审批只在线程内回答；"仅此一次 / 本线程同类都允许" | ✅ | |
| 线程完成后向协调者汇报，协调者发更新 | ✅ | |
| 项目指令（≤16,000 字符，线程与协调者都收到） | ✅ | |
| 项目记忆 = 文件（`MEMORY.md` 索引 + 文件），Claude 自己写，设置里可看可改 | ✅ | 协调者 `remember/forget` 工具；线程直接写记忆目录 |
| 自然语言调节自主度，偏好写入记忆 | ✅ | 由协调者提示词 + 记忆实现，不做开关 |
| "缺东西就说缺什么然后停下"，线程需要输入时进入 Waiting on you | ✅ | 线程以 `NEEDS_INPUT:` 行结尾 |
| Goal（一句话目标） | ✅ | |
| 仓库（本地路径）、文件库 Library（上传文件 + 线程产出） | ✅ | |
| 协调者模型 / 线程模型分别设置 | ✅ | |
| Pause / Archive / Delete | ✅ | |
| 用量按线程 / 按模型 | ✅ 基础版 | 记录 token |
| Review：diff + 本地合并（"Merge it"） | ✅ | 本地版的 PR |
| 推送并开 GitHub PR、PR 自动修 CI / 回复 review | P2 | |
| Routines（定时） | P2 | |
| Setup recommendations / 首个项目自动探索线程 | P2 | |
| 从已有会话 "Continue as project" | P2 | |
| IM 通知与审批（imbot） | P2 | tingly-box 已有能力 |
| 共享 / 多用户 | ✗ | 官方也是单用户 |

## 3. 概念与命名（全局唯一，UX 原则 3）

- **Project**：一个长期目标及其工作流。
- **Conversation**（项目对话）：用户 ↔ 协调者的唯一对话。UI 文案：英文 "Conversation"，中文"项目对话"。
- **Coordinator**：项目对话里的 Claude。
- **Thread**：一个工作会话（Claude Code）。不叫 session/task/agent。
- **Overview**：线程状态面板。
- **Library**：上传文件 + 线程产出文件。
- **Memory**：记忆文件。**Instructions**：项目指令。**Goal**：一句话目标。

## 4. 架构

```
Browser ──REST──▶ HTTP modules (project / thread / settings)
   ▲                        │
   └──SSE /projects/:id/events ◀── Hub（项目内事件广播）
                            │
                 ┌──────────┴───────────┐
           Coordinator（每项目串行队列）   Thread Manager
           llm.Client + 工具循环          agentboot/engine → claude CLI
                 │                         git worktree / prompter / 汇报
                 └────── Store (SQLite) + 项目目录（memory/、library/、worktrees/）
```

### 4.1 项目目录（`$DATA/projects/<id>/`）
```
memory/MEMORY.md, memory/*.md   记忆（Claude 写，用户可改）
library/                        上传文件；线程产出写到 library/outputs/<thread-slug>/
worktrees/<thread-id>/          每个线程的 git worktree（无仓库时为空工作目录）
threads/<thread-id>.settings.json  线程的 Claude Code 设置（additionalDirectories 等）
```

### 4.2 数据模型
> 统一线程模型后的变化见 `../../.design/projects-sessions.md`：项目对话不再有单独的表，是主线程的事件；`worktree` 改名 `cwd`；工作目录是 `type=folder` 的项目。下面按新名字写。

- `projects`: id, name, type(project|folder), path(folder 的规范化路径), main_thread_id, goal, instructions, repos(JSON `[{path, base_branch}]`), coordinator_model, thread_model, thread_permission_mode, paused, starred, archived_at, timestamps。
- 项目对话 = 主线程（`agent=llm`）的 `thread_events`：kind(user|assistant|thread_started|suggestions|thread_update|error)，没有 role，由 kind 推出。旧的 `project_messages` 表迁移后只读保留一个版本。
- 线程回报（`thread_update`）照常传给协调者；对话里按发生的先后各占一条很淡的折叠行（状态、线程名、结果，点开看回报正文），线程卡片只负责实时状态；对话下方有前端开关可隐藏这些行（仅存本机 localStorage）。
- `threads`: id, project_id(必填), parent_id, seq(项目内序号，主线程为 0), agent(llm|claude-code), title, task, status, wait_reason, repo_path, base_branch, branch, cwd, session_id, model, last_report, commits_ahead, files_changed, input/output tokens, origin(coordinator|suggestion|user), timestamps, resolved_at。
- `thread_events`: 线程过程。kind(user|assistant|tool_use|tool_result|approval|ask|system|error|report|thread_started|suggestions|thread_update), content, payload, source(空=网页|im|thread|system), source_ref, source_platform, source_chat_id, created_at。
- 新线程的目录是 `threads/<thread-id>/`（旧线程保留 `cwd` 里记的 `worktrees/...`）。

### 4.3 线程状态（与官方 Overview 分组一致）

| 状态 | 进入条件 | Overview 分组 |
|---|---|---|
| `working` | 一轮在跑 | Working |
| `waiting` | 有待回答的审批/提问；或上一轮以 `NEEDS_INPUT:` 结尾；或失败（`wait_reason` 说明） | **Waiting on you** |
| `ready` | 一轮结束，分支上有领先于基线的提交 | Ready for review |
| `idle` | 一轮结束，无待审内容 | Idle |
| `resolved` | 用户标记 / 合并后自动 / 一周无活动 | Resolved（可重开） |

（官方的 Landing 对应"PR 已批准排队合并"，本地合并是同步动作，MVP 不需要该状态。）

### 4.4 线程运行
- 线程用 `agentboot/engine` 跑，和原来 Tingly Box 的 Desk 是同一个会话引擎（Desk 现已并入 Projects，成为工作目录）（`claude` CLI，stream-json，`--resume` 续会话，`PermissionPromptTool: stdio`）。引擎负责进程、转换和后台任务；本仓库只做线程这一层：排队、项目并发上限、线程内审批、每轮汇报。引擎按 Claude 会话 id 认线程（`Thread.SessionID`）。
- **常驻进程**：每个线程保留一个常驻 `claude` 进程（池上限 10，空闲 10 分钟回收，之后的轮次带 `--resume` 重开）。后台任务（后台命令、子代理）因此能活过发起它的那一轮；任务结束后 Claude 自己接着跑的那一轮，照常定状态、汇报协调者。Stop 只中断当前轮，不杀进程；Resolve 才关掉进程。
- **事件**：引擎的对话记录条目转成线程事件（`transcript.go`）：主对话的文字、工具调用和结果照旧；子代理内部步骤不进线程视图；思考不显示；后台任务结束时留一行系统事件。
- 测试用 `agentboot/engine/enginetest` 的脚本替身，另有一个用 agentboot fixture 跑真实 Claude 驱动的用例。
- **工作区**：项目有仓库 → `git worktree add -b tp/<slug>-<id6> <worktree> <base>`；没有仓库 → 空目录。
- **上下文注入**（`--append-system-prompt`）：项目名、Goal、Instructions、`MEMORY.md` 内容、记忆/文件库目录说明、线程规则：
  - 只做分配的这一件事；在当前分支上提交（commit），不 push、不改其他分支；
  - 需要的东西缺失时，说明缺什么并停下，不要 mock 或猜；
  - 需要用户决定才能继续时，最后一行写 `NEEDS_INPUT: <问题>`；
  - 结束时给出简短汇报：做了什么、怎么验证的、分支上有什么；
  - 记住项目级事实：在记忆目录写文件，并在 `MEMORY.md` 加一行索引。
- **目录权限**：生成 settings 文件，`permissions.additionalDirectories` = [library, memory, 其他仓库]，`--settings` 传入。
- **连接**：全局连接为 Anthropic 风格时，线程继承它（`ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN`），即默认走 Tingly Box；否则用本机 `claude` 自己的登录。
- **权限模式**：默认 `acceptEdits`（编辑自动通过，命令等需审批 → Waiting on you）；项目设置可改为 `auto` / `bypassPermissions`。
- **审批**：prompter 把请求记为线程事件、线程进入 `waiting`、阻塞等待用户在线程里回答（权限审批不超时）；"本线程同类都允许"按工具名记住。
- **自动回答问题**（AskUserQuestion）：每个线程一个设置 `auto_answer`：`off` / `delay` / `now`，空为按种类的默认值（项目线程 `delay`，用户多半不在；工作目录会话 `off`，用户多半在看）。只有每个子问题都有标了 “(Recommended)” 的选项、且都不是多选时才会自动回答，其余照旧等人。`delay` 在提问 10 分钟后按推荐项回答，`now` 立即回答；待回答事件带 `auto_at`，卡片写明“几点前没回答就按 X 继续”，自动回答后事件带 `auto: true`，记录里标“已按推荐答案自动回答”。改设置时正在等的问题按新设置重新计时。官方的决策卡也是“推荐一项、没人回答就按它继续”，这里只是把等待时长做成可选。未定：协调者是否要在汇报里提到这次猜测。
- **消息排队**：线程忙时收到的消息（用户或协调者）排队，当前轮结束后依次执行。
- **一轮结束后**：读取 git 状态（领先提交数、改动文件数）→ 定状态 → 记一条 `report` 事件 → 通知协调者。

### 4.5 协调者
- 每个项目一个串行队列；触发源：用户消息、线程汇报（完成 / 需要输入 / 失败）。
- **按需构建上下文**（与官方一致：基于最近消息、线程表和记忆，而不是全量历史）：系统提示 = 角色 + Goal + Instructions + MEMORY.md + 仓库列表 + 线程表（id、标题、状态、最近汇报摘要）；消息 = 最近 N 条项目对话。
- **工具**：`start_thread`、`propose_threads`、`message_thread`、`get_thread`、`resolve_thread`、`remember`、`forget`、`update_instructions`。
- **路由规则**（提示词）：快速问题就地答；新工作开线程；同领域的后续交给已有线程并说明交给了谁；一条多任务拆多线程；用户表达的协调偏好（先提议、最多 N 个并行、更新简短……）用 `remember` 记下并遵守。
- 线程汇报触发的协调者回合：简短更新，只在完成、受阻或需要用户时发言。
- 并发上限：偏好由记忆表达；另有安全硬上限（默认同时 4 个运行中线程，超出排队）。

### 4.6 实时
`GET /api/v1/projects/:id/events`（SSE）：`message`（新增 / 更新，协调者流式文本）、`thread`（状态变化）、`thread_event`（线程过程新增）。前端进入项目页订阅，其余数据走 REST。

`GET /api/v1/projects/events`（全部项目，不含流式文本和 `thread_event`）驱动侧栏和文件夹页共用的项目列表。`thread` 事件带上该项目变化后的线程计数（`threads`），列表据此就地更新那一行；`project` 事件就地合并（归档即移除）。只有遇到列表里没有的项目、流重连、标签页回到前台、页面报告变化时才重新拉 `GET /projects`，另有 5 分钟的兜底轮询。事件流不受服务器 `WriteTimeout` 限制。原因、不变量和被否决的做法见 `live-updates.md`。

### 4.7 Review 与合并（本地版 PR）
- `GET /projects/:id/threads/:tid/diff`：`git diff <base>...<branch>`（+ 未提交改动提示）。
- "Merge it"：仅当主仓库工作区干净且当前在 base 分支时执行 `git merge --no-ff <branch>`；否则给出原因和可复制的命令（UX 原则 11）。成功后线程 → `resolved`。
- 删除项目：移除 worktree，保留分支（与官方"不影响分支和 PR"一致）。

## 5. UI

- **项目页**：左侧项目对话（消息、线程卡片、提议卡片、输入框）；右侧 **Overview**（Threads 按状态分组 / Library / Memory 入口），点线程卡片或行 → 右侧切到线程详情（过程、审批、diff、操作、线程输入框）。头部：名称、收藏、Overview 开关（等你处理时有小红点）、设置。
- **输入框上方的信息面板**：线程和工作目录会话的输入框正上方一块面板，回答“在哪跑、哪个分支、会不会等我”，一条信息占一行（图标、名称、值、行尾操作）：文件夹（完整路径，长了保留结尾；复制 `cd`）、当前分支（项目线程是 worktree 分支，会话读文件夹当前分支，每轮结束刷新；复制分支名）、自动回答（点值切换）。面板可折叠（默认折叠，按浏览器记住）；折叠时只露当下最相关的一条，其余记成 “+N”：先是不需要用户也会发生的事（等待中的问题几点自动回答，将来 CI 失败），再是会变的状态（当前分支，将来 PR / CI），文件夹路径只在没有别的可露时才露。这样宽屏窄栏都只占一行，加信息也不会把这一行越撑越长。路径和分支只在这里出现，不再在线程头部重复；以后的 PR / CI 等信息再加一行即可。
- **项目设置**：General（名称、Goal、协调者模型、线程模型、权限模式、Pause / Archive / Delete）、Memory（Instructions + 记忆文件）、Environment（仓库）、Usage。
- **新建项目**：名称必填，Goal、仓库（本地路径，即时校验是否 git 仓库并显示当前分支）可选。
- 空项目首屏：教育内容内嵌（UX 原则 8）——说明"把任务丢进来、它会开线程、回来看等你处理"，并给 3 个可点的示例任务。

## 6. 与旧实现的关系
- 移除：知识库（`internal/knowledge`）、旧项目记忆（`internal/memory`）、项目内聊天。
- 保留：独立聊天（Chats）作为轻量功能，与项目解耦；设置页（模型连接）继续作为协调者的连接。

## 7. 验证
- 后端：fake LLM（脚本化工具调用）+ agentboot fixture（脚本化 Claude CLI 输出）+ 临时 git 仓库，覆盖：路由开线程、线程提交 → ready、审批 → waiting → 允许 → 继续、NEEDS_INPUT、汇报回到协调者、合并、暂停。
- 前端：typecheck / lint / vitest；无头浏览器走一遍主流程截图。
- 真机：本机 `claude` CLI 指向 fake Anthropic 服务跑通一个真实线程（P1 末）。

## 8. 状态（2026-10-11）

MVP 已实现（§2 中标 ✅ 的部分）。验证：
- 后端单测与 HTTP 端到端测试（含真实 Anthropic SDK 的 tool_use 流），`go test -race` 通过。
- **真机**：本机 `claude` CLI（2.1.296）作为线程，指向一个模拟 Anthropic 服务：协调者 `start_thread` → Claude Code 建 worktree 分支、请求 `Bash` → 线程进入 Waiting on you → 在 UI 允许 → 提交 → Ready for review → diff → Merge 落到真实仓库。

已解决的问题：
- **agentboot 进程句柄的竞态**（上游 tingly-box）：`process/osexec.go` 启动后立即在后台 `cmd.Wait()`，而 `Wait` 会关闭 `StdoutPipe` 的读端；CLI 写完 result 后立即退出时，解码器最后一次读会报 `read |0: file already closed`，成功的一轮被判为失败。已在 [tingly-dev/tingly-box#2022](https://github.com/tingly-dev/tingly-box/pull/2022) 修复（stdout 改为自有管道，直到 `Handle.Wait` 才关闭），tingly-project 已升级到包含该修复的 agentboot，并移除了临时容错代码。

## 9. 嵌入 Tingly Box（2026-10-12）

这一节是嵌入契约。Tingly Box 那一侧怎么接（Host 适配器、开关、导航、Desk 改名），写在仓库根目录的 `.design/projects.md`。

### 9.1 为什么搬进 tingly-box 仓库（2026-10-15 改）

| 方案 | 做法 | 取舍 |
|---|---|---|
| **搬进 tingly-box 做嵌套模块（采用）** | `tingly-box/projects/` 自带 go.mod，前端进同一个 pnpm workspace | 改接口、改引擎、改共享组件都只需一个 PR，不用钉版本也不用复制；代价是 tingly-box 变大，Projects 跟着它的 CI 和发版 |
| submodule 引用（原方案） | tingly-box 挂 `libs/tingly-project`，用 `replace` 指过去 | 两边独立节奏，但 agentboot 要“先合 TB、再升 tp”，`@tingly/ui` 要复制，前端组件没法共用 |
| 平铺进 tingly-box `internal/` | 代码直接并入根模块 | 边界只能靠自觉，很快会反向依赖；不能再独立运行 |

改主意的原因：两个仓库互相依赖（tp 用 TB 的 agentboot、swagger、`@tingly/ui`，TB 要嵌入 tp），每次跨仓库改动都要两个 PR；而 Desk 和线程页的对话记录组件要共用。搬进来用的是 `git subtree`，保留了原仓库的提交历史。边界设计不变：`projects` 仍是独立 module，不 import 根模块，CI 检查。

### 9.2 后端边界

依赖只朝一个方向：Tingly Box 根模块 → `projects` → `tingly-box/agentboot`、`tingly-box/swagger`。本模块绝不 import Tingly Box 根模块（`task projects:boundary` 用 `go list -deps` 按 module 检查，CI 的 `test-projects` 也跑）。同在一个 go.work 里时，根模块的包是解析得到的，所以这条检查比以前更要紧。对外只有两个公开包：门面 `projects` 和数据类型 `llm`，其余都在 `internal/` 下。

- **门面**：`New(Options{DataDir, Host})`、`Register(api *swagger.RouteGroup)`（只挂路由，不启动任何东西）、`Start(ctx)`（恢复线程、跑后台任务；幂等，开关打开时再调用即可）、`Close()`。`DataDir` 为空时用内存库，供宿主生成 OpenAPI。
- **Host**：宿主提供协调者的模型客户端（`ClientFor(ctx, model)`）、可选模型（`Models`）、线程里 Claude Code 的环境和默认模型（`ThreadEnv`）。独立版由设置模块实现；Tingly Box 用网关实现（`ThreadEnv` 复用它的 `tbclient.GetClaudeCodeEnv`）。

### 9.3 路由、鉴权、存储

- **路由**：全部在 `/api/v1/projects` 下，嵌入和独立两种模式路径一致。线程是 `/projects/:id/threads/:tid/*`，只能经由所属项目访问；另有 `/projects/models`、`/projects/runtime`、`/projects/repos/inspect`。设置、Chats、health 只在独立版注册（`server.build`）。
- **鉴权**：嵌入时挂在宿主的 V1 组下，受宿主的 Bearer token 保护。独立版无鉴权、只听本机。
- **存储**：嵌入时 `DataDir` 是 `<tingly-box 数据目录>/projects`，数据库和项目目录（memory、library、worktree）都在这里，不和宿主主库合表：各自迁移、互不阻塞，删掉这个功能就是删一个目录。
- **启停**：宿主的开关关闭时不调用 `Start`，不恢复线程，也不拉起 `claude` 进程。
- **OpenAPI**：宿主生成 schema 时调用 `Register` 但不调用 `Start`，所以宿主的 `openapi.json` 会包含这些路由；本仓库也保留自己的一份，两边 codegen 互不依赖。

### 9.4 前端

- **入口**：`src/embed.ts` 导出 `ProjectsHostProvider`、`ProjectsPage`、`ProjectPage`。宿主通过 `ProjectsHost` 注入带鉴权的 `fetch`（事件流和 Library 下载也走它）、路由位置（`paths.list`、`paths.project(id)`）和 `onProjectsChanged`。事件流用 fetch 读取，不用 `EventSource`，因为后者带不了鉴权头。页面不带自己的 `ThemeProvider`，用宿主的。
- **外观**：独立版和 Tingly Box 共用 `@tingly/ui`（主题、图标、页面原语：`PageHeader`、`EmptyState`、`ConfirmDialog` 等），即同一 pnpm workspace 里的 `frontend/packages/ui`。
- **挂载方式**：宿主把本仓库前端加进自己的 pnpm workspace，用 `lazy()` 挂页面。不用 iframe，也不在子路径下另挂一个 SPA，否则会出现两套侧栏、两套主题和两套登录状态。
- **共享依赖**：react、react-dom、@mui/material、react-router-dom、i18next、react-i18next 作为 peerDependencies，只打包一份；i18n 用独立命名空间 `projects`。
- **包内路径**：`@/` 别名属于 Tingly Box 的应用，`@tingly/projects` 内部一律用相对路径。
- **外边距**：宿主传 `embedded: true`（`ProjectsHost`）时，页面去掉为独立版浮动侧栏按钮留的位置，项目页标题前加回到列表的链接；宿主布局给 Projects 路由不留内边距。另有可选的 `notify`，让 toast 走宿主的通知。

### 9.5 产品边界

Tingly Box 原来的 Desk 也驱动 Claude Code，回答的是另一个问题：坐下来和一个 Claude Code 一起干活，而项目是丢一串任务进来就走开，回来只看“等你处理”。现在两者合在 Projects 里：项目负责后者，工作目录会话负责前者（见根目录 `.design/projects-desk-absorption.md`）。嵌入后项目设置里没有“模型连接”一栏，模型选择器列的是宿主里可用的模型。Chats 只在独立版保留。

### 9.6 步骤与风险

1. 本仓库改边界，行为不变（tingly-project #4）。
2. 统一视觉：tingly-box 抽出 `@tingly/ui`（#2038），复制接入并改用其原语（tingly-project #5）。
3. 搬进 tingly-box：Go 部分进 `projects/`（go.work、Taskfile、CI）；前端进 `frontend/packages/projects`（`@tingly/projects`），删掉 `@tingly/ui` 的复制，`@/` 改成包内路径。
4. 共享对话记录：Desk 和线程页共用 `@tingly/projects/transcript`。
5. tingly-box 接入：Host 适配器、`projects` 开关、路由、导航入口、i18n。
6. 数据迁移：已有 `~/.tingly-project` 时一键导入。

| 事项 | 风险 / 问题 | 建议 |
|---|---|---|
| worktree 路径 | git 记录的是绝对路径，直接搬数据目录会断链 | 导入时跑 `git worktree repair`；失败就保留旧目录继续用 |
| agentboot 版本 | 搬进来以后 `replace ../agentboot`，永远用同一份 | 已解决 |
| 并发 | 项目线程和工作目录会话都拉起 `claude` 进程 | 共用同一个引擎（`agentboot/engine`）和同一个 `pool` |
| 独立二进制 | 是否继续维护 | 保留：开发调试和离线演示都靠它 |
