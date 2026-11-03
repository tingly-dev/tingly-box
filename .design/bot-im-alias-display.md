# Bot / IM display convention: alias first, ID reachable

> Status: **proposal** (nothing implemented). Inventory and evidence: [`.research/bot-im-alias-display.md`](../.research/bot-im-alias-display.md) — cited below as R-A / R-B / R-C (Parts A, B, C).
> Judged against [`ux-principles.md`](./ux-principles.md); the checklist is at the end.
> Related: [`bot-arch.md`](./bot-arch.md), [`bot-capability-access-control.md`](./bot-capability-access-control.md), [`projects-im.md`](./projects-im.md), [`logging.md`](./logging.md).

## 1. Problem

Bots and IM entities have an ID *and* a human name, but the product treats them inconsistently:

- The same chat gets up to five different labels across five surfaces; Projects shows the raw platform chat id even after the user names the chat (R §2.1, R-B §6).
- Several surfaces show the wrong thing: an internal uuid in a delete confirm, a raw id in a picker whose input shows the name (R §2.2).
- The names themselves are often empty or stale at the source, so a UI-only fix would just show `""` more consistently (R §2.3).
- The backend has the aliases but never uses them: logs, CLI, and the messages the bot sends to users carry IDs only, and imbot adapter logs carry no bot identity at all (R §2.5).
- Where users *need* the ID (paste into `/join`, the notify API, grep logs, compare with the IM app), it is often hidden (R §2.4).

This is a display-convention problem on top of a data-quality problem. Both need work, in that order.

## 2. Principles for this feature

1. **Alias is the primary text; the ID is secondary but always one step away.** People recognise “Ops alerts”, machines need `-100123456789`.
2. **Show the ID itself (mono, copyable) wherever it is the thing the user will paste or grep** — this is ux-principles §5 (“show the concrete value, not the alias”). The two rules do not conflict: the *kind* of surface decides which one leads (§3.2).
3. **Never blank, never `undefined`, never the wrong ID space.** Every entity always renders something, and a deleted/unknown entity renders its stored ID marked as such.
4. **One word, one meaning** (ux-principles §3). One term for the user-set label of each entity, used in every locale and surface.
5. **Resolve once, near the data.** A label is computed in one place per layer (a server resolver for what clients cannot join, one shared component/helper for UI, one field-set helper for logs) rather than re-derived at every site.
6. **Scope to surfaces.** A rename in the Access dialog must show up in Notify, Projects, toasts and logs on the next render/line, without a manual sync.

## 3. The convention

### 3.1 Entity fields and fallback chains

“Label” is the single human string for an entity. Chains are evaluated top to bottom; the first non-empty trimmed value wins. `⟨short⟩` is a 4-char disambiguator derived from the ID (§3.4).

| Entity | Label chain | Always-available ID shown as secondary | Notes |
|---|---|---|---|
| Bot | `name` → `⟨Platform display name⟩ ⟨short⟩` | `uuid` (mono, copy) | Never `"<platform id> Bot"`. New bots get a *unique* generated default (`Telegram`, `Telegram 2`) at creation so the stored name is meaningful. |
| Platform | descriptor `display_name` | `platform` id (tooltip/dev only) | Single source = `GET /imbot-platforms`. No hand-copied maps. |
| Direct chat | `alias` → peer actor `display_name` → `external_chat_id` | `external_chat_id` | Peer name needs the server join (§4). |
| Group | `alias` → `name` → `external_group_id` | `external_group_id` | |
| Actor | group-context `label` → `display_name` → `external_actor_id` | `external_actor_id` | `label` is today stored and never shown. |
| Route | `name` | `id` (tooltip) | Plus the resolved target label. |
| CC profile | `name` → `id` | `claude_code:<id>` | Already consistent today; keep. |
| Pairing code | not an alias entity | the value itself | Keep showing the full `/pair <code>` string. |
| Project / thread | unchanged (`#seq title`) | — | Already the model for IM text. |

The platform *id* is never user-visible text; the platform display name is.

### 3.2 Three display densities

