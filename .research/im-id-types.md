# IM ID types: which IDs can be a binding target

> Research for the Projects IM bind selector. Question: IM platforms use several
> kinds of entity ID; not all of them are a **message receiver**. The bind
> selector lists "chats", but a raw entity id reaching that list must be
> classified before it is offered as a send target. The same entity must stay
> visible in the raw overview (IM settings / access control) with a reason.
>
> Method: read the code that produces the records, the platform adapters, and
> the SDK type comments in `GOMODULE` (no live API calls, no real credentials).
> Docs are cited as `file:line`; SDK comments reproduce the vendor docs.

## 1. Where the binding candidates come from

The Projects bind dialog does not ask the platform for a user/group directory.
It merges two Remote access-control lists, one row per entity the bot has seen:

- **Direct chats** — `GET /api/v1/bots/:bot/chats`
  (`remote/access/resources.go:29`, `internal/server/module/imbot/access_api.go:228`)
- **Groups** — `GET /api/v1/bots/:bot/groups`
  (`remote/access/resources.go:45`, `internal/server/module/imbot/access_api.go:342`)

`frontend/src/pages/projects/imHost.tsx:147-179` flattens both into `ImChat`
(`kind: 'direct' | 'group'`, `chatId = external_chat_id | external_group_id`).
`BindChatDialog` then shows every entry that is not already bound here
(`frontend/packages/projects/src/components/im/BindChatDialog.tsx:49-72`).
The IM settings page (`ImSettingsPage.tsx:101-157`) shows the same list without
the "free candidate" filter — that is the raw overview today.

Both records are created from **inbound** messages only:

| Record | `external_*_id` source | Code |
|---|---|---|
| Direct chat | `msg.GetReplyTarget()` (`Recipient.ID`) | `remote/bot/authorization.go:153-177` |
| Group | same, for group/channel messages | `remote/bot/authorization.go:164-170` |

So, by construction, today's records hold whatever the adapter puts in
`Recipient.ID`. That is the key invariant to audit per platform (§2).

The binding value stored is just `platform + chat_id` (`channel_bindings`,
`projects/internal/db/channels.go:71-111`, API `projects/internal/project/bind.go:29`).
There is **no ID-kind column**, no backend sendability check on bind, and the
persisted audit log is still log-only (`.design/bot-im-alias-display.md` §6,
`internal/server/module/notify/bot_api.go:579-584`). The only place a raw id is
shown with context is the IM settings table.

## 2. Per-platform ID inventory

`Recipient.ID` is what a direct chat/group row stores; "sendable" is whether the
platform can deliver a message to that id **through the path this repo uses**.

### Feishu / Lark

The two are the same bot with a different base URL (`imbot/platform/lark/lark.go`).

| ID kind | Prefix / format | Scope and meaning | Receiver? | Evidence |
|---|---|---|---|---|
| `chat_id` | `oc_…` | A conversation (group **or** the p2p conversation). | **Yes** — `receive_id_type=chat_id` | `imbot/platform/feishu/feishu.go:59-60`; SDK: "支持 open_id/union_id/user_id/email/chat_id" (`oapi-sdk-go/v3@v3.9.10/service/im/v1/model.go:13399`) |
| `open_id` | `ou_…` | A user as **this app** sees them; app-scoped. | **Yes** — `receive_id_type=open_id` (recommended for users) | `feishu.go:61-62`; SDK comment + "如果消息接收者为用户，推荐使用用户的 open_id" (`model.go:13245`) |
| `union_id` | `on_…` | A user as the **developer** sees them; developer-scoped. | **Yes** — `receive_id_type=union_id` | `feishu.go:63-64`; SDK allowed list (`model.go:13399`) |
| `user_id` | no prefix (tenant-assigned) | A user inside the tenant. | **Yes** — `receive_id_type=user_id` | `feishu.go:67-69`; inbound DM prefers it (`feishu.go:369-390`) |
| `email` | contains `@` | A user addressed by email. | **Yes** — `receive_id_type=email` | `feishu.go:65-66`; SDK allowed list |
| `open_department_id` | `od-…` | A department. | **No** — not a message receiver; it is a contact/approval target | SDK: "部门 ID，前缀固定为 `od-`" (`service/approval/v4/model.go:29`); no department value in the message receive list |
| `app_id` | `cli_…` | The app itself. | **No** | Feishu app credential format; not in `receive_id_type` |
| tenant key | 16 hex chars, no prefix | Tenant identity. | **No** | Not in `receive_id_type`; never produced as a chat id |

What the bot actually stores:

- Group/channel message → `chatID` from the event (`oc_…`) (`feishu.go:350-363,391-393`).
- Direct message → the sender's **`user_id`** when present, else **`open_id`**
  (`feishu.go:365-390`); it does not store the p2p `chat_id`.
