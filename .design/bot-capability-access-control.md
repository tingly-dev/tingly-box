# Bot Capability Access Control — Bot、Direct Chat、Group 与 Actor

> Status: **spec** · Date: 2026-08-01
>
> Scope: Bot 能力、个人会话、群组与群内参与者的最终领域模型、授权模型、
> 持久化结构、运行链路、控制面 API 与前端信息架构。
>
> This is a **target-state design**. It deliberately does not describe legacy
> compatibility, staged migration, dual-write, or transitional schemas.

本文延续 `.design/bot-arch.md` 的「Bot 是连接资源」结论，但将其中面向产品的
`Consumer` 正式收敛为 **Capability**，并重新定义会话访问控制。

本文最终模型：

```text
resource axis:    Bot ──┬── DirectChat
                       └── Group ── GroupActor

capability axis:  Notify | Remote Control

transport axis:   Telegram | Lark | Discord | ...
```

三个轴正交，不再把能力、会话资源和平台传输混在一棵树里。

---

## 1. Motivation

现有 Bot 子系统已经可以：

- 启停一个 IM Bot；
- 挂载 `remote_agent` 与 `notify` 两种用途；
- 发现并列出 IM chat；
- 配对私聊、白名单群聊；
- 禁用或删除一个 chat；
- 通过共享 Channel 发送通知或交互问题。

但控制模型仍然是历史能力逐项叠加出来的：

1. `db.Chat` 同时表示个人会话和群聊，私聊配对字段与群聊白名单字段混在同一实体。
2. `ChatID` 是全局主键，资源归属没有完整表达 Bot 与平台维度。
3. `Disabled` 同时阻止入站控制与出站通知，无法表达「Notify only」。
4. 群聊只能整体白名单，不能表达群内谁能控制、谁只能回答、谁可以执行高权限动作。
5. `remote_agent` 的 mount row 与 Notify route 共存在 `Scenarios` JSON 中，能力开关和
   消息路由是两种不同事物，却共享一个字段和一套词汇。
6. Prompt reply 由 Bot host 在 Capability handler 之前认领；若 pending request 没有携带
   Capability、目标资源和 Actor 授权上下文，回复路径可能绕过普通消息路径的门禁。
7. “Channel”在代码中表示传输表面，在产品讨论中又容易被理解为 Remote Control /
   Notify 功能通道，形成命名碰撞。

目标不是增加更多布尔开关，而是建立一个能稳定回答以下问题的模型：

> 哪个 Bot 的哪项 Capability，允许在哪个个人会话或群组中，由谁执行哪个动作？

---

## 2. Decision summary

### 2.1 Final resource hierarchy

```text
Bot
├── Capabilities
│   ├── Notify
│   └── Remote Control
│
├── Direct Chats
│   └── one peer Actor per DirectChat
│
└── Groups
    └── Actor Bindings
```

- **Bot** 决定连接是否可以运行，以及提供哪些 Capability。
- **DirectChat** 表示 Bot 与一个人的一对一入口；peer Actor 是其固有身份，不另建成员列表。
- **Group** 是与 DirectChat 并列的独立资源，不再伪装成一种 Chat。
- **GroupActor** 表示某个 Actor 在某个 Group 中拥有哪些动作权限。
- **Capability** 是贯穿 Bot、DirectChat、Group 与 Actor 的横向决策维度，不是资源父子节点。
- **Transport** 表示真实平台连接与技术能力；它不是用户配置的 Capability。
- **Route** 表示外部事件如何通过 Notify 到达 DirectChat 或 Group；它不是 Capability 开关。

### 2.2 Final authorization rule

```text
DirectChat decision:
  Bot.Enabled
  ∩ Bot.Capability.Enabled
  ∩ Transport.ReadyAndSupports
  ∩ DirectChat.NotBlocked
  ∩ DirectChat.CapabilityAccess
  ∩ DirectChat.ActionPermission

Group decision:
  Bot.Enabled
  ∩ Bot.Capability.Enabled
  ∩ Transport.ReadyAndSupports
  ∩ Group.NotBlocked
  ∩ Group.CapabilityAccess
  ∩ GroupActor.ActionPermission       // only for actor-driven actions
```

任何一层拒绝，最终拒绝。下层不能重新打开上层已经关闭的能力。

### 2.3 Runtime rule

```text
shouldRun(bot) = bot.enabled && any(bot.capabilities.enabled)
```

`Bot.Enabled` 是保留 Capability 配置的总闸；所有 Capability 都关闭时，Bot 因为没有用途
而自然停止。重新打开总闸时，原 Capability 配置恢复生效。

---

## 3. Goals and non-goals

### Goals

- G1. 以 Bot、DirectChat、Group、Actor 四类明确资源表达控制边界。
- G2. 以 Capability × Action 表达细粒度权限，不用一个 `Disabled` 承担所有语义。
- G3. 私聊保持简单：一个 DirectChat 对应一个 peer Actor，不暴露成员管理 UI。
- G4. 群聊显式管理 Actor 权限，默认拒绝未授权参与者。
- G5. 所有真实入站路径都经过同一个授权器，包括文本、命令、按钮、文件和 Prompt reply。
- G6. 所有真实出站路径也经过授权器，包括 Route 通知与通用 Bot interaction API。
- G7. 每次拒绝都产生稳定 reason code，能回答「为什么 Bot 没响应」。
- G8. 能力、路由、传输与平台能力各有一个词、一个层级和一个事实来源。
- G9. API 与 UI 直接呈现 Direct Chats 和 Groups，不要求用户先选择会话模式。

### Non-goals