| Density | Surfaces | Form | Example |
|---|---|---|---|
| **Inline** | chips, tags, select input value, breadcrumbs, graph node labels, toasts, button aria-labels | `Label` (+ ` · ⟨short⟩` only if two visible entities in scope share a label) | `Ops alerts` |
| **Row** | table cells, list rows, select *options*, dialog headers | `Label` primary, ID mono secondary line/column with copy affordance | `Ops alerts` / `-100123456789 ⧉` |
| **Text** | confirm dialogs, error/log/CLI text, IM messages when an ID must be pasted | `Label (id)` — id full for machine-use, `⟨short⟩` otherwise | `Delete “Ops alerts” (-100123456789)?` |

Rules that make the densities safe:

- **Select input value and option label must be the same Label** (fixes `NotifyTestDialog.tsx:108` vs `:118`). Options add the ID as the secondary line.
- **Disambiguation is by scope, not global.** If the visible list has two bots called `Telegram`, append `· ⟨short⟩` to both; otherwise show the clean label. (Avoids permanent noise, fixes duplicate-name pain.)
- **Copy value = the ID the user can use next.** For a chat: the *external* id (what `/join` and the notify API want); the internal uuid is available only in a “details” tooltip and labelled as such. For a bot: `uuid`.
- **Toast/confirm text always names the entity** (“Bot “Ops” restarted”, “Chat “Ops alerts” blocked”).

### 3.3 Unresolved and deleted entities

| Situation | Render | Where the info comes from |
|---|---|---|
| Entity exists, no alias, no platform name | ID in mono, muted style, tooltip “No name set — click Rename” | chain tail |
| Entity referenced but deleted (binding, route target, `default_agent`, `bot_uuid`) | stored ID in mono + muted “deleted” suffix/chip | server resolver returns `{id, label:"", resolved:false}` |
| Name was known once but source is gone | last-known snapshot name + “deleted” | needs a snapshot, see Q4 |
| Resolution failed / still loading | skeleton or ID for ≤ 1 render, never blank | UI state |

Principle: the *ID is the only thing guaranteed to exist*, so it is the universal fallback; the UI never invents text.

### 3.4 Short id (`⟨short⟩`)

Bot ids are `<unixnano>-imbot` (`internal/db/imbot_settings_store.go:316`) — prefix digits are nearly identical across bots created close together, so the disambiguator must be the **last 4 digits of the numeric part** (e.g. `…6789`). Chat/actor internal uuids: first 4 hex. The short id is only a disambiguator and must never be the copy value.

### 3.5 Terminology

- **Bot**: user-defined label = **Name** (field `name`; UI currently mixes “Alias” and “Name”, R-B §6 P5).
- **Chat / group**: user-defined label = **Alias**; platform-supplied = **Platform name** (shown as the fallback, not a separate field in UI).
- Rename affordance on a chat says “Alias”; its helper text “Leave empty to use the platform name or ID” already matches the chain above.
- The word “ID” in UI/CLI always means the identifier the user can use next (external chat id, bot uuid). The CLI `bot list` column named “ID” (list index) becomes “#”.

### 3.6 Logs, CLI, IM messages

| Channel | Rule |
|---|---|
| **Logs** | Greppable ID **and** name as separate structured fields. Canonical keys: `bot_uuid`, `bot_name`, `platform`, `chat_id` (always the *external* id), `chat_alias` (when known), `actor_id` (external), `route_name`. Internal access uuids are renamed `access_target_id` / `access_actor_id` so they cannot be mistaken for `chat_id`. Free-text form when needed: `bot "Ops" (1730…6789)`. |
| **imbot adapter logs** | One logger prefix per bot, applied once: `[imbot telegram/1730…6789 Ops]` (extends the existing `[%s/%s]` panic prefix at `imbot/core/base.go:473`). Adapters stop formatting identity themselves. |
| **CLI** | `Name (UUID)` everywhere a bot is mentioned (already done at `internal/command/bot_pair.go:98`); chats as `Alias (external id)`; accept name or uuid or `#index` as a reference. |
| **IM messages to users** | Bot names itself where it helps orient: `/status` and `/help` get a `Bot: <name>` line and the chat’s alias; pairing/authorisation replies name the bot. IDs stay where they are required (`Your ID`, `Chat ID`, `/join <ChatID>`) and gain the title beside them. Follow the Projects pattern (`[#seq title]`, names first). |
| **Persisted audit events** | Today logrus-only (R-C §7). Any event we may need to re-label later must carry the name at write time. |