- Card callbacks use the `chat` value written into the card when it is the same
  DM (`feishu.go` card path, `card_callback.go:99-104`), so DMs stay keyed by
  user id/open id.

Caveat: `on_` (union_id) is a **valid receive id** per the SDK's own allowed
list, even though the task hint treated it as probably invalid. It is kept
sendable. `od-`/`cli_` are the two confidently non-sendable shapes. A tenant key
has no prefix and is indistinguishable from a `user_id`; no current path stores
one, and the sender code already defaults a prefix-less id to `user_id`
(`getReceiveIdType`), so the classifier follows that and notes the residual risk.

### Slack

| ID kind | Prefix | Meaning | Receiver? | Evidence |
|---|---|---|---|---|
| Channel / DM / private group | `C…`, `D…`, `G…` | The conversation `chat.postMessage` addresses. | **Yes** | Adapter stores `event.Channel` (`imbot/platform/slack/slack.go:281`); the same field is the post target (`slack.go:403`) |
| User | `U…` (also `W…`/`B…` for legacy/bot users) | A person, not a conversation. | **No, not directly** — a DM channel (`conversations.open` → `D…`) must be used | Slack `chat.postMessage` takes a channel, not a user; the adapter never puts a user id in `Recipient.ID` |
| Unknown | other | Enterprise-grid/legacy shapes. | **Unknown** | No mapping in code |

The Slack adapter's chat type is decided from `event.Channel`, and its fallback
comment says "DMs typically start with 'D', channels with 'C', groups with 'G'"
(`slack.go:347-354`). So a `U…` id in a chat row is an anomaly worth showing but
not offering.

### Telegram

| ID kind | Format | Meaning | Receiver? | Evidence |
|---|---|---|---|---|
| Chat id | integer (negative for groups/supergroups) | Direct/group/channel conversation. | **Yes** | `imbot/platform/telegram/adapter.go:47` stores `msg.Chat.ID` |
| Public name | `@name` | A public channel/group username. | **Yes** (Telegram accepts a username as `chat_id`) | Bot API semantics; not produced by this adapter |
| Unknown | anything else | — | **Unknown** | — |

Telegram has no separate "user id" vs "chat id" in a private chat: the numeric
id is the receiver.

### Discord

| ID kind | Format | Meaning | Receiver? | Evidence |
|---|---|---|---|---|
| Channel id | snowflake (digits) | DM/channel/thread the bot posts to. | **Yes** | `imbot/platform/discord/adapter.go:45-51` stores `msg.ChannelID` |
| User/guild id | also a snowflake | A person or server, not a post target. | **No** | Discord ids share the snowflake shape, so format alone cannot separate them; the record kind is what makes it a channel |

Because Discord ids are indistinguishable by format, the classifier trusts the
record kind (`direct`/`group` came from a received message → it is a channel),
and only rejects empty/non-numeric values. This is the one platform where
"looks like an id" is not enough — a future user-directory feature must not feed
this list.

### DingTalk

| ID kind | Format | Meaning | Receiver? | Evidence |
|---|---|---|---|---|
| Conversation id | `cid…` (bot `ConversationId`) | The 1:1 or group conversation. | **Yes** while the session webhook is known | `imbot/platform/dingtalk/adapter.go:43`; send looks the conversation up in `webhookMap` (`dingtalk.go:273-281`) |
| Staff/user id | e.g. `manager1234` | A person (`SenderStaffId`). | **No** — the robot replies into a conversation, not to a staff id | Adapter keeps it as sender only (`adapter.go:71-79`) |
| Unknown | other | — | **Unknown** | — |

The send path is webhook-based per conversation; a restarted bot that has not
received a message yet cannot send to a stored conversation even though the id
is the right kind. That is a deliverability issue, not an ID-kind issue, and is
out of scope here.

### WeCom (Enterprise WeChat)

| ID kind | Format | Meaning | Receiver? | Evidence |
|---|---|---|---|---|
| `chatid` | `chatid` metadata / group chat id | The conversation. | **Yes** | `imbot/platform/wecom/adapter.go:34-46` prefers `Metadata["chatId"]`; SDK sends `chatid` (`weixin@v0.4.10/wecom/bot.go:155,227`) |
| External user id | `wm…`/`wo…` | The customer in a direct chat (`msg.To`). | **Yes** for the customer-service send | Same adapter fallback; the SDK's direct reply uses the recipient id |

The SDK's WeCom inbound conversion leaves `To` empty and only sets
`chatId` when the event has one (`wecom/convert.go:11-23`), so a direct chat
without a `chatId` never becomes a record (empty target is skipped by the
gate, `authorization.go:155-157`) — no invalid row is created.

