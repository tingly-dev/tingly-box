# Bot / IM entities: ID vs alias — where each is shown today

> Inventory only. Decisions taken after this audit (display format, naming model) are recorded in the design doc §0. The proposal built on this inventory is [`.design/bot-im-alias-display.md`](../.design/bot-im-alias-display.md).
> Audit date: 2026-11-03, against `main` @ `01a128ee2`. Everything was read in code, nothing was run;
> items marked UNVERIFIED are called out where they occur. Paths are repo-relative.

## 0. How to read this

- **Part A** — the data model: every entity with an ID plus a name/alias, where the alias lives, who writes it, delete behaviour, and which HTTP APIs return the ID with or without the alias.
- **Part B** — the frontend: every display site (`frontend/src`, `frontend/packages/projects`).
- **Part C** — everything non-frontend: IM messages the bot sends, logs, errors, CLI, `imbot` adapters, `projects/` module, persisted records.
- This summary (sections 1–3) cross-cuts the three parts. Details and `file:line` are in the parts.

Gap classes in this summary and Part C: **A** alias/name missing · **I** ID missing where needed for lookup · **S** stale or unresolved (empty source, deleted entity, snapshot never refreshed) · **F** inconsistent format/field/key · **W** wrong field (the wrong ID space or field is shown).

> Part B uses its own letters (A alias missing · B ID missing · C stale/unresolved · D inconsistent format · E fallback problem); Part A has none. Mapping to the summary: B→I, C→S, D/E→F.

## 1. Entity cheat sheet

| Entity | ID (what machines use) | Human name | Name set by | Unique? | Notes |
|---|---|---|---|---|---|
| Bot | `uuid` (server-generated as `<unixnano>-imbot`, `internal/db/imbot_settings_store.go:316`; not a real UUID) | `name` | user; web may send empty; QR flows default to `"<platform> Bot"` | no | cannot be cleared once set (`imbot_settings_store.go:181`) |
| Platform | `telegram`, `feishu`, … | `DisplayName` ("Telegram") | compiled table `imbot/core/platforms.go` | yes | frontend keeps 3 more hand-copied name sources |
| IM chat, legacy `bot.Chat` | `ChatID` (external, global PK) | **none** | — | — | pairing / project path / disabled flag live here |
| IM direct chat `access.DirectChat` | `id` (internal uuid) + `external_chat_id` | `alias` (user only) | `PUT …/alias` | no | **no platform-captured name at all**; peer person's name is on `Actor` and never joined |
| IM group `access.Group` | `id` + `external_group_id` | `alias` (user) + `name` (platform) | name captured **once** at discovery | no | `name` empty on feishu/lark/weixin/wecom |
| Actor (person) | `id` + `external_actor_id` | `display_name` | overwritten from every inbound message | no | feishu: raw open_id; weixin/wecom: empty; empty overwrites a good value |
| Group↔actor | `(group_id, actor.id)` | `label` | user | per group | **never displayed anywhere** |
| Route | `id` + target `{kind,id}` (internal uuids) | `name` | user | unique per bot | target comes back unresolved |
| Scenario binding | `Name`, `ChatID` (raw external) | none | — | — | JSON blob in `imbot_settings.scenarios`; older mechanism than Route |
| Projects channel binding | `id`, `platform+chat_id`, `bot_uuid` | `chat_title` | frontend never sends it; host bridge passes `""` | one per `(platform, chat_id)` | snapshot, never refreshed, `bot_uuid` can dangle |
| Thread event source | `source_platform`, `source_chat_id` | none | — | — | immutable raw id |
| Claude Code profile (bot `default_agent`) | `claude_code[:<id>]` | profile `Name` | user | per scenario | bot API returns only the id; profile delete leaves bots dangling |
| Pairing code | in-memory per bot | none | — | — | not an entity with an alias; the value *is* the thing to show |
| API tokens / "sharing keys" | `token_id` | `display_name` | user | — | **No relation to bots/IM** (checked: no bot/imbot/remote references in `internal/server/module/sharing` or `internal/db/api_token_store.go`). Out of scope. |

Two ID spaces collide under the word "chat id": the **external** platform id (what users see in Telegram/Feishu, what they paste into `/join` or the notify API) and the **internal** access uuid (`DirectChat.id`/`Group.id`, used by routes, `target_id` in audit logs, and `NotifyTarget.id` in the UI).

## 2. Worst gaps (ranked by user impact)

1. **Projects shows raw chat IDs everywhere a chat is named** (S/A). UI-created bindings never send `chat_title` (`frontend/packages/projects/src/components/im/useImBindings.ts:58`), the host bridge passes `""` (`internal/server/module/projects/chatbridge.go:118`), and the optional host hook `chatTitle` that `ThreadPanel.tsx:480` calls is **not implemented** in `frontend/src/pages/projects/imHost.tsx`. Result: project/workdir IM tags, “From <platform> · <chat>” source lines and the transcript all show e.g. `-100123456789` even when the user has named the chat. Two different resolvers exist for the same data (`ConversationPane.tsx:356` vs `ThreadPanel.tsx:480`).
2. **Wrong field / alias dropped on the Notify surface** (W/A). The delete-chat confirm prints the **internal uuid** (`frontend/src/components/notify/BotNotifyGroup.tsx:593`). Direct-chat aliases are never mapped onto Notify targets (`BotNotifyGroup.tsx:137-158`), groups map only `name` not `alias`; the test-send picker shows `external_id` in the list but `name` in the input (`NotifyTestDialog.tsx:108` vs `:118`).
3. **Name data is empty or wiped at the source** (S). `upsertActor` overwrites `display_name` on every message, even with `""` (`internal/db/bot_access_store.go:246`); `Group.Name` is never refreshed; `DirectChat` has no name; feishu sets display name = raw sender id (`imbot/platform/feishu/adapter.go:72`) or `""` on its WebSocket path; weixin/wecom send none. Any UI fallback chain is hollow until this is fixed.
4. **Bots are hard to tell apart** (F/I). On creation an empty name is stored as `"<platform id> Bot"` (e.g. `telegram Bot`; `BotConfigDialog.tsx:163`, `:252`), so two bots on one platform have the same name; the display fallback has at least four variants (`name||platform` ×9, `name||platformLabel` ×2, `name||'Bot'`, name-only). The bot **uuid** is visible in only three places (Bots table, Access dialog, a graph-node tooltip); not on the Remote card, Notify card, Projects IM settings, bind dialog, toasts or confirms.
5. **Backend never reads aliases and the bot never names itself** (A). `DirectChat.Alias`, `Group.Alias/Name`, `Route.Name`, profile name are read by **no** log, error, CLI line or IM message. `/help` and `/status` in IM print only IDs, no bot name, profile or chat alias. imbot adapter logs (85 lines) have a fixed `[imbot]` prefix with no bot identity, so two Feishu bots are indistinguishable in logs (`imbot/core/logger.go:60-70`).
6. **Dangling IDs after delete** (S). Deleting a bot leaves Projects `channel_bindings.bot_uuid` (separate DB, no FK), `remote_chats.paired_bot_uuid`; deleting a profile leaves bots’ `default_agent`; deleting a chat leaves its Projects binding. UI shows the raw id with no “deleted” hint.
7. **Toasts and icon-only buttons name nothing** (A). ~20 toasts (“Bot restarted”, “Target blocked”, “Chat deleted”, …) and repeated-row buttons have no entity name, even though the pages list many bots/chats.
8. **Format and key chaos in logs** (F). Bot UUID logged under `uuid`, `bot_uuid`, `bot`, `botUUID`, `channel`; chat under `chat_id`, `chatID`, `target`, free text; `chat_id`(external) vs `target_id`(internal) in the same function; “Bot ID” means three different things (CLI/Ilink/tingly); `bot list` column “ID” is a list index; pairing instruction is `/bind` in CLI help and `/pair` in the server banner.

## 3. Classification by gap type

| Class | Main sites (details in parts) |
|---|---|
| **A alias missing** | Notify graph direct chats (B §3); toasts/aria-labels (B §6 T1); IM `/help`, `/status`, pairing/auth messages (C §1); all bot/imbot log lines except 7 (C §2–3); CLI `bot pair`, `bot config`, `bot_add` echo (C §5); CC profile dialog title; Access “Rename” dialog doesn’t name the chat |
| **I ID missing where lookup needs it** | Remote card & Notify card header (no bot uuid); Projects IM settings chat rows and bind dialog (external id hidden once an alias exists); Notify chat row can’t copy the external id; stderr pairing banner has name but no UUID (`remote/control/bot/manager.go:179`); auth failures log without bot/chat (`authorization.go:81-109`); ~40 imbot adapter lines |
| **S stale / unresolved** | Projects `chat_title`, `ThreadEvent.source_chat_id`, `ChannelBinding.bot_uuid`, `default_agent`, `Route.target.id`, `DirectChat.peer_actor_id` returned unresolved (A §2); `Group.name`, `Actor.display_name` (A §1.4–1.5); deleted entities (A §3.5) |
| **F format / key / term** | “Name” vs “Alias” for the same bot field; chat label has 5 variants (B §6 C1–C6); 4 platform-name sources (B §6 L1–L4); log keys (C §8); `%q` vs `%s`; `/bind` vs `/pair`; “ID” column = index |
| **W wrong field** | Notify delete confirm shows internal uuid; audit logs mix internal `target_id` with external `chat_id`; `ImBot.online` is `enabled`, not running (A §2.6) |

## 4. Things that already work (patterns to reuse)

- `BotAccessDialog.tsx:48` `rowHeader(title, id)`: bold alias, mono external id underneath, id hidden if equal. Best existing UI pattern.
- `CCProfileNode.tsx:49` tooltip “name (id)” and `CCProfileDialog.tsx:96-97` name + `claude_code:<id>`: consistent profile fallback.
- `internal/command/bot_pair.go:98` `Bot:  Name (UUID)`: best human text format in the repo.
- Projects IM text: `[#<seq> <title>]` thread tag, project/folder name first, `#N` as the handle the user types (`projects/internal/project/relay.go`, `chat_commands.go`).
- `Message.GetSenderDisplayName()` (`imbot/core/message.go:243`): the one existing “name else id” resolver; nothing in `remote/**` calls it.
- `manager.go:172-181` logs `uuid + name + platform` as fields: the right log shape.
- Tension between `.design/ux-principles.md` §5 (“show the concrete value, not the alias”) and alias-first display: resolved by the user's `平台 · alias · 群 · id` format, which shows both (see the design doc §0, §3).

## 5. Not verified

- Whether `NotifyPage` / `BotChatsButton` are still reachable (`/notify` redirects; `BotChatsButton` has no importers).
- Whether `bot.Chat.ProjectPath` or `DirectChat.ProjectPath` is the runtime-authoritative project binding.
- Whether any `ErrNoChannel` text (carries a bot uuid) reaches an IM chat or only logs (`chatbridge.go:347,372`).
- Whether the Projects `Host` interface already has a suitable alias-resolution hook (`internal/server/module/projects/host.go`).
- Mock handlers (`frontend/src/mocks/*`) were not audited.
- Output of WhatsApp/tingly adapters’ display names (set in code, not exercised).

## 6. Effect of the decisions on these findings (2026-11-03)

The user decided on the `平台 · alias · 群 · id` display (which superseded an earlier `平台 · alias · id [type]` the same day; the segment order does not change any finding below), on storing the platform name as the default alias, and on bots defaulting to their platform account name (design doc §0). That changes how several findings above should be read:

| Finding | Before | After the decisions |
|---|---|---|
| Group `name` and `alias` are two fields (R-A §1.3) | Two sources to chain at read time | Platform title is promoted into `alias` as default, so the data model change (not just display) is now part of the work; priority up |
| `upsertActor` overwrites `display_name` with `""` (§2 item 3) | Data-quality bug | Now a stated invariant violation (design N1); fix is a prerequisite, not a nice-to-have |
| `Group.name` never refreshed; DMs have no name (§2 item 3) | Stale / missing source | With "platform name = default alias" this becomes the main data gap; DM name must be copied into `DirectChat.alias` (design §4.3) |
| Bot stored name `"<platform id> Bot"` (§2 item 4) | Duplicate-name problem needing a disambiguator | Obsolete as a stored value: bots now default to their platform account name, else empty (D4, decided), and the id is part of every display so a separate disambiguator rule is dropped |
| Feishu display name = raw sender id (§2 item 3) | Hollow name | Must be treated as "no name" (N4), otherwise the id is stored as an alias |
| Five chat labels / four platform-name sources (§2 item 1, §3 class F) | Inconsistency | Collapse into one format; platform label comes from the descriptor, the raw platform id is never shown |
| Hidden bot/chat ids on several surfaces (§3 class I) | Id missing | Id is now a mandatory segment; where it is shortened, a copy path must exist |
| Priority order | Display bugs first | Naming-model backend fix (P1) before the shared display component; P0 bug fixes optional and independent |

Implementation status of these findings is in the design doc §14 (all gap classes addressed except persisted audit events). Nothing in the inventory itself was wrong; the *ranking* changes: "names are empty or wiped at the source" is now the first thing to fix because the new format depends on it.

---

# Part A — Data model and API shapes


Repo: tingly-box worktree audit-and-design-bot-im-alias-di-c6b317. All paths relative to repo root. Everything below was read in code unless marked UNVERIFIED.

### 0. Headline findings

