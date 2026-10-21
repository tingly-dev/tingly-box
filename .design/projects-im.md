# IM 聊天并入 Projects

> 状态：部分实施（TEP #2143）。M1（#2147）和统一模型（#2151、#2154）已落地：chat 绑到线程（项目对话或文件夹里的线程），
> 消息走 Web 同一条路径，回复和线程汇报发回 chat。M2b–M5（`/new`、`/stop`、IM 里审批、点名线程、已绑定 chat 的 `/help`）已落地；
> 旧的 `@cc` / `@tb` 路径保留给没绑定的 chat（决定 3 已修改）；M6 路由仍是提案，见“里程碑”。承接 [projects-desk-absorption.md](./projects-desk-absorption.md)：Desk 已并入，
> 这里把 Remote 的 IM 聊天（`@cc`）也并进来，并**重新打开**那篇 Decisions §1“`@cc` 不共享、不导入”。
> 图见 [projects-im.pencil.md](./projects-im.pencil.md)（其中按 `target_kind`/`workdir_id`/`focus_thread_id` 画的图已被取代）。
>
> **数据模型以 [projects-sessions.md](./projects-sessions.md) 为准**：一切会话都是 Thread，IM 绑定指向线程（`thread_id`），
> 不再是 `project_id`/`workdir_id`/`focus_thread_id`。下面“数据模型”和“统一到 Project ID”两节保留为演变记录，其中与之冲突处以新文档为准。

## 决定

1. **Projects 是唯一数据源**。IM 里的一次对话就是 Projects 里的线程（或项目对话的消息），只多几个来源字段；
   不再有第二个会话存储。
2. **IM chat 绑定一个落点**：Project（对话对象是 coordinator）或 working folder（对话对象是那个文件夹里的会话，
   即今天的 `@cc`）。绑定属于 chat，不属于会话；换绑要显式，旧线程保留。
3. ~~**取消 `@tb` / SmartGuide**。没绑定落点的 chat，说话的对象就是 coordinator，由它帮用户选落点。~~
   **改为（2026-10-21，YZ）**：Remote 与 Projects 仍是两个功能。没绑定到 Projects 的 chat 照旧走 `@cc` / `@tb`；
   Remote 页面、SmartGuide、`remote/session` 保留，bot 仍在 Remote 里配置（没开 Remote 就没有 IM）。
   绑定仍只在 Projects 的 Web 上做（见“M5 的落地”）。下面第 5 条同样作废。
   之后 YZ 要求把 Remote 的原有行为也迁过来、不留两套：见 [remote-into-projects.md](./remote-into-projects.md)（讨论中）。
   以后 `@xxx` 让 coordinator 决定用哪个模型、profile、agent 去跟进（建立在 `Launcher` 和 `Thread.Model` 上），
   第一期只做“提示用户选”。
4. **Web 与 IM 可同时操作同一线程**：一个线程一个进程，审批和排队统一。
5. **Remote 页面整页并入 Projects**：bot 连接和访问控制进 Projects 侧栏的“IM”设置；聊天内容不再有独立视图。

## 现状里要动的东西

| 现状 | 位置 | 去向 |
|---|---|---|
| Chat（`ProjectPath`、`ProjectHistory`、`CurrentAgent`、配对/白名单、`BashCwd`…） | `remote/control/bot/chat_store.go` | **拆开**：配对、白名单、`ContextToken` 等“接入”属性留在 Remote；“绑定到哪”和历史搬到 Projects 的 `ChannelBinding` |
| Session（chat+agent+project 为键；`Profile`、`Model`、`PermissionMode`） | `remote/session` | 被线程取代；`Profile`/`Model`/`PermissionMode` 在 `Thread` 上已有同名字段 |
| `/bind`、`/cd`（目录浏览器选路径） | `remote/control/remoteagent/handler_bind.go` | 改成从 Projects 的项目和 working folder 里选；目录浏览器仍可用来**新建** working folder |
| SmartGuide 执行器 | `remote/control/remoteagent/executor_smartguide.go` | 删除；未绑定时转 coordinator |
| 执行 `@cc` 的执行器、审批/提问 | `executor_claude.go`、`ask` | 改为调用 Projects 的线程操作（Start/Send/Respond/Stop），不再自己起 engine |
| 输出渲染（流式、折叠、文件发送） | `remote/control/render`、`imbot-output.md` | 不动；输入换成线程事件 |
| Remote 页面 | `frontend/src/pages/remote-agent` | 删除，内容分到 Projects（见前端） |