## 4. Where labels get resolved — options

The UI needs `Label` for entities it did not load (route target, binding chat, thread-event source, `peer_actor_id`, `default_agent`, `bot_uuid` on a binding). Three ways to get it:

| | Option 1: server resolves, embeds in responses | Option 2: client joins from lists it already has | Option 3: separate `labels` resolve endpoint |
|---|---|---|---|
| How | Responses carry `*_label` / nested ref `{id, label, resolved}` | Shared `useBotDirectory` / `useChatDirectory` hooks fetch bots, chats, groups once and look up | `POST /imbot/labels {refs:[…]}` → labels |
| Pros | One source of truth; log/CLI/IM can use the same resolver; works in the Projects module through host without N+1; deleted-entity handling is natural | No API change; fast to ship for bot-centric pages | Single place to evolve; batched |
| Cons | Touches several response structs + swagger + codegen; Projects (separate Go module) needs a host-injected resolver | Does not cover Projects/thread-event/binding data; every page re-implements fetch/cache/invalidation; stale after rename elsewhere; cannot serve logs/CLI/IM | New endpoint nobody else benefits from; extra round trip |
| Cost | M–L | S–M per page, repeated | M |

**Recommendation: Option 1 as the backbone, Option 2 only as a thin client cache on top.** Reasons: the biggest gaps (Projects tags, transcript source lines, route targets) are exactly the ones a client-side join cannot cover without moving join logic into the Projects package (which must not import the root module, `CLAUDE.md`), and the backend text channels (logs/CLI/IM) need the same resolver anyway. Option 3 is a fallback if response-shape changes prove too wide.

### 4.1 Backend resolver

One small package in the remote domain, e.g. `remote/access` (`labels.go`), no new storage:

```go
type Ref struct {            // goes in API responses
    ID       string `json:"id"`                 // the id the user can use next (external where applicable)
    Label    string `json:"label"`              // resolved per §3.1; never empty when Resolved
    Resolved bool   `json:"resolved"`           // false = entity gone/unknown, label echoes the ID
    Name     string `json:"name,omitempty"`     // user alias, if any (lets UI style "unnamed")
}

type Resolver interface {
    Bot(uuid string) Ref
    Chat(botUUID, platform, externalID string) Ref   // DirectChat→Group lookup, applies §3.1 chain
    Actor(botUUID, platform, externalID string) Ref
    Profile(id string) Ref
}
```

It reads the existing `remote_direct_chats`, `remote_groups`, `remote_actors` and `imbot_settings` tables, caches per request, and is the **only** place the §3.1 chains live on the server. The logging helper and the IM-message builders call the same resolver.

For the Projects module: extend its `Host` interface (`internal/server/module/projects/host.go`) with `ChatLabel(platform, chatID, botUUID) Ref` and `BotLabel(uuid) Ref`; the root module implements them with the resolver above. `projects/` never imports the root module. (Whether `Host` already has a suitable hook is UNVERIFIED, R §5.)

### 4.2 Source data fixes (prerequisite, otherwise labels are empty)

| # | Fix | Where | Why |
|---|---|---|---|
| D1 | Do not overwrite `display_name` with `""`; only update when the incoming value is non-empty | `internal/db/bot_access_store.go:246` `upsertActor` | Names currently wiped by nameless messages (R §2.3) |
| D2 | Refresh `Group.name` when the platform reports a different non-empty title | `DiscoverGroup` `bot_access_store.go:477-505` | Never refreshed → stale after rename |
| D3 | Give direct chats a name: resolve via `peer_actor_id → Actor.display_name` in the chain (preferred, no new column), or copy sender name into a new `DirectChat.name` at pairing/discovery | `bot_access_store.go:296-322`, `authorization.go:93-108` | DMs have no platform-captured name at all |
| D4 | Real feishu sender names (contact API) and weixin/wecom where the platform exposes one; otherwise leave `""` so the chain falls through to the id | `imbot/platform/feishu/adapter.go:72`, `feishu.go:386`, `weixin/adapter.go:55`, `wecom/adapter.go:42` | Today feishu name == raw id, which defeats the chain. Needs API scope — Q3 |
| D5 | Unique generated default bot name; stop writing `"<platform id> Bot"`; allow clearing a bot name | `BotConfigDialog.tsx:163,252`, server QR flows `wechat_qr.go:330-335`, `feishu_qr.go:320-322`, `imbot_settings_store.go:181` | Duplicate names are the root of “which Telegram bot?” |
| D6 | Dangling-reference handling: on bot delete remove its Projects `channel_bindings` (or mark unresolved); on profile delete clear/flag bots’ `default_agent` | `imbot_settings_store.go:224`, Projects bridge; `internal/config/profile.go:228` | Needed for §3.3 “deleted” rendering to be rare and honest |