1. TWO parallel chat models exist.
   - Legacy `bot.Chat` / table `remote_chats` (remote/control/bot/chat_store.go:74, internal/db/remote_chat.go:15). Keyed by the platform-native chat id ALONE (PK `chat_id`, no bot, no platform dimension). Has NO name/alias field at all.
   - New `access.DirectChat` / `access.Group` (remote/access/resources.go:29,43), tables `remote_direct_chats` / `remote_groups`, keyed by internal UUID `id`, unique (bot_uuid, platform, external_chat_id). Carries `alias` (user) and, for Group only, `name` (platform-captured).
   - Relation: DirectChat.ExternalChatID == bot.Chat.ChatID (both the msg reply target). Bridged in remote/control/bot/authorization.go:99-108 (`legacyChats.IsChatPaired(externalTarget, botUUID)` -> `PairDirectChat`). No FK between them.
2. The old chat-list endpoint (notify `BotAPIHandler.ListChats`, internal/server/module/notify/bot_api.go:374, `ChatSummary` :75, swagger `BotChatsResponse` internal/server/module/notify/bot_routes.go:88) is NOT REGISTERED. `BotAPIHandler.RegisterRoutes` (bot_routes.go:106-134) registers only notify / interact / interact-wait. `GET /bots/:bot/chats` is served by the access API (`ListDirectChats`, internal/server/module/imbot/access_api.go) -> list of DirectChat. So `bot.Chat` can never return a name, and `ChatSummary`, `DeleteChat`, `SetChatDisabled` HTTP handlers are dead code (handlers exist at bot_api.go:413/446, no route).
3. DirectChat has no platform-captured name column. The only name sources for a DM are (a) user alias, (b) peer Actor.DisplayName (via `peer_actor_id`), which the DirectChat API never resolves. Groups have `name` captured ONCE at first discovery and never refreshed.
4. Frontend host (frontend/src/pages/projects/imHost.tsx:38-66) builds chat titles as `platformLabel · (alias || bare external id)`. It ignores `Group.name` and the peer actor's display name, so a group the platform named "Dev Team" shows its raw id unless the user set an alias.
5. Projects `ChannelBinding.chat_title` is effectively never populated by the bridge (always passes `""`): internal/server/module/projects/chatbridge.go:118 (`ChannelToFolder(..., "", folder ...)`), projects/projects.go:228 (`BindChannel(..., "", ...)`). Only a direct `PUT /projects/channel-bindings` body with `chat_title` sets it (frontend bind calls in frontend/packages/projects/src/components/im/useImBindings.ts:57-58 do NOT send it). UI fallback is `chat_title || chat_id` (frontend/packages/projects/src/components/im/ProjectImBindings.tsx:75,107). The binding therefore shows raw chat_id even when the DirectChat has an alias.
6. Sharing keys / API tokens (internal/server/module/sharing, internal/db/api_token_store.go) have NO relation to bots/IM. grep for bot/imbot/remote in those files returns nothing. They are user/team scoped (`display_name`, `team_id`). Bot API auth is the operator user token (middleware), not a sharing key.

### 1. Entity tables

Columns: entity | ID field(s) | alias field(s) | defined at | storage | set by | unique? mutable? empty? | delete behaviour

#### 1.1 Bot (IM bot account)

| entity | ID | alias | defined at | storage | set by | unique/mutable/empty | delete |
|---|---|---|---|---|---|---|---|
| Bot (db view) | `uuid` (`Settings.UUID`) | `name` | internal/db/imbot_settings_store.go:14-37 (Name :16); GORM record internal/db/imbot_settings.go:9-35 (Name :10) | table `imbot_settings`, PK `bot_uuid`, col `name` | User via create/update API (handler.go:170 trims `req.Name`; update only when non-empty handler.go:295). Server defaults: Weixin QR flow `platform + " Bot"` (wechat_qr.go:330-335), Feishu QR flow same (feishu_qr.go:320-322). CLI `remote add` passes `botName` (internal/command/bot_add.go:73). Web create may send empty. | NOT unique (no index). Mutable. CAN BE EMPTY (create path allows ""). Cannot be cleared once set: UpdateSettings ignores empty name (imbot_settings_store.go:181 `if settings.Name != ""`). UUID format for server-generated: `<unixnano>-imbot` (imbot_settings_store.go:317), not a real UUID. | `DeleteSettings` (imbot_settings_store.go:224) deletes row. FK CASCADE removes bot_capabilities, remote_actors, remote_direct_chats, remote_groups, remote_routes (bot_access_store.go:149-160; foreign_keys=1 in internal/db/open.go:38). NOT cleaned: legacy `remote_chats.paired_bot_uuid` (remote_chat.go:25), Projects `channel_bindings.bot_uuid` (separate DB file projects/internal/config/config.go:34 `tingly-project.db`, no FK) -> dangling bot ids. |
| Bot (runtime) | `UUID` | `Name` | remote/control/bot/chat_store.go:10-37 (Name :12) `BotSetting` | in-memory copy of settings | derived from Settings | same | n/a |
| Bot status | `uuid` | `name` | internal/server/module/imbot/manager.go:31 `BotStatus` | computed | from Settings | n/a | n/a |
| Scenario BotInfo/Resolved | `UUID`, `BotUUID` | `Name`, `BotName` | remote/binding/binding.go (BotInfo, Resolved) | computed | from Settings | n/a | n/a |

Stale risk: bot name is stored once on the row; every consumer re-reads, so low risk. Display fallback is inconsistent: CLI prints `-` (internal/command/bot.go:208-216,248-254), frontend host falls back to platform label (imHost.tsx:30 `b.name || platformLabel(b.platform)`).

#### 1.2 Platform

| entity | ID | alias | defined at | storage | set by | notes |
|---|---|---|---|---|---|---|
| Platform | `ID` (`telegram`, `feishu`, `lark`, ...) | `DisplayName` ("Telegram", "Lark", "WeCom", "Google Chat") | imbot/core/platforms.go:15-30 `PlatformDescriptor`; lookup `GetPlatformName` :364-369, `PlatformNames` :355 | compiled table, not DB | code | static, never stale. Exposed over HTTP as `PlatformConfig.display_name` (internal/server/module/imbot/types.go `PlatformConfig`, GET /imbot-platforms handler.go:471). Frontend ignores that and keeps TWO more hand-copied sources: `PLATFORM_NAMES` map in frontend/src/pages/projects/imHost.tsx:19-21 (missing e.g. whatsapp/discord/slack) and i18n key `layout.platforms.*` (imHost.tsx:23-24). |
| Bot.Platform / Chat.Platform / DirectChat.Platform / Group.Platform / Actor.Platform / ChannelBinding.platform | string id only | none | respective structs | respective tables | adapter | every entity carries the raw id only; no response carries the platform display name. |

#### 1.3 IM chats (three representations)

| entity | ID field(s) | alias field(s) | defined at | storage | set by | unique/mutable/empty | delete |
|---|---|---|---|---|---|---|---|
| Legacy Chat (`bot.Chat`) | `ChatID` (platform-native, may carry platform prefix e.g. `telegram:123`); `Platform`; `PairedBotUUID`; `OwnerID`, `WhitelistedBy`, `PairedSenderID` (platform user ids) | NONE. No name, alias or title anywhere | remote/control/bot/chat_store.go:74-109; GORM `RemoteChatRecord` internal/db/remote_chat.go:15-40 | table `remote_chats`, PK `chat_id` only (comment in chat_store.go:166 and `.design/bot-capability-access-control.md` "no global bare chat_id key" says this is being retired) | created on first inbound via `GetOrCreateChat(chatID, platform)` (remote_chat_store.go:151-177) | PK unique across ALL bots/platforms (cross-platform collision guard at :160) | `DeleteChat` hard-deletes (remote_chat_store.go:300); only reachable via the unrouted notify handler -> effectively no UI path. Not cascaded from bot delete. |
| DirectChat (`access.DirectChat`) | `id` (UUID, internal), `bot_uuid`, `platform`, `external_chat_id`, `peer_actor_id` (-> Actor.id) | `alias` (user). No platform-captured name | remote/access/resources.go:29-41 (Alias :34); record internal/db/bot_access_store.go:49-61 (Alias :54); DDL :153 (no alias col in CREATE; added by migration :167-175) | table `remote_direct_chats`, UNIQUE(bot_uuid, platform, external_chat_id) | row auto-created on first DM `DiscoverDirectChat` (bot_access_store.go:296-322, called from remote/control/bot/authorization.go:93) with NO name and default-deny perms. Alias only via `PUT /bots/:bot/chats/:chat/alias` -> `SetTargetAlias` (bot_access_store.go:1054; handler access_api.go:564-580). | alias not unique; mutable; empty = unset (`TrimSpace`, empty allowed so alias can be cleared, store :1064). Name never refreshed because there is none. | `DeleteDirectChat` (bot_access_store.go:392): refuses with `target_has_routes` if a Route points at it (HTTP 409); otherwise deletes, perms cascade. `remote_routes.direct_chat_id` FK has no cascade, hence the guard. Projects `channel_bindings` rows keyed by (platform, external chat id) are NOT touched -> binding keeps the id, loses alias source. Legacy `remote_chats` row not touched. |
| Group (`access.Group`) | `id` (UUID), `bot_uuid`, `platform`, `external_group_id` | `alias` (user) AND `name` (platform-captured) | remote/access/resources.go:43-54 (Alias :48, Name :49); record bot_access_store.go:75-88; DDL :155 (has `name`, no `alias`; alias added by migration) | table `remote_groups`, UNIQUE(bot_uuid, platform, external_group_id) | `name` = `msg.Recipient.DisplayName` at FIRST discovery only (`DiscoverGroup` bot_access_store.go:477-505; existing row returns early at :481-483, so name is never refreshed -> stale if the group is renamed, permanently empty if first message came with empty Recipient.DisplayName). `alias` via PUT /groups/:group/alias. | neither unique; both mutable (alias), name immutable after create; both may be empty | `DeleteGroup` (bot_access_store.go:548) same route guard as DirectChat; group_actors/perms cascade |
| Chat title in Projects (`db.ChannelBinding`) | `id` (UUID), `platform`, `chat_id` (= external chat id), `bot_uuid` (optional, "last bot"), `thread_id`, `target_project_id`, `target_thread_id` | `chat_title` | projects/internal/db/models.go:248-282 (ChatTitle :254) | `channel_bindings` in `tingly-project.db`, UNIQUE(platform, chat_id) (idx_channel_chat) | only `PUT /projects/channel-bindings` body `chat_title` (projects/internal/server/module/channel/handler.go:33) or `/new`-style folder commands with `title` arg. Bridge always passes "" (see headline 5). Upsert keeps previous non-empty title when new one is empty (projects/internal/db/channels.go:94-96) but overwrites when non-empty. | one binding per (platform, chat_id) regardless of bot; mutable; usually EMPTY; snapshot, never refreshed from DirectChat.alias / Group.name | `DeleteChannelBinding` (channels.go:114). Deleting the bot or DirectChat leaves binding (no cross-module FK). `bot_uuid` records "last bot that brought chat in" (channels.go:98-100) and can dangle. |
| Thread event source | `source_platform`, `source_chat_id` | none | projects/internal/db/models.go:194-195 | `thread_events` columns | set from `Source{Kind: im, Platform, ChatID}` in project/bind.go (PostFromChannel) | immutable snapshot of raw ids | n/a |

#### 1.4 People (Actors)

| entity | ID | alias | defined at | storage | set by | unique/mutable/empty | delete |
|---|---|---|---|---|---|---|---|
| Actor (`access.Actor`) | `id` (UUID), `bot_uuid`, `platform`, `external_actor_id` | `display_name` | remote/access/resources.go:18-27 (DisplayName :23); record bot_access_store.go:36-47 | `remote_actors`, UNIQUE(bot_uuid, platform, external_actor_id) | Upserted on EVERY inbound message from `msg.Sender.DisplayName` (authorization.go:79 -> `upsertActor` bot_access_store.go:240-253), also on pairing (`PairDirectChat` :327, authorization.go:108) and manual `PUT /groups/:group/actors/:actor` body `display_name` (access_api.go GroupActorPutRequest access_api.go:79). | not unique. Refreshed on every message BUT the ON CONFLICT clause (bot_access_store.go:246) writes `display_name = <incoming>` UNCONDITIONALLY, so an empty incoming name ERASES a good one. Quality per adapter (see 1.5). | cascades with bot; FK from `remote_group_actors.actor_id` ON DELETE CASCADE; `remote_direct_chats.peer_actor_id` FK has no cascade. |
| GroupActor binding | `group_id`, `actor.id` | `label` (user, optional) | resources.go:63-70 (Label :66); record bot_access_store.go:99-107 | `remote_group_actors` | `PUT .../actors/:actor` body `label` (AddGroupActor bot_access_store.go:601 upserts label) | per (group, actor); mutable; empty ok | `RemoveGroupActor` deletes binding + perms |

#### 1.5 Where Sender/Recipient.DisplayName actually comes from (imbot/core/types.go:72-86)