## 数据模型（Projects 一侧）——已被 projects-sessions.md 取代

概念上，所有落点是同一种东西：**会话入口**（entry）。每个入口有一个 ID 和一个类别（kind），类别决定“谁来接话”：
`project` 由 coordinator 接话并开线程，`workdir` 直接是一个文件夹里的会话。以后加新的类别（例如只聊天的入口）
只是多一个 kind，不需要新的表和新的绑定方式。代码和接口里这个实体暂时仍叫 `Project`（改名要动整个 API 和数据库，
收益小），界面上的名字按类别分别叫“项目”“工作目录”。

```
Project                      -- 一切落点都是 Project，用 kind 区分
  kind                           project | workdir       （新增，默认 project）
  path                           workdir 的文件夹路径；project 为空
  (goal / instructions / coordinator / 合并 只对 kind=project 有意义)

Thread
  project_id                     必填（原来文件夹会话为空）

ChannelBinding               -- 一个 IM chat 当前绑在哪
  id, platform, chat_id          唯一键 (platform, chat_id)
  project_id                     必填；落点的 kind 决定路由
  thread_id                      kind=workdir 时的“当前会话”；kind=project 为空
  focus_thread_id                kind=project 时可选的“聚焦线程”；空 = 跟 coordinator 说话
  created_at, updated_at

Thread / project_messages        新增来源字段（可空，空 = Web）
  source             im | web
  source_platform, source_chat_id
```

- **为什么把工作目录也做成 Project**：绑定、线程、编号（`Thread.Seq`）、等待信号、来源字段都只认 `project_id`，不会再有
  “线程没有项目”和 `workdir_id` 两条旁路，以后加的字段也不会再分裂成两份。代价是项目相关代码要容忍“没有 coordinator”，
  但这集中在一个 `kind` 判断上。取代 [projects-desk-absorption.md](./projects-desk-absorption.md) 里“线程没有项目”（选项 1）的数据模型决定。
- **迁移**：升级时把 `Workdir` 表和 `project_id` 为空的文件夹会话迁成 `kind=workdir` 的项目（按文件夹路径合并，名字取文件夹名），
  线程的 `project_id` 回填后改为必填。没有可回退的数据丢失：原来的会话、事件、`session_id` 都不动。侧栏仍分“项目”“工作目录”两组。
- 为什么绑定存在 Projects 里：Web 端要显示“这个项目绑了哪些 chat”并改绑，IM 端要按它路由，
  放在 Remote 里两边都要跨模块查；Projects 是唯一数据源，所以放这边。
- 独立版 Projects 也有这张表，但没有任何东西往里写，行为不变（`Host` 可选能力，类型断言，同 Desk）。
- `ChannelBinding.thread_id`：workdir 绑定下 `/new` 开新线程并移动指针，旧线程留在 working folder 里。
- project 绑定下 IM 来的消息写进项目对话（`source=im`），coordinator 照常处理；线程由 coordinator 开，
  线程完成时的汇报**也**发回绑定的 chat（沿用现在的 `thread_update`，在 IM 里折成一行）。

## 路由与出站

```
IM 入站 ──▶ 访问控制（Remote：配对/白名单/Disabled，不变）
        ──▶ 查 ChannelBinding(platform, chat_id)
              ├ 无：转 coordinator（无项目上下文的轻量对话），回复里附最近落点供选择
              ├ project：追加项目对话消息(source=im)
              └ kind=workdir：Send 到 thread_id（没有则 Start 新线程）
Projects 事件 ──▶ 订阅了该落点的 binding ──▶ 渲染到 chat
```

- 回复前缀：每条消息带它来自哪里，避免用户忘了自己在哪：项目对话是项目名（`[tingly-box] …`），其他线程（项目的子线程、
  文件夹里的线程）是编号和标题（`[#2 修登录] …`），与 Web 上的编号一致。
- 一个 chat 同一时刻只绑一个落点；一个落点可以被多个 chat 绑定（手机和群各一个），出站按 binding 广播。

## 统一到 Project ID（已被取代）

