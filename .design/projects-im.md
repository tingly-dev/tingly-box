# IM 聊天并入 Projects

> 状态：提案（TEP #2143）。承接 [projects-desk-absorption.md](./projects-desk-absorption.md)：Desk 已并入，
> 这里把 Remote 的 IM 聊天（`@cc`）也并进来，并**重新打开**那篇 Decisions §1“`@cc` 不共享、不导入”。
> 本文里的 schema、API 和里程碑都是提案，不是现状。图见 [projects-im.pencil.md](./projects-im.pencil.md)。

## 决定

1. **Projects 是唯一数据源**。IM 里的一次对话就是 Projects 里的线程（或项目对话的消息），只多几个来源字段；
   不再有第二个会话存储。
2. **IM chat 绑定一个落点**：Project（对话对象是 coordinator）或 working folder（对话对象是那个文件夹里的会话，
   即今天的 `@cc`）。绑定属于 chat，不属于会话；换绑要显式，旧线程保留。
3. **取消 `@tb` / SmartGuide**。没绑定落点的 chat，说话的对象就是 coordinator，由它帮用户选落点。
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

## 数据模型（Projects 一侧）

```
ChannelBinding            -- 一个 IM chat 当前绑在哪
  id, platform, chat_id          唯一键 (platform, chat_id)
  target_kind                    project | workdir
  project_id / workdir_id        二选一
  thread_id                      workdir 绑定时的“当前会话”；project 绑定为空
  focus_thread_id                project 绑定时可选的“聚焦线程”；空 = 跟 coordinator 说话
  created_at, updated_at

Thread / project_messages        新增来源字段（可空，空 = Web）
  source             im | web
  source_platform, source_chat_id
```

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
              └ workdir：Send 到 thread_id（没有则 Start 新线程）
Projects 事件 ──▶ 订阅了该落点的 binding ──▶ 渲染到 chat
```

- 回复前缀：已绑定时每条回复带落点短名（例：`[tingly-box] …`），避免用户忘了自己在哪。
- 一个 chat 同一时刻只绑一个落点；一个落点可以被多个 chat 绑定（手机和群各一个），出站按 binding 广播。

## 在 IM 里点名线程

project 绑定的 chat 默认在跟 coordinator 说话；要对某个线程说话，需要能“点名”。点名是一次 `@`：

- **线程编号**：每个线程在项目内有稳定的短编号 `#1`、`#2`（新增 `Thread.Seq`，项目内自增，不复用）。线程的通知、
  汇报、审批、提问消息都带 `[#2 标题]` 前缀。
- **主路径 `@2 补上边界用例`**：行首 `@` 加编号，整条消息去给线程 2，不改变 chat 的状态，每次显式，不会发错。
- **顺手路径：引用回复**线程的某条消息，消息去给那个线程。不需要记编号，但容易忘记，所以它只是快捷方式，不是唯一入口。
  平台不支持引用回复时，这条路径不存在，`@` 仍可用。
- **兜底 `/focus #2`**：之后的普通消息都进线程 2，回复前缀 `[#2 …]`；`/back` 或该线程结束后退回 coordinator。
  聚焦只存在绑定上，不改线程本身，用于长时间只盯一个线程的场景。
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
| **M1 只到 coordinator**（先做） | `ChannelBinding`（只支持 project）、消息来源字段、绑定的增删查接口；绑了项目的 chat 的消息进项目对话，coordinator 的回复和折叠后的线程汇报发回 chat；前端：侧栏 IM 入口、项目页头标签、绑定弹窗、对话里的来源图标 | working folder 绑定、IM 里的审批/提问（只发一行“有线程等你，请到 Web 处理”）、点名线程、下线旧路径 | 未开始 |
| **M2 working folder 绑定** | 绑定到文件夹，消息进当前会话（取代 `@cc`）；`/new`、`/stop`；目录浏览器改为新建 working folder | 同 M1 | |
| **M3 审批与提问进 IM** | 线程审批/提问发到所有绑定的 chat，按钮，先答为准，另一边改写为“已处理” | | |
| **M4 点名线程** | `Thread.Seq`、`@N`、引用回复、`/threads`、`/focus` | | |
| **M5 收尾** | 未绑定走 coordinator；删 SmartGuide、Remote 页面、`remote/session`；旧绑定转换 | | |
| **M6 coordinator 路由** | `@opus`/`@codex` 决定模型、profile、agent | | |

优先级：M1 > M2 > M3 > M4 > M5 > M6。M1 是验证“IM 与项目对话合一”的最小闭环；M2 让旧 `@cc` 用户有地方迁；
M3 之前，需要审批的线程在 IM 里只有提示，靠 Web 处理，这是 M1/M2 刻意接受的限制。

## 待定

- **旧数据**：已有 chat 的 `ProjectPath` 可一次性转成 workdir 绑定（路径即 working folder）；
  旧 `remote/session` 历史是导入还是丢弃。倾向：绑定转换，历史不导入（Desk 切换时同样没有导入）。
- **群聊**（TEP-487）：一个群一条绑定，谁能操作沿用现在的白名单；群里的审批由谁回答没有定。
- **独立版 Projects 是否带 IM**：现在倾向不带，只留表和接口。
- **离线 chat 的出站**：chat 失去 `ContextToken`（微信）时，线程的通知如何降级。