### Weixin (personal WeChat / ilink)

| ID kind | Format | Meaning | Receiver? | Evidence |
|---|---|---|---|---|
| User `wxid` | `wxid_…` | The person in a direct chat. | **Yes** (reply uses `msg.To`, the user id) | `weixin@v0.4.10/message/inbound.go:109-116`; adapter `imbot/platform/weixin/adapter.go:54` |
| Chatroom | `…@chatroom` | A group. | **Yes** | Same recipient path; the SDK declares only direct today |

### WhatsApp

| ID kind | Format | Meaning | Receiver? | Evidence |
|---|---|---|---|---|
| `wa_id` / phone | digits, or `…@s.whatsapp.net` | The person. | **Yes** — the send path posts `phone_number: target` | `imbot/platform/whatsapp/whatsapp.go:516-533` (inbound `From`), `:317`/`:369` (send) |
| Group JID | `…@g.us` | A group. | **No through this bot** — the Cloud API send used here takes a phone number, and the adapter only handles direct text | Same files; inbound handler hard-codes `ChatTypeDirect` (`:526-545`) |

### Tingly (internal/loopback platform)

Ids are the local channel's own; any non-empty id is a chat. Sendable.
(`imbot/core/platforms.go:291-307`.)

### Platforms with a descriptor but no adapter

`googlechat`, `signal`, `bluebubbles` are listed in
`imbot/core/platforms.go:168-230` but have no `imbot/platform/*` implementation.
If a record ever appears for them, the safe answer is **UNKNOWN**: show it,
disable binding, say why.

## 3. Does today's pipeline emit non-sendable ids?

By construction, no: every direct-chat and group row is created from a
`Recipient.ID` that the adapter built as a reply target, and every adapter above
uses a receiver-shaped id. The non-sendable shapes (`od-`, `cli_`, Slack `U…`,
WhatsApp `@g.us`) are **not produced by the current inbound path**.

They can still appear through:

- legacy rows from earlier adapters (the repo has changed Feishu DM targets
  before: `feishu.go:49-57`),
- a hand-written/imported database,
- a future user/group directory or a new platform adapter that feeds the same
  `DirectChat`/`Group` tables,
- a port that forgets the "reply target" invariant.

The selector has no guard for any of these, and the raw row carries no type, so
the failure mode would be a row that looks bindable and then fails to send.
The fix is therefore a **classification guard**, not a data migration.

## 4. Classification implemented

`frontend/packages/projects/src/components/im/imIdKind.ts`:

- `classifyImId(platform, recordKind, rawId) → { kind, class, typeKey, reasonKey?, receiveIdType? }`.
- `class` is `sendable` | `unsendable` | `unknown`.
- The table is per platform (§2); a record kind (`direct`/`group`) is used where
  the platform has no distinguishing prefix (Discord, WeCom, Weixin) or to pick
  between a channel and a user id.
- The id is normalized with the shared `bareId` helper, so stored `feishu:oc_…`
  and bare `oc_…` classify the same.

UI policy:

| Class | Bind selector | Raw overview (IM settings) |
|---|---|---|
| `sendable` | shown, enabled | shown with its type |
| `unsendable` | **hidden** (never offered) | shown, type + "not a message target" reason, action disabled |
| `unknown` | shown but disabled, reason under the row | shown with type + "cannot tell" reason, action disabled |

If every free candidate is `unsendable`, the dialog's empty state says so and
links to IM settings (where the rows are still visible). A quiet line reports
how many entries were hidden.

## 5. Open questions / follow-ups

1. **Feishu `union_id`**: the SDK's allowed `receive_id_type` list includes
   `union_id`, so it is treated as bindable. If the product wants to keep it
   hidden anyway (developer-scoped, rarely usable by a self-built bot), that is
   a product decision; it is a one-line table change.
2. **Slack user ids**: this bot's send path addresses `chat.postMessage` with a
   channel id. A `U…` row is therefore `unknown` (visible, disabled), not
   silently hidden. If Slack is confirmed to accept a user id directly, the
   table can mark it sendable.
3. **Feishu tenant key**: prefix-less ids are treated as `user_id` (the sender
   path's default). A tenant key is not stored by any known path; if one is ever
   imported, it would be misread as a user id. A provenance/kind column on the
   access records is the durable fix.
4. **Backend guard**: the filter lives in the Projects UI. Other API clients can
   still write a `channel_binding` for an arbitrary id. A server-side check
   would need the same table in Go (or a kind column persisted at discovery
   time); worth doing if a second binding client appears.
5. **Dedicated audit view**: the persisted audit events are log-only
   (`.design/bot-im-alias-display.md` §6). The IM settings table is the closest
   raw overview today; a real audit page would be its own task.