曾提议给 `Project` 加 `kind`（`project`/`workdir`），所有落点认 `project_id`。讨论中确认：coordinator 对话、线程、文件夹会话、
IM 聊天本质上都是“与 agent 的一次交互”（Claude Code 自己也是这样存的：子 agent 是独立转录，渠道是消息的 `origin` 属性），
所以单位应当是 **Thread**，不是项目。完整设计、调研、命名和迁移见 [projects-sessions.md](./projects-sessions.md)。

## 在 IM 里点名线程

project 绑定的 chat 默认在跟 coordinator 说话；要对某个线程说话，需要能“点名”。点名是一次 `@`：

- **线程编号**：每个线程在项目内有稳定的短编号 `#1`、`#2`（新增 `Session.seq`，项目内自增，不复用）。线程的通知、
  汇报、审批、提问消息都带 `[#2 标题]` 前缀。
- **主路径 `@2 补上边界用例`**：行首 `@` 加编号，整条消息去给线程 2，不改变 chat 的状态，每次显式，不会发错。
- **顺手路径：引用回复**线程的某条消息，消息去给那个线程。不需要记编号，但容易忘记，所以它只是快捷方式，不是唯一入口。
  平台不支持引用回复时，这条路径不存在，`@` 仍可用。
- **兜底 `/focus #2`**：之后的普通消息都进线程 2，回复前缀 `[#2 …]`；`/back` 或该线程结束后退回 coordinator。
  聚焦就是把绑定改到那个会话，`/back` 改回它的父线程（projects-sessions.md），不改线程本身，用于长时间只盯一个线程的场景。
- **`/threads`**：列出项目的线程（“等你处理”排前），每行带按钮：聚焦、停止、查看最近一条回复。
- 审批和提问始终直接发到对应线程，按钮带编号，不受聚焦影响。
- 优先级：引用回复 > 行首 `@N` > 聚焦 > coordinator。
- working folder 绑定只有一个当前会话，不需要这些。

`@` 的另一个用法是 coordinator 路由（`@opus`、`@codex` 之类，决定模型、profile、agent，M6）。两者不冲突：
纯数字是线程编号，其余名字交给 coordinator 解析；名字不唯一或认不出时 coordinator 回问，不猜。

## 审批与排队

- 审批是线程事件（`EvApproval`），本来就只“在线程里回答”。现在它同时出现在 Web 和所有绑定的 chat。
  **谁先答谁算，另一边收到“已处理”并撤掉按钮**。这是统一后新增的并发点，必须有测试。
- 线程忙时的消息由服务端排队（线程已有行为），IM 与 Web 共用同一队列。

## 前端

- 删除 `pages/remote-agent`，其中文案（`remoteAgent.*`、`remoteControl.*`）迁到 `projects` 命名空间。
- 侧栏底部新增“IM”入口：机器人连接、访问控制、平台状态。这是配置，不是内容，所以不占主区。
- 项目页和 working folder 页的页头加“已绑定的 IM”小标签，点开管理绑定（新增/解绑/改绑）。
- 会话视图不变，IM 来源的消息加一个平台图标。
- 导航：rail 上的 Remote 入口去掉；等待信号（小红点）已覆盖 IM 来的审批，无需新增。
- 先放 API 占位函数（`ChannelBinding` 的 swagger 没生成前），按 CLAUDE.md 约定，之后跑 `task codegen`。

## 里程碑（逐步做，每个都能单独上线；IM 交互复杂，不要求一次到位）

原则：**绑定了才走新路径，没绑定的 chat 保持今天的行为**（`@cc`、`@tb` 照旧），所以每个里程碑都不破坏现有用法，
最后才下线旧路径。每个里程碑拆成后端和前端两个 PR，互不依赖，可并行；前端先接占位 API，后端合并后跑 `task codegen` 再接。

