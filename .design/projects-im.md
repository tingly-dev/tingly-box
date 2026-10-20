# IM 聊天并入 Projects

> 状态：提案（TEP #2143）。承接 [projects-desk-absorption.md](./projects-desk-absorption.md)：Desk 已并入，
> 这里把 Remote 的 IM 聊天（`@cc`）也并进来，并**重新打开**那篇 Decisions §1“`@cc` 不共享、不导入”。
> 本文里的 schema、API 和阶段都是提案，不是现状。

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

## 阶段（每阶段一个或几个 PR；前两阶段可并行）

1. **数据模型**：`ChannelBinding`、来源字段、Projects 路由（list/create/delete binding）、`Host` 的可选 IM 能力。
   不动 UI 和 Remote。
2. **前端骨架**（与 1 并行）：Projects 侧栏“IM”入口、页头标签、绑定管理弹窗，先接占位 API 和 mock。
3. **入站路由**：`remoteagent` 按 binding 路由到线程/项目对话；`@cc` 执行器换成线程调用；审批双端同步。
4. **未绑定走 coordinator**，删 SmartGuide。
5. **退役**：删 Remote 页面和 `remote/session` 存储，迁移旧绑定（见待定），更新 `.design`。
6. **coordinator 的 `@xxx` 路由**（模型/profile/agent）。

## 待定

- **旧数据**：已有 chat 的 `ProjectPath` 可一次性转成 workdir 绑定（路径即 working folder）；
  旧 `remote/session` 历史是导入还是丢弃。倾向：绑定转换，历史不导入（Desk 切换时同样没有导入）。
- **群聊**（TEP-487）：一个群一条绑定，谁能操作沿用现在的白名单；群里的审批由谁回答没有定。
- **独立版 Projects 是否带 IM**：现在倾向不带，只留表和接口。
- **离线 chat 的出站**：chat 失去 `ContextToken`（微信）时，线程的通知如何降级。