- NG1. 控制面管理员、Web UI 用户、API service account 的组织级 RBAC。
- NG2. 从旧 `remote_chats` / `Scenarios` JSON 迁移到新 schema 的步骤。
- NG3. 跨平台合并同一个自然人的身份。
- NG4. 从 Telegram/Lark/Discord 实时同步完整群成员目录。
- NG5. 用户自定义角色系统或通用策略语言。
- NG6. 把平台自身的管理员角色自动等同为 Tingly-Box 高权限 Actor。

---

## 4. Vocabulary

| Term | Definition | Must not mean |
|---|---|---|
| **Bot** | 一个带平台凭据、受监督运行的连接资源 | 某项业务能力 |
| **Capability** | Bot 提供的产品用途；首批为 Notify、Remote Control | Telegram/Lark 连接 |
| **Transport** | Bot 运行时的真实平台连接和 Send/Prompt 表面 | Notify / Remote Control |
| **DirectChat** | Bot 与一个 peer Actor 的一对一会话资源 | 群聊、所有平台 conversation 的泛称 |
| **Group** | 多 Actor 参与的群组资源 | DirectChat 的一个布尔变体 |
| **Actor** | 由平台认证并随入站事件提供的参与者身份 | ChatID 或 GroupID |
| **GroupActor** | Actor 在特定 Group 中的授权绑定 | Actor 的全局身份 |
| **Route** | 外部事件到 Notify 目标的路由规则 | Capability mount、TOFU `/bind` |
| **Action** | Capability 内可单独授权的操作 | Capability 本身 |

目标代码词汇：

```text
Consumer                  → Capability
Consumer.Mounted          → Capability.Enabled / configured state
remote_agent              → remote_control
remote/channel.Channel    → Transport surface（目标命名，不再承担产品 Channel 含义）
Scenarios mount row       → BotCapability row
Scenarios route row       → Route row
```

用户可见名称统一为 **Remote Control**、**Notify**、**Chats**、**Groups**。

---

## 5. Domain model

### 5.1 Aggregate overview

```text
┌──────────────────────────────────────────────────────────────┐
│ Bot                                                          │
│ uuid · name · platform · credentials · enabled               │
│                                                              │
│ Capabilities                                                 │
│   notify(enabled, config)                                    │
│   remote_control(enabled, config)                            │
│                                                              │
│ Runtime                                                      │
│   Transport(status, platform capabilities)                   │
└───────────────┬──────────────────────────────┬───────────────┘
                │ owns                         │ owns
                ▼                              ▼
┌──────────────────────────────┐  ┌──────────────────────────────┐
│ DirectChat                   │  │ Group                        │
│ id · external_chat_id        │  │ id · external_group_id       │
│ peer_actor_id                │  │ capability access            │
│ capability/action access     │  │ blocked                      │
│ pairing · blocked            │  └──────────────┬───────────────┘
└──────────────────────────────┘                 │
                                               │ has bindings
                                               ▼
                                ┌──────────────────────────────┐
                                │ GroupActor                   │
                                │ actor identity               │
                                │ capability/action permission │
                                └──────────────────────────────┘

Notify Route ──targets──► DirectChat | Group
```

### 5.2 Bot

Bot 是唯一拥有平台凭据和连接生命周期的资源。

```go
type Bot struct {
    UUID       string
    Name       string
    Platform   Platform
    Auth       AuthConfig
    Enabled    bool
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

Bot 不直接保存：

- DirectChat / Group 的访问权限 blob；
- Route 列表 JSON；
- Group Actor 白名单；
- Capability mount row。

这些都是可独立查询、更新和约束的资源。

### 5.3 Capability

Capability 是有限、类型化、由产品定义的集合：

```go
type CapabilityName string

const (
    CapabilityNotify        CapabilityName = "notify"
    CapabilityRemoteControl CapabilityName = "remote_control"
)