| | 范围 | 不做 | 状态 |
|---|---|---|---|
| **M1 只到 coordinator**（先做） | `ChannelBinding`（只支持 project）、消息来源字段、绑定的增删查接口；绑了项目的 chat 的消息进项目对话，coordinator 的回复和折叠后的线程汇报发回 chat；前端：侧栏 IM 入口、项目页头标签、绑定弹窗、对话里的来源图标 | working folder 绑定、IM 里的审批/提问（只发一行“有线程等你，请到 Web 处理”）、点名线程、下线旧路径 | 已实施（#2147） |
| **统一模型** | 见 [projects-sessions.md](./projects-sessions.md) 的交付：一次变更，后端和前端两个叠放的 PR | IM 路由、前端新功能 | 已实施（#2151、#2154）；绑定文件夹里的线程、弹窗列项目和文件夹随之可用 |
| **M2b 文件夹绑定** | 绑定已是 `thread_id`；`/new`、`/stop`；已绑定 chat 的斜杠命令不再落到旧 `@cc` | 同 M1 | 已实施（见下“M2b 的落地”） |
| **M3 审批与提问进 IM** | 线程审批/提问发到所有绑定的 chat，按钮，先答为准，另一边改写为“已处理” | 没有按钮的平台用文字回答 | 已实施（见下“M3 的落地”） |
| **M4 点名线程** | `Thread.Seq`、`@N`、引用回复、`/threads`、`/focus` | 引用回复 | 已实施（见下“M4 的落地”），引用回复未做 |
| **M5 收尾** | ~~未绑定走 coordinator；删 SmartGuide、Remote 页面、`remote/session`；旧绑定转换~~ 改为：已绑定 chat 的 `/help` 只列 Projects 命令 | 下线旧路径（保留，见决定 3） | 已实施（见下“M5 的落地”） |
| **M6 coordinator 路由** | `@opus`/`@codex` 决定模型、profile、agent | | |

优先级：M1 > M2 > M3 > M4 > M5 > M6。M1 是验证“IM 与项目对话合一”的最小闭环；M2 让旧 `@cc` 用户有地方迁；
M3 之前，需要审批的线程在 IM 里只有提示，靠 Web 处理，这是 M1/M2 刻意接受的限制。

## M1 后端的落地

对应上面的提案，落地时的具体形状（与提案不同或提案没写到的地方）：

- **表与字段**：`channel_bindings`（`(platform, chat_id)` 唯一；`target_kind` 目前只有 `project`；`workdir_id`、`focus_thread_id` 可空，留给 M2/M4（统一模型折成 `thread_id`，`target_kind`、`workdir_id`、`focus_thread_id` 随之去掉））。`project_messages` 新增 `source`（`im`，空=Web）、`source_platform`、`source_chat_id`（统一模型后在事件上）。**归档**项目或移除文件夹会解绑它所有线程的 chat，chat 回到未绑定时的行为（今天是旧 `@cc`/`@tb` 路径），而不是每条消息都报“项目已归档”；取消归档不会重新绑定（chat 可能已改作他用，要用户显式再绑）。线程归档（resolve）暂不解绑：文件夹的 chat 由 M2b 的 `/new` 换到新线程，子线程的 chat 按 M4 退回父线程。
- **接口**（`/api/v1/projects` 下）：`GET /channel-bindings?project_id=`、`PUT /channel-bindings`（`platform`、`chat_id`、`project_id`，可选 `chat_title`；已绑定的 chat 再绑即**改绑**，同一行）、`DELETE /channel-bindings/:id`。归档的项目不能被绑定。
- **入站**：`remoteagent.ProjectRouter`（小接口，`BotHandler` 可选持有）在 `HandleMessage` 里、`@cc`/`@tb` 握手词检测**之前**取走**已绑定 chat 的非斜杠文本**；斜杠命令（`/help`、`/bind`、`/cd`…）、正在输入 `/cd` 路径的 chat、以及访问控制/配对/黑名单都照旧在它前面或旁边。没有绑定的 chat 不受任何影响。绑定了项目的 chat 发附件，回一句“暂不支持”，而不是悄悄送去 `@cc`。投递失败（如项目已暂停）会把原因回给 chat。
- **走 Web 同一条路径**：入站只是 `Service.PostFromChannel`（解析绑定的线程，再调 `Service.Send(thread, text, source)`，来源记为 `im`），所以排队、暂停/归档检查、事件推送都和 Web 一致。
- **出站**：Projects 的可选 Host 能力 `ChannelSink.SendChannelText(platform, chatID, text)`（类型断言，独立版没有，不发送）。`Relay` 在 `Coordinator.OnFinal`（主线程的助手回复、线程汇报、出错）、`Threads.OnReply`（非主线程每轮结束的文本或错误，只发给绑在该线程自己上的 chat）和 `Threads.OnWait`（线程要审批/提问）上挂钩，**按完成顺序逐条**发给项目绑定的**所有** chat，前缀是消息来自的地方：项目对话 `[项目名] `，其他线程 `[#N 标题] `（线程汇报的事件载荷带 `seq`）；线程汇报折成一行（`[#2 修登录] Finished: 首行`，最多 240 字）；审批/提问只发一行“`[#2 修登录] Needs you`，请到 Web 处理”，同一线程 5 分钟内只提醒一次。用户自己发的消息不回显。
- **Tingly Box 一侧**：`internal/server/module/projects.ChatBridge` 同时实现 `ProjectRouter` 和 `ChannelSink`，经 bot 的 `remote/channel.Channel.Send` 发送（与通知同一条路径，微信的 `context_token` 等照常）。哪个 bot 发：优先该 chat 最近一次入站所用的 bot；重启后还没收到过消息时，依次试该平台的 bot。被屏蔽（disabled）的 chat 不发。Projects 开关关闭时桥不接任何消息。
- **没做**：文件夹 chat 的 `/new`、`/stop`（M2b）、IM 里审批（M3）、`@N`、聚焦、媒体入站、`chat_title` 的自动获取（现在只有显式传入）。