`Sender` :73-79 and `Recipient` :82-86 both carry `DisplayName`. Per adapter (verified in imbot/platform/*):

| platform | Sender.DisplayName | Recipient.DisplayName (group name) |
|---|---|---|
| telegram | first+last name, else username (telegram/adapter.go:124-139) | `msg.Chat.Title` (:47); empty for DMs; callback path "" (:106) |
| discord | GlobalName/Username(+#disc) (discord/adapter.go:105-123) | channel name via `getChannelName` (:51); "" on one path (:87) |
| feishu | `detail.SenderID` i.e. the raw open_id, comment says "fetch user info in production" (feishu/adapter.go:70-75) -> alias == id | "" (:54) |
| dingtalk | `SenderNick` (dingtalk/adapter.go:70-79) | `ConversationTitle` (:42); "" on other path (:61) |
| slack | profile display name / real name via API (slack.go:263-277) | channel name via API (:279-290) |
| weixin | "" (weixin/adapter.go:55) | "" (:54) |
| wecom | "" (wecom/adapter.go:42) | "" (:43) |
| whatsapp | set (whatsapp.go:515) | set (:527) |
| tingly | set | set |

Consequence: group `name` is empty for feishu/lark/weixin/wecom; actor display_name is the raw id for feishu and empty for weixin/wecom, and each such message overwrites any manually entered display_name with "".

#### 1.6 Routes, capabilities

| entity | ID | alias | defined at | storage | set by | unique/mutable/empty | delete |
|---|---|---|---|---|---|---|---|
| Route (notify) | `id` (UUID), `bot_uuid`, `target {kind, id}` (DirectChat.id or Group.id) | `name` (required) | resources.go:72-83 (Name :75), `TargetRef` remote/access/types.go:56; record bot_access_store.go:120-132 | `remote_routes`, UNIQUE(bot_uuid, name), CHECK exactly one of direct_chat_id/group_id | user via `POST/PUT /bots/:bot/routes` (access_api.go RouteWriteRequest access_api.go:101) | name unique per bot; mutable; required non-empty (handler 400s on empty, access_api.go:522,538) | `DeleteRoute`; deleting its target is blocked (409) |
| BotCapability | (`bot_uuid`, `capability` enum: notify, remote_control) | none (enum code) | resources.go:9-16 | `bot_capabilities` | defaults at bot create | n/a | cascades with bot |
| Pairing code | none persisted (in-memory per bot uuid, imbot/core/pairing.go) | none | imbot/core/pairing.go | memory only, lost on restart | `GET/POST /imbot-settings/:uuid/pairing-code[/rotate]` | n/a | n/a. Pairing RESULT lives in `remote_chats.is_paired/paired_bot_uuid/paired_sender_id` and `remote_direct_chats.peer_actor_id/paired_at` (peer_actor_id is an Actor id; sender id raw string is not name-resolved). |

#### 1.7 Bindings (scenario) and agents

| entity | ID | alias | defined at | storage | notes |
|---|---|---|---|---|---|
| Scenario binding (`binding.Binding`) | `Name` (scenario code), `ChatID` (raw external id) | none | remote/binding/binding.go (Binding) | JSON blob in `imbot_settings.scenarios` (imbot_settings.go:31); returned raw to UI as `Settings.scenarios` | used for delivery (remote/scenario/runtime_default.go:114 `Target{ChatID: resolved.Binding.ChatID}`). Raw chat id, no alias; the access `Route` (internal UUID target) is the newer replacement. Chat alias cannot be resolved from this without matching (bot, platform, external id) against remote_direct_chats/groups. |
| Claude Code profile as Bot.DefaultAgent | `default_agent` string `claude_code` or `claude_code:<profileID>` (handler.go:~540 `validateDefaultAgent`; Settings field imbot_settings_store.go:25; Record imbot_settings.go:17 comment says "Default Agent UUID" - misleading) | profile `Name` in `typ.ProfileMeta` (internal/typ/type.go:367-372; `ID` e.g. "p1", `Name` unique per base scenario, config.go profile.go:132-144,204-218) | config file `Profiles`, not DB | Bot stores only the id, API returns the id string; UI must join to profiles list itself (frontend uses `ccProfileIdFromDefaultAgent`, e.g. components/bot/RemoteControlGraph.tsx:32). Profile rename is safe (id stable) but profile DELETE (internal/config/profile.go:228, scenario/handler.go:591-614) contains no cleanup of bots' `default_agent` (grep default_agent in internal/config, scenario module: none) -> dangling profile id; validated only at write time. |
| Projects thread profile | `Thread.Profile` (id), `ProfileOverride`, project `CoordinatorProfile/ThreadProfile` | none resolved | projects/internal/db/models.go:38-39,122,138; `ChannelBinding.Profile` :271 (json "-") | Projects DB; "Projects sees a profile only as a name" (internal/server/module/projects/launch.go:17-18) | ids only; no profile display name in Projects responses. |

#### 1.8 Sessions / agentboot

| entity | ID | alias | notes |
|---|---|---|---|
| Remote session (agentboot Claude session bound to a chat) | `Thread.SessionID` (Claude session id, projects/internal/db/models.go:116), `Thread.ID` | `Thread.Title` (:100), `Thread.Seq` | `bot.Chat` doc says sessions are NOT part of a Chat; they live in Projects (chat_store.go:70-72). Old `remote_sessions` store is gone (only mentioned in .design/remote-storage.md). The link chat -> session is `channel_bindings.thread_id` (+ `focus_title`, `focus_seq` joined at read: channels.go:12 `bindingSelect`, models.go:276-281). agentboot only carries a raw `ChatID` string in events/context (agentboot/events.go:37,57; agentboot/claude/transport.go:44,198,225) - no names. |
| Folder / Project | `Project.ID`, `path` | `Project.Name`; `project_id` on binding resolved only via thread join | binding returns `project_id` but NOT project name (ChannelBinding json fields models.go:276-281: focus_title, focus_seq, focus_agent, target_title, target_seq; no project name). |

#### 1.9 Others found in the area

- `GroupActor.Actor` nested full Actor (resources.go:63) - good, carries display_name + label.
- Feishu QR flow `FeishuRegStartRequest.BotName` (imbot/types.go:~100) deferred creation name; `temp-` prefixed bot uuid placeholder (routes.go feishu comment) -> real uuid returned later as `FeishuRegStatusData.BotUUID`.
- Weixin credentials store `bot_id`, `user_id` (ilink) inside `Auth` map (wechat_qr.go:300-301): platform-side identities of the bot account, no display use.
- API tokens (sharing): `TokenID`, `DisplayName` (internal/db/api_token_store.go:15-26; internal/server/module/sharing/types.go) - unrelated to bots/IM.
- Teams: internal/db/team_store.go - unrelated.

### 2. HTTP APIs that return these ids

Legend: Y = alias/name present and usable; PART = present but incomplete; N = id only.
Base path `/api/v1` in all cases.

#### 2.1 Root module: bot settings / platform (internal/server/module/imbot)

| endpoint | response struct (file:line) | ids returned | alias present? | missing resolved refs |
|---|---|---|---|---|
| GET /imbot-settings, GET/POST/PUT /imbot-settings/:uuid | `ListResponse` / `SettingsResponse` internal/server/module/imbot/types.go:15,21 wrapping `db.Settings` internal/db/imbot_settings_store.go:14 | `uuid`, `platform`, `default_agent` (id string), `scenarios` (raw JSON, contains `chat_id`s) | Y for bot `name` (may be ""), platform id only (no platform display name) | `default_agent` -> profile id only, no profile name; `scenarios[].chat_id` raw with no chat alias; no pairing/peer info |
| POST /imbot-settings/:uuid/toggle | `ToggleResponse` types.go:68 | none | n/a | n/a |
| GET /imbot-settings/:uuid/pairing-code (+rotate) | `PairingCodeResponse` types.go:59 | none | n/a | does not say which bot (name) it is for |
| POST /imbot-admin/restart/:uuid, /reload | gin.H handler.go (`uuid`) / `BotStatus` manager.go:31 | `uuid` | Y `name`, platform id | none |
| GET /imbot-platforms, /imbot-platform-config | `PlatformConfig` types.go (`platform`, `display_name`) | platform id | Y (the only API with platform display name) | not used by Projects embed or imHost (which hard-codes) |
| Feishu / Weixin QR flows | `FeishuRegStatusData` types.go (`bot_uuid`), `QRStatusResponse` wechat_qr.go | `bot_uuid` | N | no bot name in status reply |

#### 2.2 Root module: bot access API (registered in internal/server/module/imbot/access_routes.go)

| endpoint | response struct (file:line) | ids returned | alias present? | missing resolved refs |
|---|---|---|---|---|
| GET /bots/:bot/capabilities, PUT .../:capability | `CapabilityListResponse` access_api.go:35, `CapabilityUpdateResponse` :39 -> `access.BotCapability` resources.go:9 | `bot_uuid` | N | no bot name (caller already has it) |
| GET /bots/:bot/chats | `DirectChatListResponse` access_api.go:68 -> `DirectChatDetailResponse` :64 -> `access.DirectChat` resources.go:29 | `id`, `bot_uuid`, `platform`, `external_chat_id`, `peer_actor_id` | PART: `alias` only (may be empty) | `peer_actor_id` NOT resolved to Actor (so no `display_name` of the person, no `external_actor_id`); no bot name; no platform label; no bound Project/thread info. There is no endpoint at all to GET an Actor by id for a DM. |
| GET /bots/:bot/chats/:chat | `DirectChatDetailResponse` :64 | same as above | PART (alias) | same: peer actor unresolved |
| PUT /bots/:bot/chats/:chat/alias | request `AliasUpdateRequest` access_api.go:560, response `OKResponse` :116 | none | write path | none |
| GET /bots/:bot/groups | `GroupListResponse` :76 -> `access.Group` resources.go:43 | `id`, `bot_uuid`, `platform`, `external_group_id` | Y: `alias` AND `name` (name may be empty/stale) | no bot name |
| GET /bots/:bot/groups/:group | `GroupDetailResponse` :71 | group + `actors []GroupActor` | Y group alias/name; actors carry `actor.display_name` + `label` | none beyond staleness |
| GET /bots/:bot/groups/:group/actors | `GroupActorsResponse` :84 -> `access.GroupActor` resources.go:63 (nested `Actor` :18) | `group_id`, `actor.id`, `actor.external_actor_id`, `bot_uuid` | Y: `actor.display_name`, `label` | display_name may equal raw id (feishu) or be erased to "" (weixin/wecom) |
| PUT /bots/:bot/groups/:group/actors/:actor | returns bare `access.Actor` (access_api.go PutGroupActor) | actor ids | Y display_name | n/a |
| GET/POST/PUT /bots/:bot/routes[/:route] | `RouteListResponse` :113, `RouteResponse` :110 -> `access.Route` resources.go:72 | `id`, `bot_uuid`, `target.{kind,id}` | Y route `name` | **`target.id` (DirectChat/Group UUID) is NOT resolved**: no target alias/name/external id in the Route response; UI must join `GET /chats` and `GET /groups` itself (frontend/src/components/notify/BotNotifyGroup.tsx works from targets it already loaded) |
| POST /bots/:bot/chats/:chat/authorize-check, groups/:group/authorize-check | `AuthorizeCheckResponse` :94 -> `access.AuthorizationDecision` types.go:136 incl. `facts` (`PeerActorID`, target ids) | `actor_id`, `peer_actor_id` | N | actor / target not resolved |
| PUT .../blocked, .../permissions, DELETE ... | `OKResponse` | none | n/a | 409 body `{"error":"target_has_routes","details":"target_has_routes: N"}` (accessError access_api.go:141) lists no route names |

#### 2.3 Root module: notify / interaction (internal/server/module/notify)

| endpoint | response struct | ids | alias present? | missing |
|---|---|---|---|---|
| POST /bots/:bot/notify | `BotNotifyResponse` bot_routes.go:61 | none (request takes `target {kind,id}`) | n/a | n/a |
| POST /bots/:bot/interact, GET /bots/:bot/interact/:request_id | `BotInteractStartResponse` / `BotInteractWaitResponse` | `request_id` | n/a | no chat/bot names in decision |
| (documented) GET /bots/:bot/chats [legacy] | `BotChatsResponse` bot_routes.go:88 / `BotChatSummary` :35 | `chat_id`, `platform` | N - no name field | **dead model**: handler bot_api.go:374 never registered; swagger model lingers. If it were wired it could NOT carry a name because `bot.Chat` has none. |
| Claude Code hook notify `/tingly/:scenario/notify|wait` | internal/server/module/notify/handler.go | resolved via `binding.Resolved` (`BotName`) | n/a | n/a |

#### 2.4 Root module: Projects facade

internal/server/module/projects (routes.go:34-47) only mounts the Projects module's own routes (`h.app.Register(group)`) behind auth + feature flag; it adds NO endpoints of its own and joins NO bot/chat names. Host-side hooks (`ChatBridge`, chatbridge.go) take raw `(platform, chatID, botUUID)` and discard titles (chatbridge.go:118 passes "" as title; `ProjectChoices` :153-171 returns project names only). `ImHost.chatTitle` (frontend/packages/projects/src/host/ProjectsHost.tsx:82) is an optional host hook that Tingly Box's `imHost` (frontend/src/pages/projects/imHost.tsx:73-85) does NOT implement.

#### 2.5 Projects module (projects/internal/server/module/*)

| endpoint | response struct (file:line) | ids returned | alias present? | missing resolved refs |
|---|---|---|---|---|
| GET /projects/channel-bindings | `BindingsResponse` projects/internal/server/module/channel/handler.go:19 -> `db.ChannelBinding` projects/internal/db/models.go:248-282 | `id`, `platform`, `chat_id`, `bot_uuid`, `thread_id`, `target_project_id`, `target_thread_id`, `project_id`, `folder` | PART: `chat_title` (mostly empty); thread side Y (`focus_title`, `focus_seq`, `focus_agent`, `target_title`, `target_seq`) | **no bot name for `bot_uuid`; no chat alias (DirectChat.alias / Group.alias/name)**; no platform label; no project name for `project_id`/`target_project_id` (only thread titles) |
| PUT /projects/channel-bindings | request `BindChannelRequest` handler.go:24, response `db.ChannelBinding` | same | `chat_title` only if client supplied | same; Tingly Box frontend never supplies `chat_title` (useImBindings.ts:57-58) |
| DELETE /projects/channel-bindings/:id | `OKResponse` | none | n/a | n/a |
| GET /threads/:id/events (and project conversation events) | `EventsResponse` projects/internal/server/module/thread/handler.go:58 -> `db.ThreadEvent` models.go:184-205 | `source`, `source_platform`, `source_chat_id`, `source_ref` (thread id) | N for chat | **session/event origin shows raw `source_chat_id`; no alias, no bot**. UI resolves via bindings' `chat_title` (ProjectImBindings.tsx:86) then falls back to the id (ConversationPane.tsx:356) |
| GET /threads/:id, GET /projects/:id/threads | `ThreadDetailResponse` thread/handler.go:50, `ThreadsResponse` :26 -> `db.Thread` | `id`, `session_id`, `project_id`, `profile`, `profile_override` | Y thread `title`/`seq` | profile ids unresolved (no profile name); thread does not say which IM chats are bound to it (needs the bindings endpoint filtered by `thread_id`) |
| project settings | projects/internal/server/module/settings/handler.go | `coordinator_profile`, `thread_profile` ids | N | profile names unresolved (UNVERIFIED which fields the response exposes; model fields at projects/internal/db/models.go:38-39) |

#### 2.6 Other places that surface bot ids/names

- Frontend `listBots` (imHost.tsx:27-34) turns `Settings.name` into `ImBot.name`, with fallback to platform label. `ImBot.online` is actually `enabled !== false`, not running (listBots :33), unlike `BotStatus.running`.
- CLI: `tingly-box bot list/show` print `-` for empty name (internal/command/bot.go:208-216,248-254) and raw chat ids for `unpair` (bot.go:95-99).
- Sharing keys / API tokens: `TokenCreateResponse`, `APITokenInfo` (internal/server/module/sharing/types.go) carry `token_id`, `user_id`, `team_id`, `display_name`. No bot/chat reference. Checked: no occurrence of bot/imbot/remote in sharing module or api_token_store.go.

### 3. Answers to the specific questions

1. Can list-chats return a chat name given Chat has none? The live endpoint returns `DirectChat`, not `bot.Chat`, so it returns `alias` only (user-entered). There is no platform-captured name for a DM at all in the data model; the closest is the peer Actor's `display_name`, which is stored (remote_actors.display_name) but not joined into the DirectChat response. For groups, `name` is stored and returned by GET /groups. The legacy `ChatSummary` (bot.Chat based) cannot carry a name and is unrouted.
2. Two parallel models: yes. `bot.Chat` (global id, pairing/whitelist/project path/context token/disabled flag) vs `DirectChat`/`Group`/`Actor` (per bot, UUID, alias/name, permissions). Both are written on inbound traffic; authorization reads DirectChat/Group, while `bot.Chat` still carries `is_paired`, `project_path`, `context_token`, `disabled`. `DirectChat.ProjectPath` / `Group.ProjectPath` also exist (resources.go:38,52) alongside `bot.Chat.ProjectPath` - duplicated state (UNVERIFIED which is authoritative at runtime).
3. Id-to-id references that ship unresolved: Route.target.id; DirectChat.peer_actor_id; ChannelBinding.bot_uuid and chat_id; ChannelBinding.project_id/target_project_id (no project name); ThreadEvent.source_chat_id; Settings.default_agent (profile id); Settings.scenarios[].chat_id; AuthorizationDecision facts; Thread.profile / profile_override.
4. Alias write paths and their reach: DirectChat/Group alias (PUT .../alias) is not propagated to ChannelBinding.chat_title nor to any Projects response; Group.name is never refreshed; Actor.display_name is overwritten on every message (including with ""); Bot.name cannot be cleared; Platform display name exists in 3 separate copies on the frontend side.
5. Dangling ids after delete: deleting a bot leaves Projects `channel_bindings.bot_uuid`, legacy `remote_chats.paired_bot_uuid`, and `default_agent` references to deleted profiles; deleting a DirectChat/Group does not touch the Projects binding keyed by its external id; deleting a Claude Code profile leaves bots' `default_agent` pointing at it (validated only on write, handler.go:528-548 (validateDefaultAgent :532)).

### 4. UNVERIFIED

- Whether `bot.Chat.ProjectPath` or `DirectChat.ProjectPath` is the runtime-authoritative project binding now that Projects owns bindings.
- Exact field set of the Projects settings response (profile ids).
- WhatsApp and tingly adapter DisplayName quality (set in code, not exercised).
- Whether `/imbot-platforms` is consumed anywhere in the frontend for labels (not searched beyond imHost.tsx).

---

# Part B — Frontend display sites

Scope: frontend/src/**, frontend/packages/projects/**, frontend/packages/ui (no bot/IM sites found there).
Paths below are relative to `frontend/` unless absolute. Line numbers verified by reading the code.
Gap classes: A = alias missing, B = ID missing where needed, C = stale/unresolved, D = inconsistent format, E = fallback problem.

### 0. Entities and data shapes (verified)

- Bot: `BotSettings` (src/types/bot.ts:17-24) = codegen Settings; fields `uuid`, `name` (UI labels it "Alias" in the edit dialog, "Name" in the table), `platform` (raw id such as `telegram`), `default_agent`.
- Direct chat: `DirectChat` (src/types/bot.ts:35) has `id` (internal uuid), `bot_uuid`, `platform`, `external_chat_id`, `alias?`, `peer_actor_id?`. There is no actor display name on it.
- Group: `BotGroup` (src/types/bot.ts:37) has `id`, `external_group_id`, `alias?`, `name?`.
- Actor: `GroupActor.actor` has `id`, `external_actor_id`, `display_name?`, plus `label?` (src/types/bot.ts:36). The UI never shows `label`.
- `NotifyTarget` (src/types/bot.ts:44-55): `id` (internal uuid), `external_id`, `name?`. It has no alias field.
- `BotChat` (src/types/bot.ts:237): `id`, `chat_id`. It has no name or alias.
- Projects `ImBot` {uuid,name,platform,online} and `ImChat` {botUuid,platform,chatId,title,kind,state} (packages/projects/src/host/ProjectsHost.tsx:50-68). `ChannelBinding.chat_title` (a server-stored snapshot) and `chat_id`.
- Existing shared helpers: NONE for bot/chat labels. There is no BotLabel, shortId or formatBotName.
  - `platformDisplayName(id,t)` (src/constants/platformGuides.tsx:403) only formats platforms.
  - The only "name + id" helper is the local `rowHeader(title,id)` in src/components/bot/BotAccessDialog.tsx:48-53. It is the best existing reference pattern: bold alias, mono external id underneath, id hidden if it equals the title.
  - Projects has `usePlatformLabel` (packages/projects/src/components/im/PlatformMark.tsx:~8) and `chatKey(platform,chatId)` (useImBindings.ts:13). `chatKey` omits the bot.
- No truncated IDs by `.slice()` on bot/chat ids. The only truncation is CSS ellipsis (BotTable.tsx:183-197, BotChatsButton.tsx:138-151).
- Dead code: `BotChatsButton` (src/components/bot/BotChatsButton.tsx) is not imported anywhere. `/notify` route redirects to `/bots/overview` (src/routes/appRoutes.tsx:198), so `NotifyPage` may be unreachable (UNVERIFIED whether it is still mounted elsewhere).
- Frontend routes and URL params carry NO bot uuid or chat id (only `?platform=`, `?add=1`). API paths use uuid (src/services/botApi.ts, imbotApi.ts), which is correct for APIs.
- Not present in the frontend (verified by grep): scenario bindings by chat_id, usage/log/dashboard/history pages mentioning bot or chat (only the bot count in src/layout/PowerUpsMenu.tsx:136 and per-platform counts in src/layout/useBotPlatformSummary.ts:1-30, no ids).

### 1. Bots page (/bots/overview, platform pages) - BotTable, BotConfigDialog, toasts

Format: file:line | component | entity | shown today | ID also needed? | should show | gap

- src/components/bot/BotTable.tsx:158-160 | Name cell + tooltip | bot | `name || platform` (raw platform id, e.g. "telegram") | n/a (UUID has its own column) | name; fallback should not be the raw platform id (see 5 and D1) | E (duplicate defaults, raw id fallback)
- BotTable.tsx:169-199 | Bot UUID cell | bot | full uuid, mono, ellipsis, tooltip, copy button | yes, this is the reference "ID needed" site | keep | OK (but the only place a bot uuid is visible besides Access dialog)
- BotTable.tsx:203, 275 | Platform chip | platform | raw `bot.platform` id | no | platformDisplayName(platform) | D (ImBotNode uses display name; this uses id)
- BotTable.tsx:274 | mobile card title | bot | `name || platform` | uuid is shown at :283 | same as above | E
- BotTable.tsx:92-94 | toast after copy | bot | "Bot UUID copied" | n/a | fine | OK
- BotTable.tsx:303-306 | delete ConfirmPopover | bot | `Delete "{{name}}"` with `name || platform` | uuid would help when two bots share the default name | name plus short uuid when name is a default/duplicate | E, B
- BotTable.tsx:222, 291 (restart aria-label), 230-239 (edit), 240 (delete) | row action buttons | bot | generic labels, no bot name | no | aria-label should include the bot name (several identical rows) | A (a11y)
- src/hooks/useBotToggle.ts:44-45, 49-51 | toast | bot | "Bot enabled"/"Bot disabled"/"Failed to toggle bot: err", no name | no | include bot name | A
- src/hooks/useBotList.ts:58, 61, 75, 78 | toasts | bot | "Bot restarted", "Bot deleted successfully", failures, no name; load errors :43,47 | no | include bot name (list has several bots) | A
- src/components/bot/BotConfigDialog.tsx:163 | create/update payload | bot | default name written to the DB: `name.trim() || "${platform} Bot"` (e.g. "telegram Bot"). Two bots on one platform get identical stored names | n/a | unique default (platform display name + short id or counter) | E (root cause of duplicate names)
- BotConfigDialog.tsx:198-202 | dialog title | bot | "Edit bot", no name | no | "Edit {name}" | A
- BotConfigDialog.tsx:273-277 | name field | bot | label "Alias", helper "friendly name"; the table column calls the same field "Name" (BotTable.tsx:108) | no | one term across UI | D
- BotConfigDialog.tsx:252 (-> BotAuthForm.tsx:64, FeishuQRAuth.tsx:24, WeixinQRAuth.tsx:24/65/80) | QR flow | bot | `botName={nameDraft || "${platform} Bot"}` sent to the backend as display name | n/a | same default as :163 | D (duplicate literal)
- BotConfigDialog.tsx:194+ (notify at "Bot created/updated successfully.") | toast | bot | no name | no | include name | A
- src/components/bot/useQrPollingSession.ts:14 | temp uuid `temp-<ts>-<rand>` | bot | internal only, never displayed | n/a | n/a | OK
- WeixinQRAuth.tsx:55/221, FeishuQRAuth.tsx:41 etc. | error text | bot | "Bot UUID is required" | internal | n/a | OK
- src/components/bot/PairingCodePanel.tsx:87-95, 108, 110 | pairing code | pairing | toasts "Pairing command copied", "Pairing code rotated", no bot name. The panel itself is inside a bot card, so context exists, but toasts are global | no | include bot name | A (minor)
- PairingCodePanel.tsx:154 | code text | pairing | `/pair <code>` | code is the value, yes | OK

### 2. Remote Control page / card (RemoteAgentPage, RemoteAgentBotCard, RemoteControlGraph, CCProfile*)

- src/components/bot/RemoteAgentBotCard.tsx:132-134 | card header | bot | `name || platform` (raw id fallback); NO uuid anywhere on the card or tooltip; platform shown only as brand icon | yes for support/cross-reference (Remote card has no id; the graph node tooltip has it, see ImBotNode) | name + platform display name; uuid in tooltip/secondary line | B, E
- RemoteAgentBotCard.tsx:221 | delete ConfirmDialog | bot | `Delete "{{name}}"` using `name || platform` | no | name (+ short id if duplicate) | E
- RemoteAgentBotCard.tsx:188-194 | folded off-state line | profile | "Claude Code · `ccProfileName || ccProfileId`" | profile id useful only if profile missing | OK | fallback `name || id` consistent with CCProfileNode
- RemoteAgentBotCard.tsx:107-114 | notice text | chats | "{{count}} direct chats reached this bot..." counts only, no chat names | no | list the pending chats by alias so the user knows which to grant | A (UX)
- src/components/nodes/ImBotNode.tsx:65 | graph node (variant full) | bot | `name || 'Bot'` hard-coded English, italic when empty (:106) | no | shared fallback | E (third distinct fallback)
- ImBotNode.tsx:82 | tooltip title | bot | `name || platformLabel` (display name, differs from `name || platform` elsewhere) | no | consistent | D
- ImBotNode.tsx:86-90 | tooltip | bot | "Bot UUID: {uuid}" | yes | keep; only place on Remote/Notify cards that shows the uuid, and only in a tooltip of the platform node | OK/B
- ImBotNode.tsx:97-104 | variant "platform" | bot | platform icon + platform display name only (bot name omitted by design since the card header names it) | n/a | n/a | OK
- src/components/nodes/CCProfileNode.tsx:39-41 | node label | CC profile | `profileName || profileId`, "Default" if none; missing profile shows raw id with warning (:32, 57) | id when unresolved: yes | OK | fallback chain consistent
- CCProfileNode.tsx:49 | tooltip | CC profile | "Claude Code profile: {name} ({id})" | yes (both) | keep; the model for name+id | OK
- src/components/bot/CCProfileDialog.tsx:61-69 | dialog title | bot | static "Claude Code profile", no bot name; user cannot tell which bot is being edited when opened from the Bots overview dialog | no | include bot name | A
- CCProfileDialog.tsx:95-98 | list rows | profile | primary `p.name`, secondary `claude_code:<id>` | yes | OK (name + id) | OK
- src/pages/remote-agent/RemoteAgentPage.tsx:85-86, 91, 98 | toasts | bot | "Remote Control enabled/disabled", failure text, no bot name (page lists many bots) | no | include name | A
- RemoteAgentPage.tsx:117, 120 | toast | bot/profile | "Claude Code profile updated" with neither bot nor profile | no | "{bot}: profile set to {profile}" | A
- src/pages/bots/BotOverviewPage.tsx:226 | capability dialog title | bot | "IM Notify" / "Remote Control" only; the inner card header shows `name || platform` | no | title could include bot | A (minor)
- BotOverviewPage.tsx:237 | inline profile updater | bot | thrown error 'Failed to update profile', no success toast | no | n/a | n/a
- src/components/bot/RemoteControlGraph.tsx:35-41 | graph | bot, profile | passes `imbot`; profile name resolved from `ccProfiles` | n/a | n/a | OK

### 3. IM Notify surface (BotNotifyGroup, ChatNode, NotifyTestDialog, ApiEntryNode, NotifyGuide)

- src/components/notify/BotNotifyGroup.tsx:137-146 | direct-chat target mapping | direct chat | builds `NotifyTarget` from `detail.chat` WITHOUT `name` and without `alias`; `name` is only set for groups (:151, `group.name`, not `group.alias`) | alias is the whole point | map `alias || peer display || external_chat_id` | A (alias dropped; the alias a user sets in Access dialog never appears on Notify)
- BotNotifyGroup.tsx:147-158 | group target mapping | group | `name: group.name` only; `group.alias` ignored (Access dialog uses `alias || name || external`) | no | `alias || name || external` | A, D
- BotNotifyGroup.tsx:314 | card header | bot | `name || platform` | uuid not shown | see section 2 | E, B
- BotNotifyGroup.tsx:379 | ApiEntryNode path | bot | `/api/v1/bots/${bot.uuid}/notify` (tooltip only, ApiEntryNode.tsx:23-33 shows `POST /notify` on the node) | yes, copyable curl target | OK, but not copyable from the tooltip | OK
- BotNotifyGroup.tsx:388 | ImBotNode variant platform | bot | platform only | n/a | n/a | OK
- BotNotifyGroup.tsx:429-437 -> src/components/nodes/ChatNode.tsx:60 | chat leaf label | chat | `name || chatID` where chatID = `external_id` (the raw platform id, e.g. Telegram numeric id), mono font; for direct chats `name` is always empty so it is ID-only | ID must remain reachable; alias should be primary | alias bold, external id in tooltip/secondary | A
- ChatNode.tsx:41-42 | tooltip | chat | "Chat ID: {external}" / "Group ID: ..." plus "Target UUID: {internal}" | yes (both ids) | keep; tooltip is the only place the internal id is visible | OK
- BotNotifyGroup.tsx:512-516 (+ handleCopy :181-188) | copy button | chat | copies the INTERNAL `target.id`; tooltip "Copy internal target UUID"; toast "Target UUID copied" (src/i18n/locales/en/remote.ts:360, 368) | yes | OK, but no way to copy the external chat id here | B (external id not copyable)
- BotNotifyGroup.tsx:517-533, 534-548 | disable/delete buttons | chat | generic aria-labels ("Disable chat", "Delete Direct Chat"), no chat name | no | include chat name | A (a11y)
- BotNotifyGroup.tsx:212-214 | toast | chat | "Target blocked"/"Target unblocked", no name | no | include chat name | A
- BotNotifyGroup.tsx:233 | toast | chat | "Chat deleted", no name | no | include chat name | A
- BotNotifyGroup.tsx:588-606 | delete ConfirmDialog | chat | body shows `{deleteTarget}` in mono (:593), which is the INTERNAL uuid (target.id), not alias nor external chat id | user cannot recognise the chat | alias + external id | WRONG FIELD
- BotNotifyGroup.tsx:208, 228, 262 | error toasts | chat | raw `Error.message` | no | prefix chat name | A
- BotNotifyGroup.tsx:243, 260 + src/components/notify/useChatProbe.ts:40, 127-139 | probe runner | chat | keyed by internal id; result chip row sits inside the chat row so no name needed | no | n/a | OK
- BotNotifyGroup.tsx:399-400 | empty text | platform | `platform: bot.platform || 'its platform'` raw platform id in the sentence | no | platformDisplayName | D
- BotNotifyGroup.tsx:608-615 -> src/components/notify/NotifyTestDialog.tsx:99 | dialog title | bot | "Send a test notification - {botName}", botName = `bot.name || bot.platform` (:611) | no | OK | E (fallback)
- NotifyTestDialog.tsx:108 | Autocomplete getOptionLabel / selected value | chat | `target.name || target.external_id` | no | alias | A (as above), D (differs from option row)
- NotifyTestDialog.tsx:117-119 | option row | chat | ALWAYS `external_id` in mono; ignores `target.name`, so a named group shows its raw id in the list but its name in the input | no | same label as :108 plus external id secondary | D
- NotifyTestDialog.tsx:77, 88, 91 | toasts | chat | "Select a discovered chat first", "Send failed: err", "Notification sent" - no target name | no | include target | A
- src/components/notify/NotifyGuide.tsx:32-46 | curl example | bot/chat | `<BOT_UUID>`, `<TARGET_UUID>` placeholders; origin from window | yes | OK | OK
- src/i18n/locales/en/remote.ts:400 | guide text | chat | says "Each Chat node ... shows the real platform Chat ID for recognition", which is no longer true for named groups (ChatNode shows name first) | n/a | update copy | stale text
- src/components/notify/useChatProbe.ts result lines (BotNotifyGroup.tsx:58-63) | verdict | chat | "Delivered / Failed: err", inline under the row | no | OK | OK
- src/pages/notify/NotifyPage.tsx:62 | toast | bot | error message only; renders `BotNotifyGroup` per bot (:97-103) | no | n/a | possibly dead page (route redirects)

### 4. Access dialog (BotAccessDialog, used in Bots dialog, Remote card inline, and Projects IM settings via imHost)

- src/components/bot/BotAccessDialog.tsx:168-171 | direct chat row header | direct chat | `rowHeader(alias || external_chat_id, external_chat_id)`: alias bold + external id mono underneath (id hidden when no alias) | yes (shown) | this is the best current pattern; still lacks internal id and peer actor display name | OK; fallback skips actor name (E)
- BotAccessDialog.tsx:311 | group row header | group | `rowHeader(alias || name || external_group_id, external_group_id)` | yes | OK | D (differs from direct chat rule: group uses `name`, direct has none)
- BotAccessDialog.tsx:149-166 | Rename dialog | chat/group | title "Rename", field "Alias", helper "Leave empty to use the platform name or ID." Does not name the chat being renamed (inline mode shows it under the row, dialog mode does not) | yes (to confirm which chat) | include current label + id in the dialog | A, B
- BotAccessDialog.tsx:181-182, 226 | Rename button | chat | generic "Rename" | no | aria-label with chat | A (a11y)
- BotAccessDialog.tsx:325 | actor chips | actor | `actor.display_name || actor.external_actor_id`; `label` ignored | external actor id is useful on hover | tooltip with external id | D (minor)
- BotAccessDialog.tsx:287 + src/services/botApi.ts:158-163 | add actor | actor | external actor id used both as `{actor}` path id and external_actor_id; display name optional | n/a | n/a | OK
- BotAccessDialog.tsx:263 | add-actor dialog title | group | "Add authorized actor", no group name | no | include group label | A
- BotAccessDialog.tsx:342, 346-349 | dialog title | bot | `Who can control {{name}}` / `{{name}} access` with `bot.name || bot.platform || ''` (empty string if bot null) | no | name | E
- BotAccessDialog.tsx:366-367 | Bot UUID line | bot | full uuid mono (only in scope all) | yes | OK | OK
- BotAccessDialog.tsx:390-410 | headings | n/a | "Direct Chats", "Groups" + hint "concrete platform chat ID" | n/a | OK
- BotAccessDialog.tsx:243-246 (block switch), 197-201 (toggle) | controls | chat | no chat name in aria-labels | no | add | A (a11y)
- Errors (:126, 105) | raw `Error.message` | n/a | no entity | no | n/a | n/a

### 5. Projects package (frontend/packages/projects) and the host adapter

Host adapter: src/pages/projects/imHost.tsx

- src/pages/projects/imHost.tsx:34 | listBots | bot | `ImBot.name = b.name || platformLabel(b.platform)` (translated display name; other sites use raw platform id) | uuid carried in `ImBot.uuid` but not displayed | name; unique default | D, E
- imHost.tsx:48 | listChats direct | direct chat | `title = "<Platform> · <alias.trim() || bareId(external_chat_id)>"`. `bareId` strips a "platform:" prefix; actor display name unused; internal chat id lost | external id shown only when no alias | alias; id as secondary | E (only alias fallback), D
- imHost.tsx:60 | listChats group | group | `alias.trim() || bareId(external_group_id)`; ignores `group.name` (Access dialog uses `alias || name || external`) | same | `alias || name || id` | D, A
- imHost.tsx:18-23, 26 | platform label | platform | hard-coded `PLATFORM_NAMES` + `layout.platforms.*` i18n; third platform-name source (others: guides `platformDisplayName`, raw id) | n/a | one source | D
- imHost.tsx (ImHost literal :75-84) | `chatTitle` | chat | NOT implemented (type has optional `chatTitle` at packages/projects/src/host/ProjectsHost.tsx:82) | n/a | implement via alias map | C (see ThreadPanel below)
- imHost.tsx:14-16, 69-73 | AccessDialog | bot | passes cached `BotSettings` | n/a | n/a | OK

Binding list / tags (project and workdir headers)

- packages/projects/src/components/im/useImBindings.ts:58 | bind call | chat | `bindChannel({platform, chat_id, bot_uuid, ...target})` - does NOT send `chat_title` (server field exists: projects/internal/server/module/channel/handler.go:33; schema chat_title optional) | n/a | send the resolved alias at bind time | C: bindings created from the UI carry an empty `chat_title`
- packages/projects/src/components/im/ProjectImBindings.tsx:107, 112, 116-118, 138-140 | Tag / MoreTags | chat | `chat_title || chat_id`, i.e. raw external chat id when title empty (always for UI-made bindings), snapshot never refreshed after an alias rename | id useful as tooltip | live alias lookup | E, C
- ProjectImBindings.tsx:117-118, 138 | unbind aria-label/tooltip | chat | "Unbind {{chat}}" with the same fallback | no | OK once title is right | E
- ProjectImBindings.tsx:65 | toast bind/move | chat, project | `chat.title` (ImChat.title incl. platform prefix) and project name | no | OK | D (title format differs from binding tag `chat_title`)
- ProjectImBindings.tsx:75 | toast unbind | chat | `b.chat_title || b.chat_id` | no | OK once fixed | E
- ProjectImBindings.tsx:86 | `chatTitle(platform,chatId)` | chat | binding snapshot lookup via `chatKey(platform, chatId)` - bot not part of the key, two bots on the same platform seeing the same chat id collide | n/a | key by bot too | C
- packages/projects/src/components/im/useImBindings.ts:13 | chatKey | chat | `platform\0chatId` (no bot) | n/a | include bot_uuid | C (also used as React key at src/pages/ImSettingsPage.tsx:121)

Bind / move dialogs

- packages/projects/src/components/im/BindChatDialog.tsx:67 | option title | chat | `c.title` (`Platform · alias|id`) | external id only if no alias | alias plus external id secondary | E
- BindChatDialog.tsx:68 | option subtitle | chat, bot | `platform · bots.find(uuid)?.name · kind · state|where` | bot uuid not shown, bot name may be duplicate "telegram Bot" for two bots | bot name (+ short id if duplicated) | E (duplicate names), A
- BindChatDialog.tsx:63 | "Bound to X" | project | project/folder name or "another project" when unknown directory | n/a | n/a | C (unresolved shows generic text)
- BindChatDialog.tsx:91, 105 | dialog title/aria | project | "Bind a chat to {{project}}" | n/a | OK | OK
- BindChatDialog.tsx:54, 75 | key | chat+bot | `chatKey\0botUuid` - correct there | n/a | n/a | OK
- packages/projects/src/components/im/MoveChatDialog.tsx:56 | dialog title | chat | `Move {{chat}} to` with `chat.title` | no | OK | E
- MoveChatDialog.tsx:39-40 | project rows | project | name + Project/Folder | n/a | OK | OK

IM settings page (src/pages/ImSettingsPage.tsx)

- ImSettingsPage.tsx:87 | bot section header | bot | `bot.name` only (+ platform label and Connected/Not running at :88-91); no uuid | yes (support) | name + uuid tooltip | B
- ImSettingsPage.tsx:123-124 | chat row | chat | `c.title` (platform-prefixed, redundant under a bot header already showing platform) + kind; the external chat id is never visible when an alias exists | yes for cross-reference with IM-side messages | alias + id | B, D
- ImSettingsPage.tsx:129 | Bound-to cell | project | `place?.name ?? "another project"` | n/a | n/a | C
- ImSettingsPage.tsx:135 | bound session | thread | `#seq · target_title || target_thread_id` (raw thread id fallback) | n/a | n/a | E (minor)
- ImSettingsPage.tsx:138 | focus line | thread | `focus_title || thread_id` | n/a | n/a | E (minor)
- ImSettingsPage.tsx:48 | toast move | chat, project | `chat.title`, project name or '' | no | OK | OK
- ImSettingsPage.tsx:151 | aria-label | chat | "Move {{chat}} ..." uses `c.title` | no | OK | OK
- ImSettingsPage.tsx:94 | Access button | bot | generic label per bot section | no | OK | OK

Message source badges (thread transcripts)

- packages/projects/src/components/im/ImSourceBadge.tsx:13 | "From {{platform}} · {{chat}}" | chat | chat string passed in by callers | n/a | n/a | see below
- packages/projects/src/components/project/ConversationPane.tsx:356 (type at :284-285, wired from src/pages/ProjectPage.tsx:62/435) | project conversation | chat | `ProjectIm.chatTitle(platform, source_chat_id) ?? source_chat_id`: binding snapshot (usually empty, see useImBindings.ts:58) else RAW chat id; comment at :284 confirms "unknown chats show their id" | id useful as secondary | live alias | C, E
- packages/projects/src/components/project/ThreadPanel.tsx:106, 480 | thread transcript | chat | `useProjectsHost().im?.chatTitle?.(...) ?? imSource.chatId`, i.e. the HOST's `chatTitle`, which imHost.tsx never defines -> always the RAW chat id. Same data shown with a different resolver than ConversationPane (:356) | same | same resolver in both | C, D (two resolvers, one dead)
- ImSourceBadge.tsx:13 | no bot | bot | `im.sourceBot` "From bot" when platform empty; bot name/uuid never shown on message source | n/a | n/a | n/a
- packages/projects/src/components/project/threadTranscript.ts:65 | stores `{platform, chatId}` only | chat | bot uuid not carried | n/a | n/a | B (minor)
- packages/projects/src/pages/WorkdirPage.tsx:223, 328, 346 | header tags | chat | `ProjectImTags` (see above) | n/a | n/a | E
- packages/projects/src/i18n/en.ts:59-61 (and zh.ts:63-65) | strings | chat | 'From {{platform}} · {{chat}}', 'From {{platform}}', 'From bot' | n/a | n/a | OK

No bot/chat sites found in packages/ui, packages/mannequin, packages/vision.

### 6. Fallback / format patterns found (for unification)

Bot display name
- P1 `bot.name || bot.platform` (raw platform id, e.g. "telegram"): 9 sites in 4 files. BotTable.tsx:158,160,274,305; RemoteAgentBotCard.tsx:133,221; BotNotifyGroup.tsx:314,611; BotAccessDialog.tsx:342 (`|| ''`).
- P2 `name || platformLabel(platform)` (localized display name): 2 sites. imHost.tsx:34 (feeds all Projects IM UI), ImBotNode.tsx:82.
- P3 `name || 'Bot'`, hard-coded English, italic: 1 site. ImBotNode.tsx:65/106.
- P4 `name` only, no fallback: ImSettingsPage.tsx:87; BindChatDialog.tsx:68 (`?.name`, hidden if missing).
- P5 name is WRITTEN as `"${platform} Bot"` when the user leaves it empty (BotConfigDialog.tsx:163 and :252), using the raw platform id (e.g. "telegram Bot"). Two bots on one platform therefore get identical stored names. This is what makes P1/P2 fallbacks rarely trigger and makes names non-unique.
- The same field is labelled "Name" (BotTable.tsx:108), "Alias" (BotConfigDialog.tsx:274) and `name` (code); the chat/group field is "Alias" (BotAccessDialog.tsx:153).
- Bot uuid is visible in only 3 places: BotTable.tsx:196 (and :283 mobile), BotAccessDialog.tsx:367, ImBotNode.tsx:88 (tooltip). Not visible on Remote card, Notify card header, Projects IM settings, Bind dialog, any toast/confirm.

Chat / group display name (direct chat has alias; group has alias and name)
- C1 `alias || external_chat_id` + external id as secondary line (rowHeader): BotAccessDialog.tsx:169-170. Best current behaviour.
- C2 `alias || name || external_group_id` + id secondary: BotAccessDialog.tsx:311.
- C3 `"<Platform> · " + (alias.trim() || bareId(external_id))` for direct, `alias.trim() || bareId(...)` for group (no `name`): imHost.tsx:48,60. Platform prefix in the title, id never shown when alias exists.
- C4 `target.name || target.external_id` where `name` only ever set from group.name (not alias, not for direct): BotNotifyGroup.tsx:137-151 -> ChatNode.tsx:60, NotifyTestDialog.tsx:108; option row uses `external_id` only (NotifyTestDialog.tsx:118).
- C5 `chat_id` raw (BotChat, BotChatsButton.tsx:150, dead code).
- C6 binding: `chat_title || chat_id` (ProjectImBindings.tsx:75,107,86) and `chatTitle?.(...) ?? source_chat_id` (ConversationPane.tsx:356) and host `im?.chatTitle?.(...) ?? imSource.chatId` (ThreadPanel.tsx:480). `chat_title` is never sent by the frontend (useImBindings.ts:58).
- C7 internal uuid shown to user: BotNotifyGroup.tsx:593 (delete confirm). Internal uuid also copied by design at BotNotifyGroup.tsx:513 and in ChatNode tooltip.
- Result: the same chat has up to 5 different labels across Access dialog (C1), Notify graph (C4), Projects bind list (C3), project tags (C6), transcript source line (C6).

Actor
- A1 `display_name || external_actor_id`: BotAccessDialog.tsx:325 (only site; `label` never displayed; direct chats never show the peer actor).

Claude Code profile
- F1 `profileName || profileId` (+ "Default" when none): CCProfileNode.tsx:40, RemoteAgentBotCard.tsx:192. Tooltip "name (id)" CCProfileNode.tsx:49, dialog rows name + `claude_code:<id>` CCProfileDialog.tsx:96-97. Consistent.

Platform name
- L1 raw id: BotTable.tsx:203,275; BotNotifyGroup.tsx:399-400; BotChatsButton.tsx:111 etc.
- L2 `platformDisplayName(id,t)` via platform guides: ImBotNode.tsx:64, BotOverviewPage.tsx:93,149.
- L3 `layout.platforms.<id>` i18n with hard-coded `PLATFORM_NAMES`: imHost.tsx:18-23.
- L4 capitalised id (Projects fallback): PlatformMark.tsx.

Toast / confirm / aria patterns
- T1 toasts naming no entity: useBotList.ts (58,61,75,78), useBotToggle.ts (44,45,49), RemoteAgentPage.tsx (85,86,117,120), BotConfigDialog.tsx (created/updated), PairingCodePanel.tsx (91,108), BotNotifyGroup.tsx (212-214, 233), NotifyTestDialog.tsx (88,91). About 20 strings.
- T2 toasts/confirms that do name the entity: Projects bind/move/unbind (ProjectImBindings.tsx:65,75; ImSettingsPage.tsx:48), delete-bot confirm (BotTable.tsx:305, RemoteAgentBotCard.tsx:221, both P1).
- No chat id/alias in any error message; errors are raw `Error.message`.
- Icon-only buttons in repeated rows (restart/edit/delete bot, disable/delete/copy chat, Rename) have generic aria-labels with no entity name.

### 7. UNVERIFIED / caveats
- Whether bindings created IM-side populate `chat_title` (Go remote side, other investigator). The frontend never sends it, so UI-created bindings show raw external chat ids in tags and "From ..." lines. I confirmed server accepts `chat_title` (projects/internal/server/module/channel/handler.go:33, db/channels.go:94,161).
- Whether `NotifyPage` / `BotChatsButton` are reachable (BotChatsButton: no importers; NotifyPage: route `/notify` redirects to `/bots/overview`).
- Peer actor display name for direct chats: the DirectChat DTO (src/types/bot.ts:35) has only `peer_actor_id`; whether an actor name could be joined is a backend question.
- Mock handlers (src/mocks/*) not audited.
- Rendered output for 0-length `name` (backend may always store "<platform> Bot", so blank/undefined is not observed; only the italic 'Bot' at ImBotNode.tsx:106 handles empty explicitly).

---

# Part C — Non-frontend emitters (IM messages, logs, errors, CLI, persistence)

Scope: remote/**, imbot/**, internal/command, internal/server (+module/imbot, notify, projects), internal/db, projects/**.
Read-only; paths relative to repo root. UNVERIFIED marks anything not confirmed by reading code.

Gap classes: A = alias/name missing, I = ID missing where needed, S = stale/unresolved, F = inconsistent format/key, W = wrong field / ID spaces mixed.

### 0. Ground truth: entities and where names live

| Entity | ID | Human name | Notes |
|---|---|---|---|
| Bot | `BotSetting.UUID` (remote/control/bot/chat_store.go:11) | `BotSetting.Name` (:12): optional via API, NOT unique, cannot be cleared by update (internal/db/imbot_settings_store.go:181); CLI `bot add` forces non-empty (internal/command/bot_add.go:202-210) | imbot package has NO name: `core.Config` = {UUID, Platform, Enabled, Auth, Options, Logging} (imbot/core/config.go:12-19), so every adapter log line is nameless |
| IM chat (legacy store) | `Chat.ChatID` = external platform id (chat_store.go:75) | none | `notify.ChatSummary` (internal/server/module/notify/bot_api.go:75-84) has no name/alias |
| IM chat (access store) | `DirectChat.ID` / `Group.ID` = internal uuid; `ExternalChatID`/`ExternalGroupID` = platform id (remote/access/resources.go:30-50) | `DirectChat.Alias` (:34), `Group.Alias` (:48), `Group.Name` (:49), `Actor.DisplayName` (:23), `GroupActor.Label` (:66) | Alias is written only by PUT .../alias (internal/server/module/imbot/access_api.go:564-579) and is READ by no backend log, error or IM message |
| Route | `Route.ID` | `Route.Name` (resources.go:75) | never emitted: runtime audit uses route_id only (remote/scenario/runtime_default.go:77) |
| Channel (runtime) | `imchannel.Channel.ID()` = bot UUID (remote/control/bot/manager.go:212) | none | logs show `channel=<uuid>` |
| Projects binding | `ChannelBinding.ID`, `Platform`+`ChatID`, `BotUUID` (projects/internal/db/models.go:248-252) | `ChatTitle` (:254) | every chat-originated caller passes "" (internal/server/module/projects/chatbridge.go:118; projects/projects.go:228), so chat_title is effectively always empty |
| Claude Code profile | `BotSetting.DefaultAgent` "claude_code:<id>" (chat_store.go:26) -> `botProfile()` (remote/control/remoteagent/project_route.go:145) | profile name | never emitted to a chat or log |

Two chat-id namespaces are mixed: `chat_id` in logs usually = external platform id (remoteagent, projects relay, notify), but `target_id`/`actor_id` in authorization logs = INTERNAL access uuids (remote/control/bot/authorization.go:124; remote/scenario/runtime_default.go:73). No log line carries both.

### 1. Messages the bot SENDS into IM chats (audience: end user)

Format: file:line | entity | emitted today | should emit | gap

- remote/control/remoteagent/project_help.go:8-28 (`/help`, alias `/start`, command.go:137) | sender+chat | "Your ID: <senderID>", "Chat ID: <chatID>" raw ids; no bot name, no chat alias, no profile | KEEP both IDs (help text says the notify API asks for them, :5-7); add a "Bot: <name>" header line and chat alias if set | A
- remote/control/remoteagent/handler_message.go:185 | group | "...DM the bot with `/join <ChatID>` to enable." | ID required (user pastes it); add group title beside it | A (ID correct)
- remote/control/remoteagent/handler_message.go:146 | chat | "Unsupported message from upstream <ChatType> <ChatID>." | do not expose internals to user; alias if anything | F
- remote/control/remoteagent/handler_pair.go:29-31 `pairingHintMessage`; :196 handler_message.go (group operator unpaired) | bot | "This bot requires pairing..." never says which bot | bot name | A
- remote/control/remoteagent/command.go:330,333 (`/pair`, alias `/bind`) | bot | "Paired. You can now send commands to this bot." | bot name in confirmation | A
- remote/control/bot/manager.go:140 | bot | "You are not authorized to answer this request. Ask the bot owner..." | bot name | A
- remote/control/remoteagent/command.go:291,301 (`/join`) | group | "Group <groupID> is already in whitelist." / "Group ID: <groupID>" | title + ID (ID useful for notify API) | A
- remote/control/remoteagent/handler_bind.go:79, callback.go:91,118 | folder | path only | fine | -
- projects/internal/project/chat_commands.go:193-224 `/status` | chat/session | Folder / Project: <p.Name> / Session: [#2 Title], status / Model / Permissions / Verbose. NO bot name, NO Claude Code profile (Binding.Profile, projects/internal/db/models.go:271, not read), NO chat alias | add Bot and Profile lines (this command exists to say "where this chat is") | A
- projects/internal/project/chat_commands.go:103,110,118,178,183,214,234 | thread/project | `[#<seq> <title>]` tag + project name / folder name | good pattern: name first, `#N` is the handle the user types (`@N`, `/session N`) | -
- projects/internal/project/relay.go:408,420,430 | thread | every relayed message suffixed `\n\n[<tag>]` (tag = `#seq title`, fallback project name); prompts prefixed `[tag] ` | good; bot name not needed (IM shows sender) | -
- projects/internal/project/relay.go:306-322 `foldThreadUpdate` | thread | "Needs you: ... Reply with @<seq>" | good | -
- internal/server/module/projects/chatbridge.go:253,347,372,405 | bot | ErrNoChannel text "no running bot for this chat", "...: bot <uuid> stopped", "...: bot <uuid>", "...: <platform>" | raw UUID in an error string; whether any reaches a chat is UNVERIFIED (relay path only logs them) | I/A
- remote/control/remoteagent/project_route.go:120,188,246,282,318,337 | chat/project | "Could not move this chat to <folder>", "Could not send this to the project: <err>" | fine as long as wrapped errors carry no UUIDs | -
- remote/channel/imchannel/imchannel.go:105-125 `Send` (notify push) and remote/scenario/builtin/claudecode/plugin.go:99-101 | route/scenario | Title "Claude Code · <cwd tail>", body; no route name, no bot name | route name in title for multi-route chats | A
- remote/channel/imchannel/imprompter.go:443; remote/control/ask/tool_handlers.go:61-76,256 | permission prompt | "Tool Permission Request / Tool: x"; no session/bot label (Projects prompts do prefix `[tag]`, relay.go:430) | session tag | A (legacy path)

#### How the bot's own name is used in user-facing text
- Never. No IM message from remote/**, imbot/** or projects/** interpolates `BotSetting.Name`. User-facing references are the generic "the bot"/"this bot" (handler_pair.go:29,84; command.go:333; handler_message.go:185,196; manager.go:140).
- Profile/DefaultAgent is only an input to Projects (project_route.go:137-141), never shown. Platform display names ("Feishu") are not shown to users.
- Project and thread names ARE shown (relay.go, chat_commands.go): that is the pattern to copy.
- Only operator-facing stderr banner names the bot: remote/control/bot/manager.go:179-183 `Bot %q (%s) pairing code: %s` = name + platform, no UUID.

### 2. Logs and errors in remote/** (audience: developer/operator grepping logs)

Rule of thumb for logs: ID (greppable) plus name. Today only 7 bot log sites carry the name; no log carries chat alias.

#### 2a. remote/control/bot (lifecycle)
- manager.go:172-181 pairing code Warn | bot | uuid+name+platform fields; message has code only | both | ok (best site) | -
- manager.go:179-183 stderr banner | bot | name+platform, no UUID; duplicate or empty Name => `Bot "" (telegram)` ambiguous; `bot pair status` (internal/command/bot_pair.go:100-103) tells the operator to grep exactly this line | add UUID | I
- manager.go:467 "Bot started" | bot | uuid+name+platform | both | ok
- manager.go:444 "Bot has no active mount" | bot | uuid+name+platform | ok
- manager.go:502-506 panic | bot | uuid+name+platform | ok
- manager.go:402,405 Debug | bot | uuid only | add name | A
- manager.go:426-431 missing credentials Warn + returned error | bot | Warn: uuid+platform; error text: "bot for platform %s is missing required credentials" (no id; error bubbles to UI/CLI) | add name+uuid | A/I
- manager.go:89 `failed to start %s bot: %w` | bot | platform only | uuid+name | I
- manager.go:232,234 menu button | bot | platform only | uuid+name | I
- manager.go:403 error "bot is stopping, please try again later" | bot | none | name | A
- manager.go:514,516,519,528,583,596,627,636 | bot | `uuid` only ("Bot stopped", "Stopping bot", "Failed to start bot") | add name | A
- manager.go:555 stop timeout | bot | uuid only | add name | A
- authorization.go:81,88,95,109 Warn "bot access: resolve actor/group/direct chat/establish direct peer" | bot, chat, actor | err only: NO bot_uuid, NO chat, NO actor | add bot_uuid+external chat id | I
- authorization.go:124 decision fields (Debug, or Warn when a pending-prompt reply is denied :130) | bot, chat, actor | `bot_uuid`, `target_kind`, `target_id` (internal access uuid), `actor_id` (internal uuid), reason; no external chat id, no alias/name | add external chat id + alias, actor display name | W/A
- prompt_reply.go:35 and handler_message.go:37 `chat %s is disabled, dropping message` (Debug, duplicated gate) | chat | `chat %s` external id; no bot | add bot + alias | I/A
- prompt_reply.go:123-216,282-309 permission/option callbacks | request | `request_id`, `user_id`, `tool_name`; NO bot, NO chat | add bot_uuid+chat | I
- prompt_reply.go:367 weixin context token | bot, chat | `bot_uuid`, `target` (key name for chat id differs) | rename key | F
- internal/db/remote_chat_store.go:139,161,215,241,285,308,419,461 errors | chat | `get chat %s`, `chat %q belongs to platform %q, not %q` (mixes %s and %q for the same id), `chat_id and bot_uuid are required` | consistent quoting | F
- internal/db/imbot_settings_store.go:176,234,248,276 | bot | "imbot settings with uuid %s not found" (uuid only is right for not-found; no name available) | ok | -

#### 2b. remote/control/remoteagent
- handler_message.go:139,142 `Chat ID: %s` / `Group chat ID: %s` (Info per message!) | chat | bare id, no bot, no label, no sender | include bot uuid/name + sender display name; consider Debug | F/A
- handler_message.go:167-173,189-195 unpaired rejection Warn | bot, chat, sender | `user_id`, `bot_uuid`, `chat_id`, `platform` fields | good ids; add bot name + sender display name | A
- handler_message.go:184 `Group %s is not whitelisted` | group | id only | + group title | A
- handler_pair.go:87-106 `imbot.pair.fail/success` | bot, chat, sender | `user_id`, `bot_uuid`, `chat_id`, `platform` | add bot name + chat alias (this is the audit trail of who paired) | A
- handler_bind.go:41 `Bind flow started for chat %s`; :78 `Chat moved to a folder: chat=%s path=%s`; :76 field `chatID` | chat | three different shapes | unify | F
- handler_send.go:15 field `chatID` ("SendText: no bot available"), :26 `Failed to send message` | chat | :26 has NO chat/bot at all | add both | I
- project_route.go:104,107,187,317,336 field `chatID` (camelCase) | chat | external id; no bot | add bot_uuid | I/F
- callback.go:216 `chatID`+`messageID` fields | chat | | | F
- command.go:300 `Group %s added to whitelist by %s` | group, sender | ids only | + group title, sender name | A
- command.go:248,285,296 | chat | err only | add chat/bot | I
- command_dispatch.go:37 `Command handler failed` | command | `command` only; no bot/chat | add | I
- callback.go:70,79,105,185,207; handler_bind.go:35,46; command_dispatch.go:104 | chat | errors with no chat/bot context | add | I

#### 2c. remote/channel, scenario, interaction, binding
- channel/imchannel/imprompter.go:82 `No chat context for request` | request | `id` only | add bot | I
- imprompter.go:113-117 `Bot not found for request, auto-approving` (Warn) | bot | `id`, `platform`, `botUUID` (camelCase key) | key `bot_uuid`; auto-approving a permission because the bot vanished is a security-relevant line: add chat + name | F/A
- imprompter.go:185-189 `Prompt sent, waiting` | chat | `id`, `chat_id`, tool | add bot | I
- imprompter.go:174,276-292 | request | request id only | | -
- channel/autochannel/autochannel.go:73,89 audit | chat | `chat_id` | | -
- scenario/runtime_default.go:69-78 `bot.authorization.decision` audit | bot, target, route | `bot_uuid`, `target_kind`, `target_id` (internal), `route_id`; NO route name, NO bot name | add route name + external chat | W/A
- scenario/runtime_default.go:132-135 `notify push failed` | channel | `channel` (= bot uuid under key "channel"), `platform` | key `bot_uuid` + name | F
- scenario/builtin/claudecode/plugin.go:107-111,132-139,161-164,177-180 audits | channel | `channel`=ch.ID() (bot uuid), `platform`; no chat target, no route | | F/I
- Audit sink is just logrus Info (internal/server/server_control.go:117-121): "audit" events are NOT persisted anywhere. Nothing to join later. Same for internal/server/module/imbot/handler_pairing.go:29-37 (`bot_uuid`, `user_id`, `client_ip`) and notify/bot_api.go:581-583 (`bot` key).
- remote/binding/binding.go:142,178,202,218 | n/a (no ids in messages) | -

### 3. imbot/** (adapters and manager; audience: developer)

Structural gap: `BaseBot.Logger()` is `core.NewLogger(config.Logging)` with the fixed prefix `[imbot]` (imbot/core/logger.go:60-70, core/base.go:45). The prefix never contains UUID, platform or name. 85 `Logger()` calls in platform/** (feishu 37, telegram 11, weixin 11, discord 6, wecom 6, dingtalk 4, whatsapp 4, slack 3, tingly 2) + core/manager. With two Feishu bots, "Feishu bot connected" cannot be attributed. Only the contained-panic log stamps identity: core/base.go:473 `[%s/%s] panic in %s` = platform/UUID (the model to generalise: one `[platform/uuid name]` prefix applied once in the logger, not per call site). `core.Config` has no Name, so a name must be added there (or a logger wrapper injected from remote/control/bot).

- imbot/manager.go:141 `Added %s bot (UUID: %s)` | platform + uuid | format A
- imbot/manager.go:170 `Removed %s bot [%s]` | PlatformInfo().Name + uuid in brackets | format B
- imbot/manager.go:249 `Error disconnecting %s bot: %s %v` | name + bare uuid | format C
- imbot/manager.go:136,212 `Failed to connect %s bot: %v` | platform only, though uuid is in scope | I
- imbot/manager.go:166 `Error closing bot: %v` | no id at all | I
- imbot/manager.go:460,479,488,493,498 reconnect loop (`Reconnecting %s bot (attempt %d/%d)`, `%s bot reconnected successfully`, `%s bot failed to reconnect`) | platform only; `bot.UUID()` is available in handleReconnect | I
- imbot/manager.go:453 `[%s/%s] panic in %s handler` | platform/uuid (but emitPlatformHandlers :445 passes botUUID "" so it prints `[telegram/]`) | I
- imbot/platform/feishu/feishu.go:180,185,200,238-266 | `%s bot connected` with `b.domain` | platform only; lines 273,397 log whole event `%+v` and message ids | I
- imbot/platform/feishu/feishu.go:325,340,306 Debug `Chat ID: %s, Type: %s`, `Sender ID: %s, Sender UserID: %s`, `Message ID: %s` | ids with no bot | I
- imbot/platform/feishu/feishu.go:429,489,501 Debug `target=%s` | chat | I
- imbot/platform/feishu/card_callback.go:57 `Card action arrived without a chat ID` | no bot | I
- imbot/platform/telegram/telegram.go:140 `Telegram bot connected: @%s`; discord.go:91 `Discord bot connected: %s` (username); slack.go:63 (`authResp.User`); wecom.go:81 `botID=%s`; weixin.go:129,403 `account=%s`; whatsapp.go:144 `phoneID=%s` | platform-side account identity (useful: it is the identity the end user knows) but never joined to the tingly bot name/uuid; a third meaning of "bot id" | F
- imbot/platform/telegram/telegram.go:185 `Received callback query from %d` | user id | I
- imbot/platform/weixin/weixin.go:443 `Handling message: ID=%s, From=%s, To=%s, Text=%s` at Info: logs full message text and both ids for every message (PII at Info) | consider Debug | F
- imbot/platform/weixin/pairing.go:87,148 `Weixin pairing started for account: %s` | "pairing" here = QR login, a different concept from the TOFU `/pair` code (imbot/core/pairing.go) | name collision | F

#### Sender / recipient display-name handling (affects every downstream label)
- imbot/platform/feishu/adapter.go:72 `DisplayName: detail.SenderID // In production, fetch user info` -> display name = raw open_id. Used only on the webhook path (feishu.go:576 `AdaptWebhook`). The WebSocket path builds the message itself with `WithSender(senderID, "", "")` (feishu.go:386) so DisplayName is "" there. Both: no real name. S/A
- imbot/platform/weixin/adapter.go:55 and wecom/adapter.go:42: `WithSender(id, "", "")` -> empty. whatsapp.go:523: DisplayName = phone part of wa_id. dingtalk/adapter.go:72: `SenderNick` (real name, good). discord/adapter.go:105-118: GlobalName/Username(#disc) good. slack.go:272: Profile.DisplayName/RealName good. telegram/adapter.go:129-135: `"%s %s"` FirstName LastName (trailing space when LastName empty) else Username.
- Recipient (chat) title: telegram/adapter.go:47 `msg.Chat.Title`, discord/adapter.go:51 channel name; feishu `WithRecipient(replyTarget, chatType, "")` (feishu.go:387) and weixin/wecom/dingtalk "" -> group name empty on those platforms.
- Persistence consequences (remote/control/bot/authorization.go:79,91,108): `Actor.DisplayName = msg.Sender.DisplayName` and the upsert OVERWRITES display_name on every message, including with "" (internal/db/bot_access_store.go:246 `DoUpdates display_name`) => a once-known name is wiped by a later nameless message. `Group.Name` is set only at first discovery (bot_access_store.go:488) and never refreshed => stale after rename. `DirectChat` gets no name at all (bot_access_store.go:296-316; only manual Alias), although the DM peer's `Sender.DisplayName` is available (it only goes to Actor). S
- `Message.GetSenderDisplayName()` (imbot/core/message.go:243) falls back DisplayName -> Username -> ID: the one existing "name else ID" resolver; nothing in remote/** calls it (grep: only FormatMessage uses it).

### 4. internal/server (HTTP handlers and server logs)

#### 4a. internal/server/module/imbot
- manager.go:134,140,263 `uuid` only; :145,188,258 uuid+name+platform | bot | A at 134/140/263
- manager.go:129,175 error `bot settings not found for uuid: %s` | ok (not-found cannot have a name)
- handler.go:205 created: uuid+platform (name is in the request, not logged); :212 uuid; :349 updated: uuid; :362,366,405,415,444,452,457,594,599: uuid only (name available from `settings`) | bot | A
- handler.go:538,547 `unsupported default_agent %q`, `claude code profile %q not found` | profile id in quotes; returned to UI as `details` | name vs id of profile: shows what the user typed | ok
- handler_pairing.go:29-37 `logPairAudit` fields action, user_id, client_ip, bot_uuid, by | bot | add name | A
- wechat_qr.go:117,122,130,141,248 and feishu_qr.go:84,143-153,226,316,334: key `bot` = tingly UUID; wechat_qr.go:158,190,203: key `botUUID`; wechat_qr.go:300,320,346 `bot_id` = Ilink bot id next to `bot` = tingly uuid | F (three spellings, two meanings)
- routes.go:25-174 swagger error messages ("ImBot settings not found", "UUID is required") | no ids; fine
- access_api.go:144,183,258,363,512 "target not found", "bot not found", "chat not found", "group not found", "route not found" | no ids, no names (acceptable for HTTP, caller supplied the id)

#### 4b. internal/server/module/notify (bot notify/interact API)
- bot_api.go:178,255,393 `WithField("bot", botUUID)`; :219,220,223 `bot` + `chat_id` (external); :239 audit `"target": req.Target` (internal access TargetRef) in the same handler as `chat_id` external => two id spaces in one function | W/F
- bot_api.go:184,261 HTTP 404 `{"error":"target not reachable","reason":decision.Reason}`; bot_api.go:192,277 `bot not running` deliberately uniform for unknown/stopped (anti-probe, comment :166). Design note: do not add names to these 404s. 
- bot_api.go:75-84 `ChatSummary` returned by GET /bots/:bot/chats: chat_id, platform, flags, project_path; NO alias/title, although this is the endpoint that makes chat_id "discoverable" (server_control_chats.go:71-76) | A
- internal/server/server_control_chats.go:120 `bot chats list: bot=%s platform=%s count=%d` (Debug) | format `k=v` unique here | F
- bot_api.go:317-319 `wait_url` /bots/%s/interact/%s | uuid in URL (needed) | -

#### 4c. internal/server/module/projects (chatbridge)
- chatbridge.go:81 `check chat access: %w`; :356 `errChatDisabled` "the chat is disabled" | no chat/bot id | I
- chatbridge.go:176,196,212 maps `chatKey(platform, chatID)` -> bot UUID of "last wrote through": session-affinity only, not logged
- chatbridge.go:407 `projects: chat unreachable through every bot of its platform` | platform only | I
- chatbridge.go:372 `ErrNoChannel: bot <uuid>` | see section 1

#### 4d. server/other
- internal/server/server_control.go:117-121 audit sink = logrus Info (`action` field) - not persisted
- internal/server/server_flags.go:60-63 `%d IM bot(s) in use` (counts only; bots could be listed by name) | A (minor)
- internal/server/server_lifecycle.go:225,253 | no bot ids; ok

### 5. CLI (internal/command; audience: operator)

There is no separate `remote` CLI; bot commands are `tingly-box bot {add,list,config,pair ...}` (internal/command/bot.go). All bot args are typed "Bot UUID" (bot.go:76,85,94,104); only `bot config` also accepts the list index (bot.go:39, resolveBotRef bot.go:187-200). Names are not accepted as references anywhere.

- bot.go:243-256 `bot list`: header `ID  UUID  Platform  Name  Enabled`. The column "ID" is the 1-based list index (resolveBotRef), "UUID" is the real id: the word ID means a different thing than everywhere else. Name column is 15 wide and truncation is not handled; empty name shows "-" | F/A
- bot.go:216-221 `printBotSetting`: Name then UUID on separate lines (both shown) | ok
- bot.go:150 `bot %s not found` uses `r.Bot` (what the user typed) | ok
- bot.go:196 `no bot %d (there are %d); see tingly-box bot list` | ok
- bot.go:175 "Server was notified, but the bot is not running" | no bot named | A (operator could be editing several)
- bot_add.go:112 `UUID: %s` after create (name was just typed, not echoed) | A
- bot_add.go:401-402 `Bot ID: %s` / `User ID: %s` = Ilink (Weixin) ids, printed right next to tingly's `UUID:`: a second "Bot ID" meaning | F
- bot_add.go:87 and bot_pair.go:39: tell the user to send `/bind <code>`; the server banner says `/pair` (manager.go:178-181) and the in-chat usage says `/pair <code>` (command.go:76). `/bind` is only an alias (command.go:314). Format inconsistency for the one string a user must type | F
- bot_pair.go:25,88 `bot %s not found` (uuid typed) | ok
- bot_pair.go:36 `RequirePairing %s for bot %s.` uuid only; name available in `setting` | A
- bot_pair.go:60 `Chat %s is not paired with bot %s.`; :72-73 `Revoked pairing for chat %s on bot %s.` | external chat id + bot uuid only; no name/alias for either (alias lives in access store, not looked up) | A
- bot_pair.go:63-68 audit log `bot_uuid`, `chat_id`, `by=cli` | ok ids
- bot_pair.go:98 `Bot:  %s (%s)` = `Name (UUID)`; the best human format in the repo | model
- bot_pair.go:100-103 points the operator to grep the stderr line `Bot "..." pairing code` (name + platform; no uuid) | see 2a
- bot_notify.go:15-18 `reloadedBot{UUID, Running}`; :77-82 matches by UUID; no names in the server's reload response either (Reload handler internal/server/module/imbot/handler.go:603-618; response list built just after, bots as uuid+running) | A

### 6. projects/** (own Go module; audience: end user in IM and developer)

- Chat-facing text: see section 1 (projects/internal/project/chat_commands.go, relay.go). Names used: project name, folder name (`db.FolderName`), `#seq title`. No bot, no chat alias, no profile anywhere.
- projects/internal/db/models.go:248-284 `ChannelBinding`: persists platform, chat_id (external), bot_uuid, chat_title (always empty in practice, see section 0), profile (json:"-", not exposed). A UI or CLI listing bindings can show only external id unless it joins to the host's alias store. S
- projects/internal/db/models.go:194-195 + project/threads.go:176: thread events from IM persist `SourcePlatform` + `SourceChatID` only (no bot uuid, no sender, no title) => history shows "from IM chat 123456789" at best. S
- projects/internal/project/relay.go:415,428,468,487 logs `{"platform","chat_id"[, "thread_id"]}`; the binding `b.BotUUID` is in scope (relay.go:411) but is never logged | I
- projects/internal/project/threads.go:1119 `thread_id` only; bind.go:164 etc. have no chat ids.
- projects/internal/server/module/channel/handler.go:78-103 HTTP errors via apierr ("thread or project", "binding"); no ids. `BindChannelRequest.ChatTitle` exists (:30) but host never fills it.
- Projects facade `ChannelToFolder(platform, chatID, title, folder, profile, ifNew, botUUID...)` (projects/projects.go:273): `title` param exists; the host passes "" (internal/server/module/projects/chatbridge.go:118). A quick win: pass sender/group display name once known.
- projects/ never imports the root module, so it cannot call the access store for aliases; resolution must be injected through the Host interface (internal/server/module/projects/host.go) or by the host decorating messages. UNVERIFIED whether Host already has a suitable hook.

### 7. Persisted records that hold a bot uuid or chat id (later display needs a join)

| Table / record | Fields | Human name stored? | Join needed |
|---|---|---|---|
| imbot settings (internal/db/imbot_settings.go:9) | bot_uuid (pk), name | yes (Name) | - |
| remote_chats (internal/db/remote_chat.go:25) | chat_id, platform, paired_bot_uuid, owner/paired_sender_id, whitelisted_by | no | alias via access store by (bot_uuid, platform, external_chat_id) |
| remote_actors (internal/db/bot_access_store.go:36-44) | bot_uuid, external_actor_id, display_name | yes, but overwritten each message (stale/wiped, section 3) | - |
| remote_direct_chats (:49-60) | bot_uuid, external_chat_id, alias | alias only (manual); no auto name | - |
| remote_groups (:75-85) | bot_uuid, external_group_id, alias, name | name only at first discovery | - |
| remote_group_actors (:99-105) | group_id, actor_id, label | label | - |
| remote_routes (:120+) | bot_uuid, name, target (internal ids) | name | - |
| projects channel_bindings | platform, chat_id, bot_uuid, chat_title(empty), folder, profile | no | alias via host |
| projects thread events | source_platform, source_chat_id | no | alias via host |
| usage/history records (internal/db/usage_record.go, record_outcome.go) | no bot/chat columns found (grep for bot|chat|im: none) | n/a | - |
| audit "events" (bot.authorization.decision, bot.notify*, imbot.pair.*, claude_code.*) | bot_uuid / bot / channel, target_id (internal), chat_id | logrus Info only, NOT persisted (server_control.go:117-121) | none possible later; logs must be self-describing at write time |

### 8. Distinct format patterns found (for the unification design)

Bot identity in logs (field key for the tingly bot uuid):
1. `"uuid"` - ~35 sites (remote/control/bot/manager.go x17, internal/server/module/imbot/manager.go x5, handler.go x11). Example `logrus.WithField("uuid", uuid).Info("Bot stopped")`. Ambiguous across entity types: the same key is used for provider/rule uuids in internal/server/module/{oauth,rule}, internal/typ.
2. `"bot_uuid"` - ~19 sites incl. authorization.go:124, handler_message.go:170/192, handler_pair.go:90/103, runtime_default.go:70, prompt_reply.go:368, internal/command/bot_pair.go:68, handler_pairing.go:34.
3. `"bot"` - ~20 sites (wechat_qr.go x7, feishu_qr.go x8, notify/bot_api.go x4 + appendBot :589).
4. `"botUUID"` - 4 (imprompter.go:116, wechat_qr.go:158/190/203).
5. `"channel"` = bot uuid - runtime_default.go:133, claudecode/plugin.go:110,136.
6. `"bot_id"` = Ilink platform bot id (wechat_qr.go:300,320,346; bot_add.go:396) - NOT the tingly uuid.
7. Free-text: `bot=%s` (server_control_chats.go:120, 1x), `[%s/%s]` platform/uuid (core/base.go:473, manager.go:453 - 2x), `%s bot [%s]` (imbot/manager.go:170), `(UUID: %s)` (imbot/manager.go:141), `bot %s` (chatbridge.go:347,372; bot_pair.go:36,60,72; bot.go:150), `Bot %q (%s)` name+platform (manager.go:180), `%s (%s)` Name (UUID) (bot_pair.go:98), `bot for platform %s` (manager.go:431), platform-only `%s bot ...` (imbot/manager.go:136,212,460-498; ~40 adapter lines).

Bot name: `"name"` field only in 7 bot log sites; free-text only in manager.go:180 and bot_pair.go:98.

Chat identity in logs:
1. `"chat_id"` (external) - remoteagent/handler_message.go:171/193, handler_pair.go:91/104, imprompter.go:187, notify/bot_api.go:220/223, bot_pair.go:69, autochannel.go:73/89, projects relay.go x4.
2. `"chatID"` camelCase - project_route.go x5, handler_bind.go:76, handler_send.go:15, callback.go:216 (9 sites).
3. `"target"` - prompt_reply.go:368 (weixin token), `"target_id"` (INTERNAL uuid) - authorization.go:124, runtime_default.go:73.
4. Free-text: `chat %s` (handler_message.go:37, prompt_reply.go:35, handler_bind.go:41), `chat=%s path=%s` (handler_bind.go:78), `Chat ID: %s` / `Group chat ID: %s` (handler_message.go:139,142; feishu.go:325), `Group %s` (handler_message.go:184, command.go:291,300), `Chat %s ... bot %s` (bot_pair.go:60,72), `chat %q` (remote_chat_store.go:161,280) vs `chat %s` (:139,215,241,285,308).
5. `target=%s` in feishu send debug lines (feishu.go:429,489,501).

User/actor identity: `"user_id"` (~20 sites, = external sender id; authorization.go uses `actor_id` = internal uuid), free-text `by %s` (command.go:300), `from %d` (telegram.go:185).

Quoting inconsistency: `%q` for names (manager.go:180) and for chat ids (remote_chat_store.go:161), `%s` elsewhere.

Name-bearing formats worth keeping as the seed of the convention: `Name (UUID)` (bot_pair.go:98), `[#seq title]` thread tag (relay.go:291), project/folder name first.

### 9. Highest-impact gaps (summary)

1. imbot adapters (85 log lines) carry no bot identity at all; only a per-bot logger prefix fixes it in one place.
2. Alias/Name/DisplayName are write-only on the backend: no log, error, CLI line or IM message reads `DirectChat.Alias`, `Group.Alias/Name`, `Route.Name`, or the Claude Code profile name.
3. IM `/status` and `/help` omit bot name, profile and chat alias; the bot never names itself to users.
4. Two id namespaces (external chat id vs internal access uuid) appear under the same key names; authorization logs have only the internal ones.
5. Name data is stale or empty at the source: Actor.DisplayName wiped on empty updates; Group.Name never refreshed; DirectChat unnamed; Feishu/Weixin/WeCom senders and groups unnamed; Projects `chat_title` never filled.
6. Audit-type events are logrus-only, so they cannot be re-labelled later.
7. Operator pairing path is inconsistent (`/bind` vs `/pair`; stderr banner lacks UUID; CLI list column "ID" is an index).