D1/D2/D3 are tiny, local, and unblock everything; D4 is the only one with a platform-permission cost.

### 4.3 API changes (swagger-defined, then `task codegen`)

Additive only; existing fields stay.

| Response | Add | Replaces client-side |
|---|---|---|
| `DirectChatDetailResponse` (`GET /bots/:bot/chats[/:chat]`) | `label` (resolved, §3.1), `peer_actor` ref | `alias || external_chat_id` in `imHost.tsx:48`, Access dialog |
| `Group` list/detail | `label` | `alias || name || id` in 3 places |
| `Route` | `target_ref {kind,id,label,external_id,resolved}` | UI join across chats+groups |
| Projects `ChannelBinding` | `chat_ref`, `bot_ref` (resolved live through the Host), keep `chat_title` as last-known snapshot | `chat_title \|\| chat_id` fallbacks (`ProjectImBindings.tsx:75,107`) |
| Projects `ThreadEvent` (IM source) | `source_chat_ref` | `ConversationPane.tsx:356`, `ThreadPanel.tsx:480` |
| `Settings` (bot) | `label`, `short_id`, `platform_label`, `default_agent_ref` | `name \|\| platform` (9 sites), profile join |
| `PairingCodeResponse`, QR status, reload | `bot_ref` | toasts lacking name |

If Option 1 stalls on response size, the smallest useful subset is: `label` on DirectChat/Group/Settings, `chat_ref`/`source_chat_ref` on Projects, `target_ref` on Route.

## 5. Frontend

### 5.1 One presentational primitive

`EntityLabel` in `@tingly/ui` (shared by the root app and the Projects pages, per `CLAUDE.md`), purely presentational, no data fetching:

```tsx
<EntityLabel
  label="Ops alerts"        // §3.1 result
  id="-100123456789"        // the user-usable id
  density="inline|row|text"
  resolved={true}           // false → muted + "deleted"
  unnamedHint={…}           // shown as tooltip when label === id
  copy                      // CopyIconButton, copies `id`
  icon={<PlatformMark/>}
  disambiguator="6789"      // only passed when scope has duplicates
/>
```

It generalises the existing `rowHeader` (`BotAccessDialog.tsx:48`) and reuses `CopyIconButton`, `PageLayout` primitives. Styling follows ux-principles §9: label is the visual anchor, ID is muted mono.

### 5.2 One thin data layer in the root app

`frontend/src/services/labels.ts` (plain functions, tested) + a small `useBotLabels()` that returns `labelFor(bot)` with scope-aware disambiguation (§3.2). Because the server provides `label`/`*_ref` (§4.3), this layer mostly *reads* server labels and only computes the “duplicates in scope” suffix. The four platform-name sources collapse to `platform_label` from `/imbot-platforms` (`PlatformConfig.display_name`) — delete `PLATFORM_NAMES` in `imHost.tsx:19-23`.

### 5.3 Projects package

- `ImHost` gets `chatLabel`/`botLabel` implemented (the optional `chatTitle` hook at `ProjectsHost.tsx:82` is already called by `ThreadPanel.tsx:480` but never implemented), fed from the server refs.
- `bind` sends the resolved label as `chat_title` (`useImBindings.ts:58`) so the snapshot is no longer empty (last-known fallback for deleted chats).
- `chatKey` includes `bot_uuid` (`useImBindings.ts:13`).
- Delete the second resolver; both transcript surfaces use the same one.

### 5.4 Site-by-site targets

Full per-site table is R-B. The convention applied:

| Surface | Today | After |
|---|---|---|
| Notify chat leaf (`ChatNode.tsx:60`) | external id for DMs | alias / peer name / id; tooltip keeps both ids, labelled “Chat ID” and “Internal target UUID” |
| Notify delete confirm (`BotNotifyGroup.tsx:593`) | **internal uuid** | `Delete “Ops alerts” (-100123456789)?` |
| Test-send picker (`NotifyTestDialog.tsx:108,118`) | name in input, id in options | same label both; id as option secondary line |
| Remote card / Notify card header | `name \|\| platform` | `EntityLabel` bot, density row, uuid in the secondary line/tooltip with copy |
| Bots table | `name \|\| platform` + uuid column | label + disambiguator; uuid column kept |
| Projects tags / source line / transcript | raw chat id | server-resolved chat label; id on hover |
| Projects IM settings chat rows | `Platform · alias` (id hidden) | label + external id secondary |
| Toasts (~20), icon-only button `aria-label`s | no entity | include entity label (§3.2 inline) |
| Access dialog Rename / CC profile dialog titles | no entity | name the chat / bot |
| Group actor chips | `display_name \|\| id` | `label \|\| display_name \|\| id`, id in tooltip |

## 6. Backend text channels

Concrete work (details and line refs in R-C):

1. **Logging field helper** `logfields.Bot(setting)`, `.Chat(botUUID, platform, externalID)` producing the §3.6 keys; migrate the ~80 bot/chat log sites over time, starting with lifecycle, authorization, pairing and imprompter “auto-approving” lines. Normalise `uuid|bot|botUUID|channel` → `bot_uuid`, `chatID|target` → `chat_id`.
2. **imbot per-bot logger prefix**: add `Name`/identity to the logger built from `core.Config` (or inject a prefix from `remote/control/bot`), so the 85 adapter lines are attributable without touching call sites. `imbot/core/logger.go:60-70`, `imbot/core/base.go:45`.
3. **IM messages**: `/status` (`projects/internal/project/chat_commands.go:193-224`) gains `Bot:` and `Profile:` lines; `/help` header gains the bot name and chat alias beside the two IDs (`remote/control/remoteagent/project_help.go:8-28`); pairing/auth replies and the “not authorized” notice name the bot (`handler_pair.go:29`, `command.go:330`, `manager.go:140`). Keep IDs where the user pastes them (`/join <ChatID>`).
4. **CLI**: echo name+uuid after `bot add`, `pair`, `config`; accept name/uuid/index in `resolveBotRef`; rename list column “ID” → “#”; unify `/bind` vs `/pair` text (`bot_add.go:87`, `bot_pair.go:39`, `manager.go:178-181`); add UUID to the stderr pairing banner that `bot pair status` tells operators to grep.
5. **Server errors that reach the UI** (`ErrNoChannel: bot <uuid>`, “bot for platform %s is missing credentials”): resolve to label + id before returning. Keep the deliberately uniform 404s for notify/interact (anti-probe, `bot_api.go:166-192`) unchanged — do not add names there.
6. **Audit events** (`bot.authorization.decision`, `imbot.pair.*`, notify): include `bot_name`, `chat_alias`, `route_name` at write time because they are logrus-only and cannot be relabelled later.

## 7. Suggested order of work

| Phase | Content | Size | Why this order |
|---|---|---|---|
| **P0 – bug-level fixes** (no design needed) | Delete-confirm shows internal uuid; picker input/option mismatch; map alias onto Notify targets; implement `ImHost.chatTitle`; send `chat_title` on bind; `chatKey` with bot; `imHost.tsx` group `name`; stale i18n copy `remote.ts:400`; Notify empty-state platform id | S (≈1–2 days) | Pure wrong-field/dropped-field bugs; immediately visible; independent of the rest |
| **P1 – source data (D1–D3, D5)** | actor upsert no-wipe, group name refresh, DM name via peer actor, unique default bot name | S–M | Labels are only as good as the data; tiny and local |
| **P2 – resolver + API** | `Resolver` + `label/*_ref` fields, Projects `Host.ChatLabel`, swagger + `task codegen` | M | Backbone for UI and text channels; includes platform label from `/imbot-platforms` |
| **P3 – shared UI** | `EntityLabel` in `@tingly/ui`, `labels.ts`, migrate all sites in R-B (bots, Notify, Remote, Projects, toasts, aria-labels) | M | Mechanical once P2 exists; fix by surface, not by file |
| **P4 – backend text channels** | `logfields`, imbot logger prefix, IM `/status` `/help` messages, CLI polish | M | Independent of P3; can run in parallel after P2 |
| **P5 – lifecycle/consistency** | D4 (feishu/weixin names, needs scope), D6 dangling cleanup, persisted audit, `ErrNoChannel` text | M, partly blocked by Q3/Q4 | Highest cost per benefit; do last |