## M2b 的落地

- **命令**：已绑定的 chat 里 `/stop`（及 `stop`、`/interrupt`）、`/clear`、`/new [第一条消息]` 交给 Projects
  （`ProjectRouter.RouteCommand` → `Service.ChannelCommand`），在旧的 stop 检测**之前**，所以不会再落到旧 `@cc` 会话上。
  其他斜杠命令（`/help`、`/bind`、`/cd`…）照旧；没绑定的 chat 完全不变。
- **文件夹 chat**：`/new 任务` 立刻在同一文件夹开新线程并把绑定移过去，模型、profile、权限模式沿用当前线程，第一条消息记为 `im` 来源；
  `/new` 不带文字只记下“下一条消息开新会话”（`channel_bindings.new_thread`，重绑时清除），不开空线程（文件夹线程不能没有第一条消息）。
  `/clear` = 停止当前回合 + 同上。旧线程保留在文件夹里。
- **结束的会话不拒收**：文件夹 chat 绑定的线程已 resolve（手动归档或空闲自动 resolve）时，下一条消息直接开新线程，
  而不是回“线程已结束，请先 reopen”。回复前缀 `[#N 标题]` 变了，用户就知道换了会话。
- **项目对话的 chat**：协调者不能被停止，也不由 chat 开线程；`/stop`、`/clear`、`/new` 回一句说明，不做任何事。
  子线程的 chat（只能经 API 绑）`/stop` 停它，`/new`、`/clear` 说明只对文件夹有效。
- **回复**：命令的结果（`Stopped [#2 …].`、`Started [#3 …] in billing.`）由 handler 直接回给 chat；之后线程的回复照常经 Relay。
- **没做**：`/help` 在已绑定的 chat 里还列旧命令（M5 已改）；`/stop #N` 点名停止属于 M4。

## M3 的落地

- **发给谁**：线程停下来要审批或提问时（`Threads.OnWait`，带请求事件），发给绑在该线程**及其祖先**上的所有 chat，
  与以前那句“需要你”同一批人；前缀 `[#N 标题]`。
- **按钮**：Host 可选能力 `ChannelPrompter`（`SendChannelPrompt` / `RestateChannelPrompt`）。审批是“允许 / 本线程都允许 / 拒绝”；
  提问只有**一个单选问题、1 到 8 个选项**时每个选项一个按钮（选项说明列在正文里）。多问题、多选、没有选项的提问，
  以及没有按钮的平台（今天：微信、企业微信、Slack、钉钉、Discord、WhatsApp、QQ）仍发一行“Needs you: …，请到 Web 处理”，同一线程同一 chat 5 分钟一次。
- **按下去**：按钮负载是 `pj, 线程 id, 请求 id, 选择`（Telegram 的 64 字节限制由平台的负载编码处理）。`remoteagent` 的回调分派把它交给
  `ProjectRouter.RouteAnswer` → `Service.AnswerFromChannel`：chat 必须绑在该线程或它的祖先上，否则拒绝（`ErrNotThisChat`）；
  然后就是 Web 用的同一个 `Threads.Respond`，答案带 `Via = im`，事件载荷里记 `via`。授权上，`pj` 与 `@cc` 的 `perm` 一样要求
  `remote_control.approve`。
