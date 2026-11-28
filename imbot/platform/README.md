# imbot/platform

Each subdirectory implements `core.Bot` for one messaging platform. All platforms share the same lifecycle, event model, and message types from `imbot/core`; differences are encapsulated in per-package **Adapter** (inbound events) and **action renderers** (outbound `core.ActionSet`).

## Platforms

| Platform | README | Auth | Connection |
|---|---|---|---|
| Telegram | [telegram/](telegram/README.md) | token | Long-polling |
| Discord | [discord/](discord/README.md) | token | WebSocket (Gateway) |
| Slack | [slack/](slack/README.md) | token | RTM |
| Feishu | [feishu/](feishu/README.md) | oauth | WebSocket (Event Push) |
| Lark | [lark/](lark/README.md) | oauth | WebSocket (Event Push) |
| DingTalk | [dingtalk/](dingtalk/README.md) | oauth | Stream SDK |
| Weixin | [weixin/](weixin/README.md) | token | WebSocket |
| WeCom | [wecom/](wecom/README.md) | oauth | WebSocket |
| WhatsApp | [whatsapp/](whatsapp/README.md) | token | REST (Meta Cloud API) |
| Tingly | [tingly/](tingly/README.md) | none | InProcess / pluggable |

## ID kinds and send targets

Each inbound adapter stores a **reply target** in `core.Message.Recipient.ID`;
that value is what a later `SendMessage` addresses. It is not a directory of
users: platforms use different entity kinds, and not every entity is a message
receiver. The table is the maintained mapping; each platform's README has the
detail, and `.research/im-id-types.md` keeps the evidence (adapter `file:line`,
SDK `receive_id_type` list).

| Platform | Inbound `Recipient.ID` | Send target | Not a receiver |
|---|---|---|---|
| Telegram | `msg.Chat.ID`, decimal (negative for groups) | the chat id, or `@name` for a public chat | — (a private chat's user id is its chat id) |
| Discord | `msg.ChannelID`, a snowflake | the channel id | user and guild ids (same snowflake shape; only a received message proves it is a channel) |
| Slack | `event.Channel` | a conversation: `D` DM, `C` channel, `G` private group | user ids `U…`/`W…` — open the DM channel first |
| Feishu / Lark | DM: sender `user_id`, else `open_id`; group: `chat_id` | `receive_id_type` selected from the id prefix | `od-` department, `cli_` app id, tenant key (see [feishu](feishu/README.md#id-kinds-and-routing)) |
| DingTalk | `ConversationId` (`cid…`) | the conversation, via its session webhook | staff/user id (`SenderStaffId`) |
| WeCom | `Metadata["chatId"]` (group) or `msg.To` | group `chatid` or external user id | — |
| Weixin | `msg.To` | user id (`wxid…`) or chatroom (`…@chatroom`) | — |
| WhatsApp | `msg.From` | `wa_id` phone number (`phone_number`) | group JID `…@g.us` |
| Tingly | the local transport's chat id | that id | — |

Binding these ids to a Projects thread adds one more rule: only a **sendable**
id may be offered by the bind selector; an unreadable id stays visible but
disabled. That policy and its UI are in
[`.design/projects-im.md`](../../.design/projects-im.md#绑定候选的可发送性id-类型).

## Registry

Platform constructors are mapped from `core.Platform` values to factory
functions in `imbot/registry.go` (`RegisterBuiltinPlatforms`). `imbot/platform/`
is a directory of packages, not a package itself — the global helpers live on
the top-level `imbot` package:

```go
bot, err := imbot.Create(config)
ok := imbot.IsSupported(core.PlatformSlack)
```

Register a custom platform at startup:

```go
imbot.Register(core.Platform("myplatform"), func(cfg *core.Config) (core.Bot, error) {
    return mypkg.NewBot(cfg), nil
})
```

## Adding a new platform

1. Create `platform/<name>/<name>.go` embedding `*core.BaseBot`.
2. Implement `core.Bot` (connect, disconnect, send, react, edit, delete).
3. Add an `Adapter` that converts raw SDK events to `core.Message` and calls `b.EmitMessage(msg)`.
4. Add the platform to the `platformDescriptors` table in `core/platforms.go` — display name, capabilities, behavior defaults, and auth metadata (`AuthType`, `Category`, and `Auth *AuthMapping` describing how a stored auth-map becomes a `core.AuthConfig`). This is the single registration site for all intrinsic platform attributes.
5. Register the constructor in `imbot/registry.go` → `RegisterBuiltinPlatforms()`.
6. If the platform has a settings form, add its UI fields (labels, placeholders, secret flags) to the `platformFormFields` map in `imbot/auth.go`. The intrinsic fields (`AuthType`/`Category`/`DisplayName`) are already derived from core, so do not duplicate them here.
7. Optionally add an `action_render.go` that renders `core.ActionSet` natively, and implement `core.MessageRestater` (see telegram/feishu for the pattern).
8. Add a `README.md` following the pattern of existing platforms, including an
   `## ID routing` section (what the adapter stores as `Recipient.ID`, what
   `SendMessage` accepts, which ids are not receivers), and add the platform to
   the ID kinds table above.