P0 can ship alone as a PR, then P1, then P2→P3 as the “main” feature; P4 parallel to P3.

## 8. Risks and trade-offs

- **Privacy of platform names.** Persisting sender names (D3/D4) stores more PII; today it is already stored on `Actor`. IM messages that print the bot’s name or the chat alias are visible to every member of the chat (Q5).
- **Rename propagation vs snapshots.** Live resolution makes renames propagate; snapshots (`chat_title`) survive deletion. We keep both: live first, snapshot as the “deleted” fallback.
- **Log volume/cost.** Name lookups per log line must come from the in-memory bot setting already in scope; do not add DB reads to hot paths. imbot adapters get the name once at construction.
- **Name staleness at renames** in long-running processes: the imbot logger prefix is fixed at bot start; a rename takes effect on restart (acceptable; the uuid stays correct).
- **Scope creep**: sharing keys/API tokens are explicitly out of scope (no bot/IM relation, R §1).

## 9. Open questions for the user

1. **Terminology.** OK to standardise on **Name** for bots and **Alias** for chats/groups (platform-supplied title is just the fallback)? Or one word (“Name”) everywhere?
2. **DM naming.** For direct chats, resolve the platform name through the peer actor (no schema change, my recommendation) or copy it into a stored `DirectChat.name`?
3. **Feishu/Weixin/WeCom names.** Fetching real sender names on Feishu needs contact-API permission on the user’s app; Weixin/WeCom may not offer it at all. Accept a permission/scope requirement, or live with id-fallback on those platforms?
4. **Deleted entities.** Keep a last-known-name snapshot (needs a denormalised column on bindings, more storage) or show only the ID marked “deleted”? And on bot delete: cascade-delete Projects bindings or keep them as unresolved?
5. **Should the bot name itself in IM?** Adding `Bot: <name>` / chat alias to `/status`, `/help` and pairing replies exposes those labels to everyone in the chat. Fine for all chats, only DMs, or opt-in?
6. **Logs.** Is it acceptable to include `bot_name` / `chat_alias` in every structured log line (greppability gain, minor extra PII)? Or names only on lifecycle/audit lines, uuid-only elsewhere?
7. **Bot ID shape.** Server-generated ids look like `1730600000123456789-imbot`. Worth introducing a real short, human-pasteable id (or a unique slug) for new bots, or keep and use the last-4 disambiguator?
8. **Default bot name.** New bots: generated unique name (`Telegram`, `Telegram 2`) as proposed, or force the user to name the bot in the create dialog?
9. **Reference-by-name in CLI.** OK to accept bot name as a reference in `tingly-box bot …` (ambiguity → error listing uuids)?
10. **Delivery.** Ship P0 as an immediate small PR, then the rest as one feature branch, or everything together?

## 10. UX-principles checklist (`ux-principles.md`)

- [x] Information organised by user questions: “which chat/bot is this” answered by the label; “how do I reference it” answered by the ID line (§3.2).
- [x] No mode picker introduced; density follows the surface, not a setting.
- [x] One word per concept (§3.5), and the two ID spaces are separated (§3.6 key rename).
- [x] Orthogonal axes split: label (what people recognise) vs id (what machines use) vs disambiguator vs resolved/deleted state.
- [x] Alias vs concrete value (§5): label leads on recognition surfaces, ID leads on paste/grep surfaces (`/join <ChatID>`, copy value, logs, CLI) — the rule is stated per surface, not globally.
- [x] Smart defaults: unique generated bot names, scope-aware disambiguation instead of a toggle.
- [x] Noise reduced (§9): ID is muted mono, disambiguator appears only on collision.
- [x] Next-action artifact: copy value is the ID the user can use next; Rename affordance is reachable from the unnamed hint (§3.3).
- [x] Side effects scoped: rename propagates to all surfaces via the resolver, no manual sync.
- Deviation: none requiring justification; the one tension (principle 5 vs alias-first) is resolved by the per-surface rule above.