- **先答为准**：请求结束（任一边回答、建议答案自动作答、回合被停掉）时 `Threads.OnSettled` 触发，Relay 把发出去的每条按钮消息**改写**为原文加结果
  （“Allowed in a chat.” / “Denied on the web.” / “Answered in a chat: SQLite” / “No longer waiting: the turn ended.”），按钮随之消失；
  不能改写的平台补发一条结果。晚到的按钮回 “This was already answered.”。两条路都走 Relay 的同一个有序队列，所以改写总在发送之后。
- **不用 `IMPrompter`**：它为 `@cc` 设计，找不到 bot 时自动批准、有全局“总是允许”白名单、超时自己作答，而且阻塞等待。Projects 的请求
  没有超时、只在线程里作答，所以按钮消息只是“发出去 + 之后改写”，回答走普通回调。
- **没做**：用文字回答（引用回复或编号）给没有按钮的平台；`/threads` 列表。

## M4 的落地

- **`@N 文字`**：任何已绑定 chat 的消息以 `@N`（或 `@#N`）开头时，这一条发给本项目（或本文件夹）的线程 #N，绑定不变；回复照常回到
  绑在该线程或其祖先上的 chat（绑在项目对话的 chat 收到协调者的汇报）。`@` 后不是数字的（`@opus`）照旧是普通消息，留给 M6。
  编号不存在时回 `no such thread in this project: #9`。
- **`/focus N`**：把 chat 改绑到线程 #N（就是改绑，见 projects-sessions.md），回复里说 `/back` 会回到哪；已结束的线程不能聚焦。
  **`/back`**：改绑到当前线程的父线程；在项目对话或文件夹线程上说明“上面没有了”。
- **线程结束退回父线程**：子线程 resolve（手动、合并、空闲自动）时，绑在它上面的 chat 移到父线程（`Threads.releaseChats`），
  不会继续往一个已结束的线程里发。文件夹线程没有父线程，沿用 M2b：下一条消息开新会话。
- **`/threads`**：列出未结束的线程，“需要你”和“工作中”在前，标出本 chat 所在的那个，最多 15 条，结尾提示 `@N`、`/focus N`。纯文本，没有按钮。
- **`/stop #N`**：停止指定线程（M2b 的 `/stop` 只停本 chat 所在的线程）。
- **优先级**：`@N` > 绑定（聚焦即绑定）。审批和提问按钮不受聚焦影响，按 M3 发给线程及其祖先的 chat。
- **没做**：引用回复。要知道被引用的消息来自哪个线程，Relay 得记下每条发出消息的平台 id；今天 `ChannelSink.SendChannelText` 不返回 id，
  而 `@N` 已经覆盖同样的需要，所以先不做。

## M5 的落地

- **不下线旧路径**（决定 3 的修改）：没绑定的 chat 一切照旧，Projects 只接管绑定了的 chat。
- **绑定只有一个入口**：Projects 的 IM 弹窗（Web）。曾做过 chat 里的 `/connect`（列项目和文件夹、点选绑定），
  YZ 否掉了：同一件事在两个地方都能配会让用户更乱，而 Remote 的用量还不清楚。等确有需要再加。
- **`/help`**：已绑定的 chat 里只列 Projects 的命令（`@N`、`/threads`、`/focus`、`/back`、`/new`、`/stop`、`/clear`）
  和 Your ID / Chat ID，并说明绑定在 Web 上改；没绑定的 chat 仍是旧帮助（含 `@cc` / `@tb`）。

## 待定

- **旧数据**：旧路径保留后不再需要转换；`@cc` 的 `ProjectPath` 和 `remote/session` 照旧归 Remote。
- **群聊**（TEP-487）：一个群一条绑定，谁能操作沿用现在的白名单；群里的审批由谁回答没有定。
- **独立版 Projects 是否带 IM**：现在倾向不带，只留表和接口。
- **离线 chat 的出站**：chat 失去 `ContextToken`（微信）时，线程的通知如何降级。