type BotCapability struct {
    BotUUID string
    Name    CapabilityName
    Enabled bool
    Config  json.RawMessage // validated by the named capability
}
```

Capability 负责：

- 判断 Bot 是否有运行理由；
- 注册自身的入站 handler、命令与运行依赖；
- 声明 Actions；
- 产生或消费带授权上下文的 Prompt。

Capability 不负责：

- 创建平台连接；
- 解释其它 Capability 的配置；
- 跳过统一授权器直接处理消息；
- 把 Route 当作自己的 enabled 状态。

### 5.4 DirectChat

DirectChat 是 Bot 与一个 peer Actor 的一对一入口。

```go
type DirectChat struct {
    ID             string // internal stable UUID
    BotUUID        string
    Platform       Platform
    ExternalChatID string // concrete platform value
    PeerActorID    string // internal Actor ID
    Blocked        bool
    PairedAt       *time.Time
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

身份唯一性：

```text
UNIQUE(bot_uuid, platform, external_chat_id)
```

DirectChat 的 Actor 是一对一固有关系，因此不创建 `DirectChatActorBinding`。对 Actor 驱动
的动作，授权器必须确认入站 Actor 与 `PeerActorID` 一致。

Pairing 是建立 `PeerActorID` 信任关系的 bootstrap 流程，不是 Capability。

### 5.5 Group

Group 是 Bot 可到达的多人会话资源：

```go
type Group struct {
    ID              string // internal stable UUID
    BotUUID         string
    Platform        Platform
    ExternalGroupID string // concrete platform value
    Name            string
    Blocked         bool
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

身份唯一性：

```text
UNIQUE(bot_uuid, platform, external_group_id)
```

Group 不存一个整体 `IsWhitelisted`。是否允许某项 Capability 由 Group Capability Access
表达；谁能参与由 GroupActor Permission 表达。

### 5.6 Actor and GroupActor

Actor 是平台在入站事件中认证出的身份。它在 Bot 范围内稳定，不尝试跨 Bot 或跨平台合并：

```go
type Actor struct {
    ID              string
    BotUUID         string
    Platform        Platform
    ExternalActorID string
    DisplayName     string
    LastSeenAt      time.Time
}
```

```text
UNIQUE(bot_uuid, platform, external_actor_id)
```

GroupActor 表示授权关系：

```go
type GroupActor struct {
    GroupID  string
    ActorID  string
    Label    string // optional UI label/preset, not the authorization source
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

同一 Actor 可以属于多个 Group；每个 Group 中的权限相互独立。

平台管理员身份不自动产生 Tingly-Box 权限。平台 adapter 可以提供已验证的平台角色作为
诊断上下文，但授权必须来自显式 GroupActor permission。

### 5.7 Transport

Transport 是 Bot 运行时的技术表面：

```go
type Transport interface {
    ID() string              // bot UUID
    Platform() Platform
    Status() TransportStatus
    Supports(TransportAction) bool
    Send(...)
    Prompt(...)
}
```

Transport 状态：

```text
connecting | online | degraded | offline
```

Transport 支持项是事实，不是用户策略：Buttons、Files、Message Editing、Interactive
Prompt 等。用户不能通过 DirectChat 或 Group policy 强行打开平台不支持的能力。

---

## 6. Capability and action catalog

### 6.1 Notify

Notify 表示应用或 Route 向人发送信息。

| Action | Subject | Target | Meaning |
|---|---|---|---|
| `notify.receive` | authenticated caller / Route | DirectChat or Group | 目标可接收通知 |
| `notify.reply` | peer Actor / GroupActor | pending interaction | Actor 可回答交互通知 |

`notify.receive` 没有 Actor，因为它是出站动作；`notify.reply` 必须携带 Actor。

### 6.2 Remote Control

Remote Control 表示人通过 IM 控制 Tingly-Box / agent。

| Action | Subject | Target | Meaning |
|---|---|---|---|
| `remote_control.start` | Actor | DirectChat or Group | 发起命令或 agent handoff |
| `remote_control.approve` | Actor | pending approval | 回答 agent 权限请求 |
| `remote_control.privileged` | Actor | DirectChat or Group | Shell、文件、项目绑定等高风险动作 |

具体命令可以声明所需 action：

```text
/help, ordinary agent prompt  → remote_control.start
approval button              → remote_control.approve
/cd, file access, shell      → remote_control.start + remote_control.privileged
```

### 6.3 Why Interact is not a top-level Capability

Interaction 是 Transport 的 `Prompt/Reply` 原语，会被两个 Capability 使用：

```text
Notify         → confirmation / choose / ask
Remote Control → approval / ask-user-question
```

若把 Interact 做成第三个总开关，关闭它会同时破坏 Notify 和 Remote Control，形成跨功能
副作用。因此它只作为 Capability 内的 action 和 Transport primitive 存在。

---

## 7. Access policy

### 7.1 Policy records are explicit

Capability access 不使用隐式的 absent-is-on / absent-is-off 非对称规则。每个资源创建时写入
完整、显式的初始 policy；读取缺失 policy 必须 fail closed 并返回诊断原因。

```go
type AccessEffect string

const (
    AccessAllow AccessEffect = "allow"
    AccessDeny  AccessEffect = "deny"
)
```

Bot 使用 `BotCapability.Enabled`；DirectChat、Group 与 GroupActor 使用显式 allow/deny。

### 7.2 Defaults

默认值由创建流程写入，不在读取时猜测：

| Resource creation | Notify | Remote Control | Actor actions |
|---|---|---|---|
| discovered DirectChat | deny | deny | none |
| paired DirectChat | unchanged | allow | peer gets start + approve; privileged explicit |
| discovered Group | deny | deny | none |
| operator attaches Notify target | allow | unchanged | none |
| operator enables Group Remote Control | unchanged | allow | still no Actor permission |
| GroupActor added as controller | unchanged | unchanged | start + approve |

关键安全性质：

- 仅开启 Group Remote Control 不会让所有群成员获得控制权。
- 仅添加 GroupActor permission 也不能绕过 Group 或 Bot Capability deny。
- 创建 Route 时，产品工作面可以同时显式授予目标 `notify.receive`，避免用户配置出一个
  永远不会投递的 Route；这两个写操作必须在同一事务完成并在 UI 中明确展示。

### 7.3 Blocked is a hard resource gate

`Blocked` 与 Capability policy 分离：

- Block 是可逆、最高优先级的全部拒绝；
- Capability deny 是某项用途的细粒度关闭；
- Delete 是删除资源及依赖关系，不是授权状态；
- Unpair 是解除 DirectChat 的 Actor 信任关系，不等于删除历史。

```text
Bot disabled       → deny all
Target blocked     → deny all
Capability disabled→ deny that capability
Action denied      → deny that action
```

### 7.4 Fixed permissions, no user-defined roles

授权事实存为 Capability + Action permission。UI 可以提供 Controller、Responder 等预设，
但预设只是一次性填充权限的便捷方式，不成为运行时必须解析的自定义角色层。

这避免：

- 角色组合爆炸；
- 修改角色定义后历史授权静默变化；
- 为两个 Capability 引入通用策略语言。

### 7.5 Decision request and result

```go
type AuthorizationRequest struct {
    BotUUID   string
    Target    TargetRef       // DirectChat or Group
    ActorID   string          // empty only for actorless actions
    Capability CapabilityName
    Action     ActionName
    RouteID    string          // present for route-driven notify
    RequestID  string          // present for pending interactions
}

type AuthorizationDecision struct {
    Allowed    bool
    Reason     DecisionReason
    FailedGate GateName
    Facts      DecisionFacts
}
```

稳定 reason codes：

```text
bot_disabled
capability_disabled
transport_offline
transport_unsupported
target_not_found
target_blocked
target_capability_denied
actor_required
actor_mismatch
actor_not_registered
actor_action_denied
route_inactive
route_target_mismatch
pending_request_expired
pending_request_actor_denied
```

外部 API 可以按安全需要把多个内部原因映射成同一个 404/403 body，但日志和管理 UI 必须
保留精确 reason。

---

## 8. Authorization flows

### 8.1 DirectChat remote control

```text
platform event
  → adapter authenticates bot / direct chat / actor IDs
  → resolve DirectChat by (bot, platform, external_chat_id)
  → actor must equal DirectChat.PeerActorID
  → authorize(remote_control.start)
  → command / agent dispatch
```

未配对 DirectChat 只进入 host-owned pairing bootstrap；不能进入 Capability handler。

### 8.2 Group remote control

```text
platform event
  → adapter authenticates group + actor IDs
  → resolve Group
  → authorize Group remote_control access
  → resolve GroupActor
  → authorize remote_control.start / privileged
  → command / agent dispatch
```

未登记 Actor 默认拒绝。不存在“Group whitelisted → 全体成员自动允许”。

### 8.3 Route notification

```text
authenticated scenario event
  → resolve active Route
  → resolve DirectChat | Group target
  → authorize(notify.receive, actorless)
  → resolve live Transport
  → Send
```

Route active 与 Target Notify access 是两个独立事实；决策器同时验证二者。

### 8.4 Interactive reply

共享 Prompt router 可以继续存在，但 pending request 必须携带完整授权上下文：

```go
type PendingInteractionAuthorization struct {
    BotUUID    string
    Target     TargetRef
    Capability CapabilityName
    ReplyAction ActionName // notify.reply or remote_control.approve
    InitiatorActorID string
}
```

回复链路：

```text
callback / text reply
  → normalize target + actor
  → find pending request
  → verify target exactly matches pending target
  → re-evaluate current Actor permission
  → resolve reply
```

权限在回复时重新计算，不使用 Prompt 创建时的旧快照。因此撤销 GroupActor permission、
Block target 或关闭 Capability 必须立即阻止尚未回答的 Prompt。

Host router 不能因为“识别出 request ID”就直接绕过授权器。未知或过期 request 可以被 host
认领并返回 expired，但不能触发任何 Capability side effect。

### 8.5 Callback, file and platform-specific paths

以下入口必须先由平台 adapter 归一化为同一个 authorization input：

- 普通文本；
- slash command；
- inline/card callback；
- reply keyboard；
- file/media；
- platform-specific action payload；
- pending Prompt 的文本回答。

平台特有 handler 不得自行解释 allowlist 或跳过 policy evaluator。

---

## 9. Runtime architecture

### 9.1 Capability interface

```go
type Capability interface {
    Name() CapabilityName
    Actions() []ActionName
    Attach(ctx context.Context, host HostServices) (AttachedCapability, error)
}
```

是否启用来自 `BotCapabilityStore`，不是 Capability 自己解析混合配置 blob：

```text
BotSupervisor
  → load enabled BotCapabilities
  → no enabled capability: do not start Transport
  → create Transport
  → attach each enabled Capability
  → dispatch all inbound events through Host authorization gate
```

### 9.2 Capability lifecycle

Capability 状态变化后的行为：

| Change | Required behavior |
|---|---|
| disabled → enabled | 若 Bot enabled，启动或 attach capability |
| enabled → disabled | 停止接收新动作，取消该 capability 的 pending requests，detach |
| last capability disabled | 关闭 Transport，Bot runtime 进入 stopped/no-capability |
| bot disabled | 取消全部 pending，detach capabilities，关闭 Transport |
| target blocked | 取消该 target 的 pending requests |
| actor permission revoked | 新动作立即拒绝；pending reply 时重新评估 |

### 9.3 One authorization seam

Host 提供统一入口：

```go
type Authorizer interface {
    Evaluate(context.Context, AuthorizationRequest) AuthorizationDecision
}
```

Capability handler 不直接读取 `Blocked`、pairing、GroupActor 表来拼自己的判断。领域 store
提供事实，Authorizer 负责唯一决策顺序，避免 Notify 与 Remote Control 漂移。

### 9.4 Real-path diagnostics

诊断必须调用与生产消息相同的 `Authorizer.Evaluate`，不复制判断逻辑：

```text
Can Alice start Remote Control in Engineering?

Bot enabled                         ✓
Remote Control enabled              ✓
Telegram transport online           ✓
Engineering not blocked             ✓
Engineering allows Remote Control   ✓
Alice remote_control.start          ✗

Decision: actor_action_denied
```

---

## 10. Final persistence schema

以下 schema 表达最终领域，不保留旧 `Scenarios` JSON 或 direct/group 混合字段。

### 10.1 Bot capabilities

```text
bot_capabilities
  bot_uuid       TEXT NOT NULL  FK imbot_settings(bot_uuid) ON DELETE CASCADE
  capability     TEXT NOT NULL
  enabled        BOOL NOT NULL
  config         JSON NOT NULL DEFAULT '{}'
  created_at     DATETIME NOT NULL
  updated_at     DATETIME NOT NULL
  PK(bot_uuid, capability)
```

只允许注册表中存在的 Capability 名称；Config 由对应 Capability 类型化验证。

### 10.2 Actors

```text
remote_actors
  id                 TEXT PK
  bot_uuid           TEXT NOT NULL FK imbot_settings(bot_uuid) ON DELETE CASCADE
  platform           TEXT NOT NULL
  external_actor_id  TEXT NOT NULL
  display_name       TEXT
  last_seen_at       DATETIME
  created_at         DATETIME NOT NULL
  updated_at         DATETIME NOT NULL
  UNIQUE(bot_uuid, platform, external_actor_id)
```

### 10.3 Direct chats

```text
remote_direct_chats
  id                 TEXT PK
  bot_uuid           TEXT NOT NULL FK imbot_settings(bot_uuid) ON DELETE CASCADE
  platform           TEXT NOT NULL
  external_chat_id   TEXT NOT NULL
  peer_actor_id      TEXT FK remote_actors(id)
  blocked            BOOL NOT NULL DEFAULT FALSE
  paired_at          DATETIME
  project_path       TEXT
  bash_cwd           TEXT
  current_agent      TEXT
  verbose            BOOL NULL
  created_at         DATETIME NOT NULL
  updated_at         DATETIME NOT NULL
  UNIQUE(bot_uuid, platform, external_chat_id)
```

```text
direct_chat_permissions
  direct_chat_id  TEXT NOT NULL FK remote_direct_chats(id) ON DELETE CASCADE
  capability      TEXT NOT NULL
  action          TEXT NOT NULL
  effect          TEXT NOT NULL CHECK(effect IN ('allow','deny'))
  updated_at      DATETIME NOT NULL
  PK(direct_chat_id, capability, action)
```

Capability 本身的目标准入使用 action `access`：

```text
(chat, notify, access, allow)
(chat, notify, receive, allow)
(chat, remote_control, access, allow)
(chat, remote_control, start, allow)
```

### 10.4 Groups and actors

```text
remote_groups
  id                 TEXT PK
  bot_uuid           TEXT NOT NULL FK imbot_settings(bot_uuid) ON DELETE CASCADE
  platform           TEXT NOT NULL
  external_group_id  TEXT NOT NULL
  name               TEXT
  blocked            BOOL NOT NULL DEFAULT FALSE
  project_path       TEXT
  bash_cwd           TEXT
  current_agent      TEXT
  verbose            BOOL NULL
  created_at         DATETIME NOT NULL
  updated_at         DATETIME NOT NULL
  UNIQUE(bot_uuid, platform, external_group_id)
```

```text
group_capability_access
  group_id       TEXT NOT NULL FK remote_groups(id) ON DELETE CASCADE
  capability     TEXT NOT NULL
  effect         TEXT NOT NULL CHECK(effect IN ('allow','deny'))
  updated_at     DATETIME NOT NULL
  PK(group_id, capability)
```

```text
remote_group_actors
  group_id       TEXT NOT NULL FK remote_groups(id) ON DELETE CASCADE
  actor_id       TEXT NOT NULL FK remote_actors(id) ON DELETE CASCADE
  label          TEXT
  created_at     DATETIME NOT NULL
  updated_at     DATETIME NOT NULL
  PK(group_id, actor_id)
```

```text
group_actor_permissions
  group_id       TEXT NOT NULL
  actor_id       TEXT NOT NULL
  capability     TEXT NOT NULL
  action         TEXT NOT NULL
  effect         TEXT NOT NULL CHECK(effect IN ('allow','deny'))
  updated_at     DATETIME NOT NULL
  PK(group_id, actor_id, capability, action)
  FK(group_id, actor_id)
    REFERENCES remote_group_actors(group_id, actor_id) ON DELETE CASCADE
```

### 10.5 Routes

```text
remote_routes
  id                 TEXT PK
  bot_uuid           TEXT NOT NULL FK imbot_settings(bot_uuid) ON DELETE CASCADE
  name               TEXT NOT NULL
  source              TEXT NOT NULL
  event_filter        JSON NOT NULL
  direct_chat_id      TEXT NULL FK remote_direct_chats(id)
  group_id            TEXT NULL FK remote_groups(id)
  enabled             BOOL NOT NULL
  options             JSON NOT NULL DEFAULT '{}'
  created_at          DATETIME NOT NULL
  updated_at          DATETIME NOT NULL
  UNIQUE(bot_uuid, name)
  CHECK(exactly one of direct_chat_id, group_id is non-null)
```

Route 必须属于与目标相同的 Bot。store 在事务中验证，不能依赖调用方。

### 10.6 Sessions and transcripts

Session 必须引用明确的 target：

```text
remote_sessions
  id                 TEXT PK
  bot_uuid           TEXT NOT NULL
  direct_chat_id      TEXT NULL
  group_id            TEXT NULL
  initiator_actor_id  TEXT NULL
  agent               TEXT
  project             TEXT
  status              TEXT
  ...
  CHECK(exactly one target is non-null)
```

Group Session 记录 initiator Actor，但 Session 权限不会冻结 Actor 权限；resume、reply 与
privileged action 仍需实时授权。

Transcript 继续作为无界追加产物保存；授权资源只保存可查询索引。

---

## 11. Control-plane API

所有 URL 使用内部稳定 UUID；响应同时展示平台真实 ID，满足诊断和复制需求。

### 11.1 Capabilities

```text
GET  /api/v1/bots/{bot}/capabilities
PUT  /api/v1/bots/{bot}/capabilities/{capability}
```

```json
{
  "enabled": true,
  "config": {}
}
```

更新最后一个 enabled Capability 为 false 后，响应必须返回派生运行结果：

```json
{
  "capability": "notify",
  "enabled": false,
  "bot_running": false,
  "reason": "no_enabled_capability"
}
```

### 11.2 Direct chats

```text
GET    /api/v1/bots/{bot}/chats
GET    /api/v1/bots/{bot}/chats/{chat}
PUT    /api/v1/bots/{bot}/chats/{chat}/blocked
PUT    /api/v1/bots/{bot}/chats/{chat}/permissions/{capability}/{action}
POST   /api/v1/bots/{bot}/chats/{chat}/unpair
DELETE /api/v1/bots/{bot}/chats/{chat}
POST   /api/v1/bots/{bot}/chats/{chat}/authorize-check
```

### 11.3 Groups

```text
GET    /api/v1/bots/{bot}/groups
GET    /api/v1/bots/{bot}/groups/{group}
PUT    /api/v1/bots/{bot}/groups/{group}/blocked
PUT    /api/v1/bots/{bot}/groups/{group}/capabilities/{capability}
DELETE /api/v1/bots/{bot}/groups/{group}
POST   /api/v1/bots/{bot}/groups/{group}/authorize-check
```

### 11.4 Group actors

```text
GET    /api/v1/bots/{bot}/groups/{group}/actors
PUT    /api/v1/bots/{bot}/groups/{group}/actors/{actor}
PUT    /api/v1/bots/{bot}/groups/{group}/actors/{actor}/permissions/{capability}/{action}
DELETE /api/v1/bots/{bot}/groups/{group}/actors/{actor}
```

Actor 列表表示 Tingly-Box 已观察或明确授权的 Actor，不承诺是平台完整成员目录。

### 11.5 Routes

```text
GET    /api/v1/bots/{bot}/routes
POST   /api/v1/bots/{bot}/routes
GET    /api/v1/bots/{bot}/routes/{route}
PUT    /api/v1/bots/{bot}/routes/{route}
DELETE /api/v1/bots/{bot}/routes/{route}
```

创建 Route 的 request 必须引用明确 target kind 与内部 ID：

```json
{
  "name": "claude-code-stop",
  "source": "claude_code",
  "events": ["Stop"],
  "target": {"kind": "group", "id": "grp_..."},
  "enabled": true
}
```

### 11.6 Delete behavior

Delete 默认不静默级联用户仍需处理的业务资源：

- 若 DirectChat / Group 仍被 Route 引用，返回 `409 target_has_routes`，并返回具体 Route。
- 若仍有 running/pending Session，返回 `409 target_has_active_sessions`。
- 用户在确认面显式选择处理依赖后，再先删除/关闭依赖并删除 target。
- 数据库 FK 仍作为最后一道完整性保护，而不是产品交互的主要机制。

---

## 12. Frontend information architecture

### 12.1 Bot detail is the work surface

用户进入 Bot 后直接看到真实状态、能力和可管理资源，不先经过模式选择：

```text
Telegram Operations
Connected · @operations_bot

Capabilities
  Remote Control    On
  Notify            On

Direct Chats
  Alice             Remote Control · Notify
  Bob               Notify only

Groups
  Engineering       Remote Control · Notify · 2 authorized actors
  Release Group     Notify only
```

Capability 是 Bot 页中的横向摘要，不建立一层需要先进入的“Capability 资源目录”。

### 12.2 DirectChat detail

围绕用户的问题组织：

1. **这个人是谁？** 平台名称、具体 external actor/chat ID、配对状态。
2. **他可以做什么？** Notify 与 Remote Control 的具体 action。
3. **为什么现在不能用？** Bot、Capability、Transport、Block、permission 的真实决策链。

不展示 Actors 子页，因为 DirectChat 的 peer Actor 是固有一对一关系。

### 12.3 Group detail

```text
Engineering

Capabilities
  Receive Notify       On
  Remote Control       On

Authorized Actors
  Alice                Start · Approve · Privileged
  Bob                  Reply only
  Other members        No access

Diagnostics
  Telegram online
  Last authorized action ...
```

Group 页面先展示 capability access，再展示谁能参与。不要把两条轴压进一个总状态词。

### 12.4 Presets without hiding concrete permissions

UI 可以提供快捷预设：

```text
DirectChat: Full access | Notify only | Blocked
GroupActor: Controller | Responder | Privileged controller
```

选择预设后，界面立即展示它实际写入的权限；用户可直接调整具体 action。预设不是模式，
也不是数据库中的间接 alias。

### 12.5 Empty states and next actions

- 无 DirectChat：展示“向 Bot 发送消息并完成配对”的完整入口或命令。
- 无 Group：展示如何把 Bot 加入 Group，以及如何从已配对 DirectChat 授权 Group。
- Group Remote Control 开启但无 Actor：明确显示“尚无人可以控制”，提供 Add Actor。
- Route 命中但 target deny Notify：显示具体阻断值并提供“Allow Notify”动作。
- Bot 无 enabled Capability：显示 stopped 原因并把 Capability 开关放在同一视野。

### 12.6 UX-principle check

- **按用户问题组织**：身份、可以做什么、为什么不能用。
- **消除模式选择**：直接进入 Bot、DirectChat 或 Group 工作面。
- **拆分命名碰撞**：Capability / Transport / Route 各指一件事。
- **正交轴分离**：资源轴与 Capability 轴分开呈现。
- **展示具体值**：external chat/group/actor ID 可见，友好名称只作辅助。
- **智能默认**：所有新资源 fail closed；配对或选择 Route target 时写入明确安全默认。
- **真实链路诊断**：调用生产 Authorizer。
- **保留可逆性**：Block、Capability deny、Unpair、Delete 分离。
- **提供下一步物件**：空状态给完整命令或直接动作。
- **副作用限于当前表面**：Group Actor 修改不改变同一 Actor 在其它 Group 的权限。

---

## 13. Security properties

1. **Default deny.** 缺失 resource、policy、Actor binding 或 action permission 均拒绝。
2. **Authenticated identity.** Actor ID、Group ID、DirectChat ID 必须来自平台 adapter 的
   认证事件，不接受用户消息 payload 自报身份。
3. **Bot-scoped identity.** 所有外部 ID 都带 Bot 与 Platform 维度，防止跨 Bot 归属泄漏。
4. **No group-wide implicit trust.** Group capability allow 不代表全体 Actor allow。
5. **No admin auto-trust.** 平台管理员身份不自动获得 Tingly-Box privileged 权限。
6. **Reply reauthorization.** Pending Prompt 回复按当前 policy 重新计算。
7. **No alternate path.** Callback、文本 reply、文件和平台专属 action 共用 Authorizer。
8. **Immediate revocation.** Block、Capability disable、Actor revoke 对下一次事件立即生效。
9. **Opaque external errors.** 未授权外部调用不得通过错误细节枚举 Bot、target 或 Actor；
   详细原因只进入认证后的管理 UI 与结构化日志。
10. **Structured audit.** 记录 bot、capability、action、target kind/id、actor id、decision、
    reason、request id；不记录消息正文和 secret。

---

## 14. Observability and diagnostics

统一结构化事件：

```text
bot.authorization.decision
  bot_uuid
  capability
  action
  target_kind
  target_id
  actor_id
  allowed
  failed_gate
  reason
  request_id
```

运行状态与授权状态分开：

```text
Operational failure: transport_offline, transport_unsupported
Policy denial:       target_blocked, actor_action_denied
Configuration issue: capability_disabled, route_inactive
Identity issue:      actor_mismatch, actor_not_registered
```

管理 UI 的“Can this work?”诊断 endpoint 调用真实 Authorizer，但不执行 action。

---

## 15. Error semantics

| Situation | Internal reason | Control-plane API |
|---|---|---|
| Bot/target UUID 不存在 | `target_not_found` | 404 |
| Target blocked | `target_blocked` | 409 for management action; delivery API maps to 404/403 policy |
| Capability disabled | `capability_disabled` | 409 with next action |
| Actor missing/denied | `actor_required` / `actor_action_denied` | 403 |
| Transport offline | `transport_offline` | 409 or 503 depending on endpoint |
| Platform unsupported | `transport_unsupported` | 422 |
| Delete has dependencies | `target_has_routes` | 409 + concrete dependencies |
| Pending interaction expired | `pending_request_expired` | 404/410 per interaction contract |

消息入站被拒绝时，是否向群里回复取决于拒绝类型：

- 未登记 Actor 的普通消息默认静默，避免群噪声；
- 显式调用 Bot 命令可返回简短拒绝与下一步；
- 已登记 Actor 权限不足时返回具体但不泄密的说明；
- 重复/高频拒绝按 target + actor 限流。

---

## 16. Testing strategy

### 16.1 Decision table tests

至少覆盖：

```text
Bot enabled        2
Capability enabled 2
Target kind        DirectChat | Group
Target blocked     2
Target access      allow | deny | missing
Actor              peer | bound | unbound | mismatched | absent
Action              receive | reply | start | approve | privileged
Transport           online | offline | unsupported
```

测试应以表驱动方式生成关键笛卡尔组合，并断言 `Allowed + Reason + FailedGate`。

### 16.2 DirectChat tests

- 同一个 external chat ID 在不同 Bot 下是两个资源。
- 入站 Actor 必须匹配 peer Actor。
- 未配对只允许 pairing bootstrap。
- Notify only 不允许 Remote Control。
- Block 覆盖所有 action，解除 Block 后原 policy 恢复。
- Unpair 清除 peer trust 与 Remote Control permissions，但保留资源和非敏感历史。

### 16.3 Group tests

- Group allow + 无 GroupActor → Remote Control deny。
- 同一 Actor 在 Group A allow、Group B deny，互不影响。
- Controller 不能自动获得 privileged。
- 删除 GroupActor 立即阻止 pending approval reply。
- 平台管理员但无本地 binding → deny。
- Group Notify delivery 不要求 Actor；Notify reply 要求 Actor。

### 16.4 Prompt routing tests

- Notify Prompt reply 使用 `notify.reply`。
- Remote approval reply 使用 `remote_control.approve`。
- reply target 与 pending target 不一致时拒绝。
- Capability 在 Prompt 发出后关闭，reply 被拒且 pending 被取消。
- Actor 在 Prompt 发出后撤权，reply 被拒。
- callback、文本 reply 两条路径得到相同 decision。
- unknown request ID 被 host 认领为 expired，但不进入 Capability handler。

### 16.5 Lifecycle tests

- Bot enabled + 无 Capability → 不启动。
- 开启第一个 Capability → 启动 Transport 并 attach。
- 关闭最后一个 Capability → cancel pending、detach、停止 Transport。
- Block target 取消该 target 的 pending，不影响其它 target。
- Disable Notify 不停止仍启用 Remote Control 的 Bot。

### 16.6 API and schema tests

- 所有 create/update models 都有 Swagger 定义。
- API 路径使用内部 UUID，响应展示 external ID。
- FK、唯一约束与 exactly-one-target CHECK 生效。
- Route 不可引用另一 Bot 的 target。
- Delete 有依赖时返回 409 与具体依赖。
- DirectChat endpoint 不返回 Group，Group endpoint 不返回 DirectChat。

### 16.7 Platform contract tests

每个 adapter 必须提供并测试：

```text
ConversationKind: direct | group
ExternalConversationID
ExternalActorID
Verified platform context
```

无法可靠分类的事件不得猜成 DirectChat；进入拒绝/诊断路径。

---

## 17. Module ownership

目标包结构：

```text
remote/control/bot/
  supervisor.go                 Bot lifecycle + shouldRun
  capability.go                 Capability interface + registry
  authorization.go              Authorizer orchestration
  capability_notify.go

remote/control/remoteagent/
  capability.go                 Remote Control Capability implementation
  ...

remote/access/
  types.go                      resource refs, actions, decisions, reasons
  evaluator.go                  pure decision ordering

remote/transport/
  transport.go                  Send/Prompt surface + registry

internal/data/db/
  bot_capability_store.go
  direct_chat_store.go
  group_store.go
  actor_store.go
  permission_store.go
  route_store.go

internal/server/module/imbot/
  bot capability/direct chat/group/actor management APIs

internal/server/module/notify/
  authenticated delivery/interaction API; Route execution adapter
```

依赖方向：

```text
domain/access types
      ▲
      │ implemented by
data/db stores
      ▲ injected into
Bot Supervisor / Capabilities / API
```

领域类型不放在 `../internal/db`；存储实现依赖领域，领域不依赖 GORM。

---

## 18. Prior art and alternatives

### 18.1 Industry patterns

- **Microsoft Teams** 明确区分 `personal`、`groupChat` 与 `team` Bot scopes，并通过
  Resource-Specific Consent 将能力授予具体 Chat、Team 或 User。它直接支持本文的
  「DirectChat / Group 并列资源 + resource-scoped capability」方向：
  [Bot scopes](https://learn.microsoft.com/en-us/microsoftteams/platform/bots/bot-basics)、
  [Resource-Specific Consent](https://learn.microsoft.com/en-us/microsoftteams/platform/graph-api/rsc/resource-specific-consent)。
- **Discord** 的 Application Command permissions 可以按 Guild 内的 Role、User 与 Channel
  覆盖，证明「Group access + Actor/Role action permission」是成熟产品模型：
  [Application Commands](https://docs.discord.com/developers/interactions/application-commands)。
- **Slack** 用统一 Conversations API 操作 public/private channel、DM 与 group DM，同时保留
  `im/mpim/public/private` 类型。本文借鉴的是“公共基础设施统一、产品资源明确分型”，而不是
  在 UI 中把所有对象重新混成 Conversation：
  [Conversations API](https://docs.slack.dev/apis/web-api/using-the-conversations-api/)。
- **Telegram** 对 private chat 与 group 的消息可见性和 Privacy Mode 采用不同语义，且明确
  要求 Bot 后端自行验证命令与 Actor 授权；平台命令 scope 不是授权器：
  [Bot Features](https://core.telegram.org/bots/features)。
- **Matrix** 使用统一 Room + per-user power level + per-action threshold。它验证了 Actor/action
  决策的可行性，但统一 Room 不符合本文已经决定的 DirectChat / Group 产品信息架构：
  [Matrix power levels](https://spec.matrix.org/latest/client-server-api/#mroompower_levels)。

### 18.2 Alternatives rejected

**A. One generic Conversation resource**

技术上最少表和接口，但会让 pairing、Group Actor、群组授权继续通过 nullable 字段和
`kind` 分支挤在同一个聚合中。公共 Transport、Session、Prompt 基础设施保持统一即可，
不需要牺牲产品资源边界。

**B. Capability as a parent node between Bot and targets**

```text
Bot → Capability → Chat/Group
```

同一个 DirectChat / Group 会同时出现在 Notify 与 Remote Control 两棵子树，身份、状态和
历史产生重复。Capability 必须是横向授权轴，而不是资源父节点。

**C. Pure RBAC**

RBAC 能表达 Group Actor 角色，但不能自然表达 Bot enabled、Transport offline、Target
blocked、Route inactive 等运行与资源事实。本文用类型化 resource policy 作为主模型，UI
预设可以保留角色体验，但运行时直接计算 Capability + Action permission。

**D. A third Interact Capability**

Prompt/Reply 被 Notify 与 Remote Control 共同使用。独立开关会造成关闭一项能力却破坏两条
业务链路，因此 Interaction 保持 Transport primitive 与 Capability action。

**E. Generic external authorization engine**

Casbin domain RBAC 或 OpenFGA 都能表达 scoped relation，但本文只有固定资源类型、两个
Capability 和有限 Actions。引入通用策略语言或独立授权服务会增加部署、缓存一致性与诊断
成本；目标实现使用本地类型化 Authorizer。只有出现用户自定义角色、跨 Bot 组织继承等已被
Non-goals 排除的需求时才重新评估。

### 18.3 Open questions

本设计没有阻塞实现的领域问题。Actor preset 的用户文案、各平台能够提供的 display name、
以及 Capability config 的具体字段可在对应功能规格中确定，但不能改变本文的资源关系、
授权顺序或 fail-closed 语义。

---

## 19. Superseded decisions

本文批准后，以下既有设计中的对应部分由本文取代：

- `.design/bot-arch.md`
  - `Consumer` 产品命名；
  - mount row 与 route row 共存于 `Scenarios`；
  - Chat 作为 direct/group 泛型产品资源的表达。
- `.design/remote-storage.md`
  - `remote_chats` 单表同时保存 pairing 与 whitelist；
  - `chat_id` 全局主键；
  - `IsWhitelisted` 群组总权限。
- `.design/bot-interaction-api.md`
  - `/bots/{bot}/chats` 同时发现 direct/group；
  - interaction target 仅靠原始 chat ID；
  - Chat `Disabled` 作为所有用途的唯一策略。

仍然保留的既有结论：

- Bot 是连接资源；
- Transport 是 host-owned runtime infrastructure；
- Notify 与 Remote Control 共用一个 Prompt transport；
- 平台差异在 adapter / transport capability table 中归一化；
- 大 transcript 留在追加文件，小而可查询的授权事实进入 SQLite；
- 诊断走真实生产链路。

---

## 20. Acceptance criteria

设计落地完成的判据：

1. 产品与 API 中 DirectChat、Group 是并列资源，没有 direct/group 混合 Chat DTO。
2. Bot Capability 有独立表和显式 enabled 状态，不再从 Route 是否存在推导。
3. Notify Route 有独立表，不再与 Capability mount 共用 `Scenarios` JSON。
4. 所有资源身份包含 Bot + Platform 归属，不存在全局裸 `chat_id` 主键。
5. Group Remote Control 开启后，未授权 Actor 仍然无法控制。
6. DirectChat peer、GroupActor、Prompt reply 都走统一 Authorizer。
7. Notify only、Remote Control only、两者均开、两者均关都能由明确 policy 表达。
8. 关闭最后一个 Capability 会停止 Bot，但不会删除 Capability 配置。
9. Block、Capability deny、Unpair、Delete 是四个不同动作和状态。
10. 每次拒绝都有稳定 reason code，管理 UI 能展示真实失败门。
11. 任何 callback/file/platform-specific path 都没有授权旁路。
12. API models 先在后端定义并进入 Swagger；前端 SDK 由 codegen 生成。
