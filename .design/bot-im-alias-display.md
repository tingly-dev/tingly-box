# Bot / IM display convention: `平台 · alias · 群 · id`

> Status: **partly implemented** on branch `tp/audit-and-design-bot-im-alias-di-c6b317` (see §14); not merged. Inventory and evidence: [`.research/bot-im-alias-display.md`](../.research/bot-im-alias-display.md), cited below as R-A / R-B / R-C (its Parts A, B, C).
> Judged against [`ux-principles.md`](./ux-principles.md); checklist at the end.
> Related: [`bot-arch.md`](./bot-arch.md), [`bot-capability-access-control.md`](./bot-capability-access-control.md), [`projects-im.md`](./projects-im.md), [`logging.md`](./logging.md).

**Status markers.** Every section below is tagged **[DECIDED]** (stated by the user on 2026-11-03, treat as fixed), **[DERIVED]** (follows directly from a decided rule; change only if the rule changes) or **[PROPOSED]** (my recommendation; the user can override). Section 10 lists every open question with the default taken. After the 2026-11-03 delegation the remaining [PROPOSED] tags are implementation details that I may adjust while building; I record any change in §12.

## 0. Decisions already made (user, 2026-11-03) [DECIDED]

1. **One display format everywhere:** `平台 · alias · 群 · id` (platform · alias · group · id). The group marker sits right after the alias it qualifies. (History: `平台 · alias · id [type]` → `平台 · 群 · alias · id` → this, on 2026-11-03/04; the middle position read oddly.)
   - Any segment that is empty or does not apply is omitted, together with its separator.
   - The `群` (group) segment appears only for group chats. Direct chats, bots and actors show `平台 · alias · id`.
   - The alias is omitted when there is none.
   - A very long id may be shortened or omitted in display, but the full id must always be reachable another way (copy button, tooltip, detail view).
   - The platform segment shows the platform's display name.
   - This **supersedes** `平台 · 群 · alias · id` (group second) and, before it, `平台 · alias · id [type]` (trailing type).
2. **Naming model:** if the platform supplies a name, store it as the default alias. If it supplies none, leave the alias empty and display the id until the user sets an alias.
3. **Bots follow the naming model (D4):** by default a bot's name is its platform account name (e.g. Telegram `@username`). If the platform gives no name, the name stays empty and the bot displays by id. The old generated `"<platform> Bot"` default is dropped.

4. **Delegation (2026-11-03):** the user delegated the remaining open points to me ("alias display的工作自主推进"). I took my proposed defaults for D1, D2, D3, D5, D6 and D7 and they are now **DECIDED (delegated)**: they stand unless the user overrides them. The §10 table keeps the reasoning.

## 1. Problem [context]

Bots and IM entities have an ID *and* a human name, but the product treats them inconsistently:

- The same chat gets up to five different labels across five surfaces. Projects shows the raw platform chat id even after the user has named the chat (R §2.1, R-B §6).
- Some surfaces show the wrong thing: an internal uuid in a delete confirm, a raw id in a picker whose input shows the name (R §2.2).
- The names are often empty or stale at the source, so a UI-only fix would only show blanks more consistently (R §2.3).
- The backend holds the aliases but never uses them: logs, CLI and bot-sent IM messages carry ids only, and `imbot` adapter logs carry no bot identity (R §2.5).
- Where users need the id (paste into `/join`, the notify API, grep logs, compare with the IM app) it is often hidden (R §2.4).

This is a display-convention problem sitting on a data-quality problem. Both need work, data first.

## 2. Principles [DERIVED]

1. **Alias is the recognisable part; the id is always present or one step away.**
2. **Never blank, never `undefined`, never the wrong id space.** Every entity renders something. A deleted or unknown entity renders its stored id, marked as such.
3. **One format, one word per concept** (ux-principles §3). The user-chosen string is the same segment order on every surface.
4. **Resolve once, near the data.** Segments are computed in one server resolver, rendered by one shared component, and formatted by one text helper for logs/CLI/IM, rather than re-derived per site.
5. **Platform data may fill a gap but never override the user.** (§4 invariants.)
6. **A rename shows up everywhere** on the next render or log line, with no manual sync.

## 3. The display format

### 3.1 Segments [DECIDED order and omission rules; DERIVED details]

```
平台 · alias · 群 · id
```

| Segment | Content | Rule |
|---|---|---|
| 平台 (platform) | platform **display name** ("Telegram", "Feishu", "WeCom"), from the descriptor table / `GET /imbot-platforms`. Never the raw platform id (`telegram`). | Always present. Rendered as icon + name in the UI; in lists grouped under a bot header the icon may stand in for the text (§10 D6). |
| alias | the entity's alias (§4) | Omitted when empty. Never replaced by a placeholder like "Unnamed". |
| 群 (group) | marks a group chat; rendering in §10 D5 (localised word `群` / `Group`, as a small chip in the UI) | Only for group chats. Omitted for direct chats, bots and actors. Directly after the alias so it qualifies the name it follows (`Dev team · 群`), not the platform. |
| id | the id the user can use next: external chat id for chats/groups, external actor id for actors, bot uuid for bots | Always present, mono, muted. May be shortened or hidden when too long (§3.3); full id always reachable. |

Separator is a middle dot with spaces (` · `). No trailing separator when a segment is omitted.

Examples:

| Entity | With alias | Without alias |
|---|---|---|
| Direct chat | `Telegram · Alice · 123456789` | `Telegram · 123456789` |
| Group chat | `Feishu · Dev team · 群 · oc_a1b2…9z` | `Feishu · 群 · oc_a1b2…9z` |
| Bot | `Telegram · Ops bot · …456789` | `Telegram · …456789` |
| Actor (person) | `Telegram · Alice · 123456789` | `Telegram · 123456789` |

Visual weight in the UI follows ux-principles §9: alias is the anchor (normal/bold), platform and id are muted, id is monospace, `群` is a small muted chip. When the alias is empty the id takes the anchor position instead of the muted one, so the line never looks empty.

### 3.2 Which entities use it [DERIVED: user said "everywhere"; non-platform entities are PROPOSED to keep simple labels]

The format applies to every **platform-scoped** entity: bot, direct chat, group, actor.

Entities that are not platform-scoped keep their own simpler label because there is no platform or external id to show:

| Entity | Label |
|---|---|
| Route | `name` followed by its resolved target in the same format, e.g. `alerts → Feishu · Dev team · 群 · oc_a1…9z` |
| Claude Code profile | `name` with `claude_code:<id>` as secondary (already consistent today) |
| Project / thread | unchanged: `#seq title`, project/folder name |
| Pairing code | not an alias entity; the full `/pair <code>` string stays |

### 3.3 Shortening and getting the full id [DECIDED principle; PROPOSED thresholds and per-kind rules]

Shortening is a **display-only, width-driven** decision made by one helper `shortId(kind, id)`:

| Kind | Rule | Why |
|---|---|---|
| bot uuid | drop the constant `-imbot` suffix, keep the last 6 digits: `1730600000123456789-imbot` → `…456789` | Server-generated ids are `<unixnano>-imbot` (`internal/db/imbot_settings_store.go:316`). The head digits are nearly identical for bots created close together and the tail is constant, so a generic head…tail rule would show indistinguishable text. |
| external chat / group / actor id | ≤ 20 chars: full. Longer: first 6 + `…` + last 4 | Platform ids vary in length (Telegram numeric vs Feishu `oc_` + 32 chars). |

Truncation order when the row is still too narrow: shorten the id, then hide the id behind the copy/tooltip affordance, then ellipsise the alias. The platform and `群` segments are never dropped.

**The full id must always be reachable**, so every place that shows a shortened or hidden id also has at least one of:

- a copy button that copies the full id (visible, not hover-only, so touch devices work),
- a tooltip with the full id,
- a detail view (the Access dialog row already shows the full external id).

The copy value is always the id the user can use next: the **external** chat id (what `/join` and the notify API expect) or the bot uuid. The internal access uuid (`DirectChat.id`/`Group.id`) is available only in a labelled "Internal target UUID" detail line, never as the default copy value and never in a confirm dialog.

Plain-text channels (§3.5) never shorten.

### 3.4 Unresolved and deleted entities [PROPOSED, see D2]

The id is the only thing guaranteed to exist, so it is the universal fallback.

| Situation | Render |
|---|---|
| Entity exists, no alias | `平台 · id` (the format with the alias omitted) |
| Referenced entity was deleted (binding chat, route target, `default_agent`, binding `bot_uuid`) | `平台 · id` + muted `(deleted)`; platform comes from the stored reference, not from the deleted row |
| Still loading | skeleton, not blank, not a half-rendered format |

No snapshot of the last-known alias is kept in this iteration (§10 D2).

### 3.5 Plain-text channels (logs, CLI, IM messages, toasts, confirms) [DERIVED from "everywhere"; log field names PROPOSED]

The same segment order is used wherever text is emitted. Differences from the UI:

| Channel | Rule |
|---|---|
| Toasts, confirm dialogs, aria-labels | Same format, id shortened as in the UI. Must name the entity ("Bot `Telegram · Ops bot · …456789` restarted", `Delete Feishu · Dev team · 群 · oc_a1…9z?`). The full id is in the confirm body when the action is destructive. |
| IM messages the bot sends | Same format with the **full** id wherever the user must paste it (`Your ID`, `Chat ID`, `/join <ChatID>`). Never shortened. |
| CLI | Same format, full id. `bot list` column "ID" (list index) is renamed "#". |
| Logs | Structured fields remain the primary form so logs stay greppable: `bot_uuid`, `bot_name`, `platform`, `chat_id` (always the **external** id), `chat_alias`, `actor_id` (external). Internal access uuids are renamed `access_target_id` / `access_actor_id` so they cannot be mistaken for `chat_id`. Where a free-text rendering is needed, use the same format with the full id. Never shortened. |

### 3.6 Terminology [DECIDED: one format; PROPOSED: "Alias" as the single word]

- **Alias** is the single user-facing word for the editable name of a bot, chat, group or actor. Today the bot field is "Alias" in the edit dialog and "Name" in the table (R-B §6); both become "Alias". The API field stays `name` for bots and `alias` for chats (no breaking rename).
- **ID** in UI/CLI always means the identifier the user can use next (external id / bot uuid).
- The `群` segment only ever means "group chat"; it is not a bot or platform attribute.

## 4. Naming model [DECIDED rule; DERIVED invariants; PROPOSED storage]

### 4.1 Rule [DECIDED]

- If the platform provides a name for an entity, it is stored as that entity's **default alias**.
- If it provides none, the alias stays **empty** and the id is displayed, until the user sets an alias.
- A user-set alias is authoritative from then on.

Applies to: bot (platform account name, see §4.5), direct chat (the peer's display name), group (the platform chat title), actor (sender display name).

### 4.2 Invariants (must hold in every write path) [DERIVED; N1/N2 are explicit requirements from the coordinator relaying the user's decision]

| # | Invariant | Currently violated at |
|---|---|---|
| N1 | **An empty inbound name never overwrites an existing value.** Only a non-empty, changed name is written. | `internal/db/bot_access_store.go:246` `upsertActor` writes `display_name = <incoming>` unconditionally, so a nameless message wipes a good name |
| N2 | **An inbound platform name never overwrites a user-set alias.** | not enforceable today: `DirectChat` has no platform name field and `Group.name` / `alias` are separate, so the question never arose; it becomes real once platform names are stored as default alias |
| N3 | **A platform-sourced alias may be refreshed** by a later non-empty, different platform name (group renamed on the platform), but only while its source is `platform`. | `DiscoverGroup` (`bot_access_store.go:477-505`) returns early for existing rows, so group names never refresh |
| N4 | **The platform "name" must be a real name.** If an adapter returns the raw id as the display name, it is treated as no name. | `imbot/platform/feishu/adapter.go:72` sets `DisplayName: detail.SenderID` |
| N5 | **Clearing a user alias resets it to the latest known platform name** (or empty if none), and the source returns to `platform`. | n/a (new) |

N4 is enforced twice: adapters return `""` when they have no name, and the store ignores any incoming name equal to the external id.

### 4.3 Storage [PROPOSED]

Single display field plus a source marker, reusing the existing raw-name columns as the platform-name cache:

| Entity | Display field | Source marker | Raw platform-name cache (never wiped by empty) |
|---|---|---|---|
| Direct chat | `alias` (exists) | **new** `alias_source` = `user` \| `platform` \| `` | peer `Actor.display_name` (exists) |
| Group | `alias` (exists) | **new** `alias_source` | `Group.name` (exists, now refreshed per N3) |
| Actor | `display_name` (exists) | **new** `display_name_source` (set to `user` only by the manual `PUT …/actors/:actor`) | itself |
| Bot | `name` (exists) | **new** `name_source` | n/a, adapter supplies it on connect |

How it works:

- On discovery/first message/pairing: if `alias` is empty and a non-empty platform name arrives, set `alias := name`, `alias_source := platform`.
- On a later message: if `alias_source = platform` (or `alias` empty) and the incoming name is non-empty and different, update both the alias and the cache. If `alias_source = user`, update only the cache.
- `PUT …/alias` with a non-empty value sets `alias_source := user`. With an empty value it applies N5: `alias := cache`, `alias_source := platform` (or `alias := ""` if the cache is empty).
- Group actors keep `GroupActor.label` as a user-only, group-scoped override. It is stored today and never displayed; the actor label resolves as `label` → `display_name` → external id.

Considered and rejected: keeping alias and platform name as two fields and resolving `alias → name → id` at read time. It needs no migration and cannot overwrite the user by construction, but it contradicts the user's stated model ("store as the default alias") and leaves two near-identical fields in every API response. The source marker gives the same safety with one field.

### 4.4 Migration [PROPOSED]

| Existing data | Becomes |
|---|---|
| `DirectChat.alias` non-empty, `Group.alias` non-empty | `alias_source = user` (they can only have come from `PUT …/alias`) |
| `Group.alias` empty, `Group.name` non-empty | `alias := name`, `alias_source = platform` |
| `DirectChat.alias` empty, peer `Actor.display_name` non-empty and not equal to the external id | `alias := display_name`, `alias_source = platform` |
| `Actor.display_name` equal to its external id (feishu) | cleared (N4) |
| Names already wiped by N1 | not recoverable; refill on the next inbound message |
| Bot `name` equal to `"<platform id> Bot"` (generated default) | cleared to empty, `name_source` empty (§4.5); the bot then displays by id |

### 4.5 Bots [DECIDED (D4, user confirmed 2026-11-03); details PROPOSED]

Decided: a bot's default name is its platform account name; if the platform gives none, the name stays empty. The platform-side identity exists on connect (Telegram `@username`, Discord username, Slack bot user, WeCom bot id: these are already logged at connect, R-C §3). How to implement it:

- When the user leaves the name empty, do **not** write `"<platform id> Bot"` any more (`BotConfigDialog.tsx:163,252`, `wechat_qr.go:330-335`, `feishu_qr.go:320-322`).
- After a successful connect, if the bot has no name and the adapter exposes an account name, store it as the default (`name_source = platform`). Otherwise leave the name empty; the bot displays as `Telegram · …456789`.
- Allow clearing a bot name (`imbot_settings_store.go:181` currently ignores empty).
- Because the id is part of every display, two bots with the same name (or both unnamed) are told apart by their id segment. No separate disambiguation suffix is needed.

The user confirmed this extension of the naming model to bots on 2026-11-03 (D4).

## 5. Resolver and API [PROPOSED]

The UI needs the segments for entities it did not load: route target, binding chat, thread-event source, `peer_actor_id`, `default_agent`, a binding's `bot_uuid` (R-A §2). Options:

| | 1: server resolves, embeds in responses | 2: client joins from lists it already has | 3: separate batch resolve endpoint |
|---|---|---|---|
| Covers Projects bindings / transcript / route targets | yes | no, needs join logic inside the Projects package (which must not import the root module, `CLAUDE.md`) | yes |
| Serves logs, CLI, IM messages | yes (same Go resolver) | no | partly |
| Deleted entities | natural (`resolved:false`) | client has to guess | natural |
| Cost | M–L (response structs, swagger, `task codegen`) | S–M per page, repeated | M, plus an extra round trip |

**Recommendation: option 1**, with a thin client cache. Option 3 is the fallback if response shapes get too wide.

### 5.1 Shape: segments, not a baked string

The component needs to style segments and shorten the id, so responses carry parts. Text channels reuse the same struct through a `String()`.

```go
type DisplayRef struct {
    Kind          string `json:"kind"`                    // "bot" | "direct" | "group" | "actor"
    Platform      string `json:"platform"`                // raw id, for icon lookup
    PlatformLabel string `json:"platform_label"`          // 平台
    Alias         string `json:"alias,omitempty"`         // omitted when empty
    ID            string `json:"id"`                      // full, usable id (external / bot uuid)
    Group         bool   `json:"group,omitempty"`         // drives the 群 segment
    Resolved      bool   `json:"resolved"`                // false = entity gone, only ids known
}
func (d DisplayRef) String() string // "Feishu · Dev team · 群 · oc_a1b2…9z" with the full id
```

It lives next to the access domain types (`remote/access`), reads existing tables only (`imbot_settings`, `remote_direct_chats`, `remote_groups`, `remote_actors`), caches per request, and is the only place the §3 rules exist on the server. Logging helpers and IM message builders call the same type.

For Projects, which cannot import the root module: extend its `Host` interface (`internal/server/module/projects/host.go`) with `ChatRef(platform, chatID, botUUID)` and `BotRef(uuid)`, implemented by the root module with the resolver. Whether `Host` already has a usable hook is UNVERIFIED (R §5).

### 5.2 API changes (swagger-defined, then `task codegen`)

Additive only; existing fields stay.

| Response | Add | Replaces |
|---|---|---|
| `GET /bots/:bot/chats[/:chat]` (`DirectChatDetailResponse`) | `ref` | `alias \|\| external_chat_id` in `imHost.tsx:48`, Access dialog rows |
| `GET /bots/:bot/groups[/:group]` | `ref` | `alias \|\| name \|\| id` in 3 places |
| `Route` | `target_ref` | UI join across chats and groups |
| Group-actor list | `actor.ref` | `display_name \|\| id` |
| Projects `ChannelBinding` | `chat_ref`, `bot_ref`; `chat_title` becomes a plain fallback | `chat_title \|\| chat_id` (`ProjectImBindings.tsx:75,107`) |
| Projects `ThreadEvent` (IM source) | `source_chat_ref` | `ConversationPane.tsx:356`, `ThreadPanel.tsx:480` |
| Bot `Settings` | `ref`, `default_agent_ref` (profile name + id) | `name \|\| platform` (9 sites), profile join |
| Pairing-code, QR-status, reload responses | `bot_ref` | toasts with no entity |

Smallest useful subset if scope has to shrink: `ref` on bot/chat/group, `chat_ref`/`source_chat_ref` on Projects, `target_ref` on Route.

## 6. Frontend [PROPOSED]

### 6.1 One presentational component

`EntityLabel` in `@tingly/ui` (shared by the root app and the Projects pages, per `CLAUDE.md`). It renders a `DisplayRef`, fetches nothing:

```tsx
<EntityLabel
  ref={ref}                // DisplayRef from the API (§5.1)
  maxIdLength="auto"       // applies shortId(); hides the id behind copy/tooltip when narrow
  copy                     // CopyIconButton, always visible, copies ref.id
  platformAs="text|icon"   // icon form for lists already under a bot header
/>
```

It generalises the existing `rowHeader(title, id)` in `BotAccessDialog.tsx:48`, the best current pattern (bold alias, mono external id beneath, id hidden if equal). Behaviour that follows from §3:

- Segments in the fixed order `platform · alias · 群 · id`; every empty or inapplicable segment omitted with its separator.
- `resolved: false` adds a muted `(deleted)`.
- A tooltip always carries the full id and, where relevant, a labelled "Internal target UUID" line.
- Select **input value and option** use the same component, so the two never disagree (today `NotifyTestDialog.tsx:108` vs `:118`).
- `shortId(kind, id)` lives beside it with unit tests for both rules in §3.3.

### 6.2 Data layer

Mostly read-through: the server already supplies the segments. `frontend/src/services/labels.ts` holds `shortId`, `refText(ref)` for toasts/confirms/aria-labels, and nothing else. The four hand-copied platform-name sources collapse into `platform_label`: delete `PLATFORM_NAMES` and the `layout.platforms.*` fallback in `imHost.tsx:18-26`, and the raw-id displays in `BotTable.tsx:203,275` and `BotNotifyGroup.tsx:399-400`.

### 6.3 Projects package

- `ImHost` gets `chatRef`/`botRef`. The optional `chatTitle` hook (`ProjectsHost.tsx:82`) that `ThreadPanel.tsx:480` already calls but `imHost.tsx` never implements is replaced by it. Delete the second resolver so `ConversationPane.tsx:356` and `ThreadPanel.tsx:480` share one path.
- `bind` sends the resolved alias as `chat_title` (`useImBindings.ts:58`) so the stored snapshot is not empty.
- `chatKey` includes `bot_uuid` (`useImBindings.ts:13`), otherwise two bots on one platform that see the same chat id collide.

### 6.4 Site targets

Every site in R-B moves to `EntityLabel` or `refText`. The ones with the largest visible change:

| Surface | Today | After |
|---|---|---|
| Notify delete-chat confirm (`BotNotifyGroup.tsx:593`) | **internal uuid** | `Delete Feishu · Dev team · 群 · oc_a1…9z?` with the full external id in the body |
| Notify chat leaf (`ChatNode.tsx:60`), target mapping (`BotNotifyGroup.tsx:137-158`) | external id; alias dropped; group `name` only | full format; tooltip keeps both ids, labelled |
| Test-send picker (`NotifyTestDialog.tsx:108,118`) | name in input, id in options | same `EntityLabel` in both |
| Remote card / Notify card header / Bots table | `name \|\| platform` (raw id), uuid mostly invisible | `Telegram · Ops bot · …456789`, copy button for the uuid |
| Projects tags, "From …" lines, transcript | raw chat id | full format from `chat_ref`/`source_chat_ref` |
| Projects IM settings rows, bind/move dialogs | `Platform · alias` (id hidden) or title only | full format, external id reachable via copy |
| ~20 toasts, icon-only row buttons | name no entity | `refText` in message / `aria-label` |
| Access dialog Rename, CC profile dialog titles | name no entity | include the entity |
| Actor chips (`BotAccessDialog.tsx:325`) | `display_name \|\| id` | full format, `label` first |

## 7. Backend text channels [PROPOSED]

Concrete work (line references in R-C):

1. **`logfields` helper** producing the §3.5 keys (`bot_uuid`, `bot_name`, `platform`, `chat_id`, `chat_alias`). Migrate the ~80 bot/chat log sites over time, starting with lifecycle, authorization, pairing and the imprompter "auto-approving" line. Normalise `uuid|bot|botUUID|channel` → `bot_uuid` and `chatID|target` → `chat_id`.
2. **imbot per-bot logger prefix**, applied once instead of per call site: add identity to the logger built from `core.Config` (or inject a prefix from `remote/control/bot`), e.g. `[imbot telegram/…456789 Ops bot]`, extending the existing `[%s/%s]` panic prefix at `imbot/core/base.go:473`. Covers all 85 adapter lines (`imbot/core/logger.go:60-70`).
3. **IM messages**: `/status` (`projects/internal/project/chat_commands.go:193-224`) gains `Bot:` and `Profile:` lines; `/help` (`remote/control/remoteagent/project_help.go:8-28`) gains the bot line and the chat's own `DisplayRef`; pairing/authorisation replies name the bot (`handler_pair.go:29`, `command.go:330`, `manager.go:140`). Full ids stay where users paste them.
4. **CLI**: echo bot name + uuid after `bot add`, `pair`, `config`; accept name, uuid or `#index` in `resolveBotRef`; rename the `bot list` "ID" column to "#"; make `/bind` vs `/pair` consistent between `bot_add.go:87`, `bot_pair.go:39` and the banner at `manager.go:178-181`; add the uuid to that stderr banner, which `bot pair status` tells operators to grep.
5. **Errors that reach the UI** (`ErrNoChannel: bot <uuid>`, "bot for platform %s is missing credentials"): resolve to the format before returning. Keep the deliberately uniform notify/interact 404s unchanged (anti-probe, `bot_api.go:166-192`).
6. **Audit events** (`bot.authorization.decision`, `imbot.pair.*`): include `bot_name`, `chat_alias`, route name at write time, since they are logrus-only and cannot be relabelled later.

## 8. Order of work [PROPOSED]

Display is the goal, but it depends on the data being real, so the naming invariants come first.

| Phase | Content | Size | Depends on |
|---|---|---|---|
| **P0 – bug fixes** (optional, can ship alone) | Delete-confirm shows internal uuid; picker input/option mismatch; map alias onto Notify targets; send `chat_title` on bind; `chatKey` includes bot; stale i18n at `en/remote.ts:400`; raw platform id in Notify empty state | S, 1–2 days | nothing |
| **P1 – naming model** | §4: N1–N5, `alias_source`, migration, adapters return `""` not id, stop writing `"<platform> Bot"`, bot name clearable | S–M | nothing |
| **P2 – resolver + API** | `DisplayRef`, `ref`/`*_ref` fields, Projects `Host` refs, swagger + `task codegen` | M | P1 (for good data) |
| **P3 – shared UI** | `EntityLabel`, `shortId`, migrate every site in R-B (bots, Notify, Remote, Projects, toasts, aria-labels) | M | P2 |
| **P4 – text channels** | §7: logfields, imbot prefix, IM messages, CLI | M | P2 (resolver); parallel to P3 |
| **P5 – lifecycle** | Real sender names on Feishu/Weixin/WeCom, dangling-reference cleanup, persisted audit | M, partly blocked by D1, D2 | later |

The display effect the user is after lands with P3; P1 and P2 are what make it show real names instead of ids.

## 9. Risks and trade-offs [context]

- **Privacy.** Platform names become a stored default alias (they are already stored on `Actor`). IM messages that print the bot or chat label are visible to every member of the chat (§10 D3).
- **Platform refresh vs surprise.** A platform-sourced alias changes when the platform name changes (N3). A user who finds that surprising sets their own alias once and it sticks (N2).
- **Visual noise (ux-principles §9).** The platform segment on every row is redundant under a bot header. Mitigation: icon form, with the text in the tooltip (§10 D6).
- **Log cost.** Name lookups per log line must come from the in-memory bot setting already in scope, not a DB read. imbot adapters get the identity once at construction; a bot rename affects the prefix after restart, while the uuid stays correct.
- **Scope.** Sharing keys / API tokens are out of scope: no bot/IM relation (R §1).

## 10. Remaining questions and the defaults taken [DECIDED (delegated 2026-11-03): defaults below stand unless the user overrides]

The user's main concern is the display effect, so these are sorted by whether they block it. Each has a default I will use unless overridden. **Status of every row: DECIDED (D4 by the user; the rest delegated to me and resolved with the recommended default).**

The four questions the user was asked about (Feishu scopes, deleted entities, bot naming itself in IM, PR split) are **D1, D2, D3, D7**. D4, D5, D6 are points I found while applying the user's format and rule.

**D4 is now DECIDED** (user confirmed 2026-11-03): bots default to their platform account name, else empty. See §0 item 3 and §4.5. It stays in the table below for traceability.

### Blocks the display work (confirm before building P3)

| # | Question | Recommended default | Why it matters |
|---|---|---|---|
| D4 (mine) — **DECIDED** | Bots: when the name is empty, should the platform account name (Telegram `@username` etc.) become the default alias (§4.5), or stay empty and show only `Telegram · …456789`? | **Decided:** store the platform account name when the adapter exposes one; otherwise empty. Stop writing `"<platform> Bot"`. | Decides what an unnamed bot looks like everywhere. |
| D5 (mine) — **DECIDED (delegated)** | **How the `群` segment renders.** It follows the alias (the earlier trailing `[group]` and the second-position form no longer apply). Options: (a) localised word — `群` in zh, `Group` in en (ru: `Группа`); (b) icon only (users/people icon); (c) word plus icon. Also: show the platform's own kind (`supergroup`, `channel`, `topic`) instead? | **(a) localised word**, rendered in the UI as a small muted outlined chip right after the platform; plain text (logs, CLI, IM, toasts) uses the bare word: `Feishu · Dev team · 群 · oc_a1…9z`. A word reads in every channel, including text-only ones where an icon cannot appear, and an icon next to the platform icon would be two pictures in a row that users must decode. Use the generic word, not platform kinds: the data model only knows direct vs group. | Affects the component and i18n strings. |

### Can wait (a default exists, nothing on the display path depends on it)

| # | Question | Recommended default | Blocks display? |
|---|---|---|---|
| D1 — **DECIDED (delegated)** | **Feishu / Weixin / WeCom names.** Real sender names on Feishu need contact-API permission on the user's app; Weixin/WeCom may expose none. | Do not require new scopes in the first iteration. Platforms that already return names (Telegram, Discord, Slack, DingTalk) work immediately. For Feishu, try a lazy lookup and fall back to the id on permission error (one warning, not per message). Weixin/WeCom stay id-only. The `platform · id` format already covers them. Which Feishu scope and API call is needed is UNVERIFIED. | No. Falls back to the id. |
| D2 — **DECIDED (delegated)** | **Deleted entities.** Keep a last-known-alias snapshot, or show only the id? And on bot delete: cascade-delete Projects bindings? | Show `platform · id (deleted)`; no new snapshot column (the existing `chat_title`, filled at bind time, is a free fallback). Do not cascade-delete bindings in this work; they render as unresolved. Cleanup is a separate change. | No, the unresolved rendering is already specified in §3.4. |
| D3 — **DECIDED (delegated)** | **Should the bot name itself in IM** (`/status`, `/help`, pairing replies)? These labels are visible to everyone in the chat. | Yes for the bot's own label (it is the account the user is talking to). Chat alias only in DMs, because an alias is an operator's private note and a group member would see it. | No, it lives in P4. |
| D6 (mine) — **DECIDED (delegated)** | **Platform segment under a bot header.** The user asked for the format everywhere; in a per-bot list the platform repeats on every row. | Keep the segment, rendered as the platform icon (text in the tooltip) in lists already grouped under a bot header. Full text everywhere else. Still fits: the row reads `[icon] alias · 群 · id`, the group chip right after the name; only the platform's form changes. | No, a single prop on the component. |
| D7 — **DECIDED (delegated)** | **PR split.** | PR0 = P0 bug fixes (optional, immediate). PR1 = P1 naming model (backend, small, correctness). PR2 = P2 + P3, the visible display change. PR3 = P4 text channels. | No. |

## 11. UX-principles checklist (`ux-principles.md`)

- [x] Organised around the user's questions: "which chat/bot is this?" is answered by the format, "how do I reference it?" by the id segment and copy.
- [x] No mode picker: one format; truncation is automatic, not a setting.
- [x] One word per concept (§3.6); the external-id vs internal-uuid collision is split (§3.3, §3.5 key rename).
- [x] Orthogonal axes separated: platform · alias · 群 · id are distinct segments; alias source (user vs platform) is its own marker.
- [x] Concrete value vs alias (§5): both are shown, in a fixed order. The id is never hidden without a copy path. This resolves the tension between alias-first display and the "show the concrete value" principle.
- [x] Smart defaults: platform name becomes the default alias (user's rule), no toggle.
- [x] Noise reduced (§9): muted platform and id, alias as anchor, `群` only when it carries information, icon form under a bot header.
- [x] Next-action artifact: the copy value is the id the user can use next.
- [x] Side effects scoped: a rename propagates through the resolver; platform names never overwrite the user's alias.
- Deviation: none that needs justification.

## 12. Decision log

| Date | Item | Status | Source |
|---|---|---|---|
| 2026-11-03 | Display format `平台 · alias · id [type]` everywhere; alias omitted when empty; `[type]` for group chats only | SUPERSEDED by the next row | user |
| 2026-11-03 | Display format `平台 · 群 · alias · id` everywhere; any empty or inapplicable segment omitted; `群` only for group chats; platform shows its display name | SUPERSEDED by the next row | user |
| 2026-11-04 | Display format `平台 · alias · 群 · id` (the group marker moves after the alias; "群在中间会很奇怪") | DECIDED (supersedes `平台 · 群 · alias · id`) | user |
| 2026-11-03 | Long ids may be shortened or omitted in display, with another way to get the full id | DECIDED | user |
| 2026-11-03 | Platform name stored as default alias; no name → empty alias, show id, until the user sets one | DECIDED | user |
| 2026-11-03 | A user-set alias is never overwritten by inbound names; an empty inbound name never wipes a value | DECIDED (requirement relayed by coordinator from the user's decision) | coordinator |
| 2026-11-03 | D1 Feishu/Weixin/WeCom names: no new scopes in v1, fall back to id | DECIDED (delegated by the user) | me |
| 2026-11-03 | D2 Deleted entities: `platform · id (deleted)`, no snapshot column, no cascade | DECIDED (delegated by the user) | me |
| 2026-11-03 | D3 Bot names itself in IM; chat alias only in DMs | DECIDED (delegated by the user) | me |
| 2026-11-03 | D4 Bot default name from platform account, else empty; drop `"<platform> Bot"` | DECIDED (user confirmed) | user |
| 2026-11-03 | D5 (old) `[type]` is literal `[group]`, localised | WITHDRAWN (trailing type removed by the new order) | me |
| 2026-11-03 | D5 `群` segment renders as a localised word (`群` / `Group`), small chip in the UI, bare word in text | DECIDED (delegated by the user) | me |
| 2026-11-03 | D6 Platform segment as icon under a bot header (checked: still fits the new order) | DECIDED (delegated by the user) | me |
| 2026-11-03 | D7 PR split PR0/PR1/PR2/PR3 | DECIDED (delegated by the user) | me |
| 2026-11-03 | Build order followed: P0, P1, P1b, P3, P4 text channels, then logfields, `/status`, Feishu names, deleted-binding display; server `ref` API fields dropped (§14) | DECIDED (implementation) | me |
| 2026-11-03 | `/status` carries Bot and Profile only, no chat line | DECIDED (implementation, from D3) | me |

## 13. Visuals

Screenshots were taken in frontend mock mode (headless Chromium, mock data extended with two bots that share the stored default name `telegram Bot` and with aliases on chats; the scratch mock edits were not committed). Files are in [`images/bot-im-alias-display/`](./images/bot-im-alias-display/); the mockup is a static page, [`mockup.html`](./images/bot-im-alias-display/mockup.html), not product code.

**Today (problem sites)**

| Image | What it shows |
|---|---|
| [current-1](./images/bot-im-alias-display/current-1-bots-table-name-collision.png) | Bots table: two bots both named `telegram Bot`, raw platform id as the chip, bot ids truncated with no way to tell them apart |
| [current-2](./images/bot-im-alias-display/current-2-notify-chats-alias-dropped.png) | Notify graph: the chats have aliases (`Alice (ops)`, `Bob (on-call)`, `Releases`) but the nodes show `telegram:123456789`; the group is `Release Crew` from its platform name |
| [current-3](./images/bot-im-alias-display/current-3-notify-delete-confirm-internal-uuid.png) | Delete-chat confirm shows the internal uuid `direct-1` |
| [current-4](./images/bot-im-alias-display/current-4-notify-test-picker.png) | Test-send picker lists raw `telegram:…` ids, no alias |
| [current-5](./images/bot-im-alias-display/current-5-projects-im-rows.png) | Projects IM rows: `Telegram · alias` with the external id not visible anywhere |
| [current-6](./images/bot-im-alias-display/current-6-project-chat-tag-raw-id.png) | Project header tag and "From Telegram · 123456789" show the raw chat id although the chat has an alias |

**`平台 · alias · 群 · id` (mockup; the real-UI shots are `ui-*`)**

| Image | What it shows |
|---|---|
| [new-1](./images/bot-im-alias-display/new-1-format-bots-chats.png) | The four segments; bots with platform-account alias, no alias, user alias; chats direct/group/no-alias/deleted; a shortened Feishu id with the hover tooltip (full id, copy button, internal uuid as a labelled detail) |
| [new-2](./images/bot-im-alias-display/new-2-confirm-and-picker.png) | Delete confirm naming the chat with its external id; test-send picker where input and options use the same label |
| [new-3](./images/bot-im-alias-display/new-3-projects.png) | Projects tag and message-source line; group tags with and without alias; the icon-only platform form under a bot header (D6) |

**Built (real UI, mock mode, current order)**

| Image | What it shows |
|---|---|
| [ui-2](./images/bot-im-alias-display/ui-2-notify-chats.png) | Notify graph: aliased chats, group node, long id shortened with the full id in the tooltip |
| [ui-7](./images/bot-im-alias-display/ui-7-projects-im-settings.png) | Projects IM settings: `Release Crew · Group · -10024681012`, an unnamed Feishu group, copy buttons |
| [ui-9](./images/bot-im-alias-display/ui-9-project-docs-deleted-tag.png) | Project header tags: compact form, and a chat deleted in Remote marked `(deleted)` |

## 14. Implementation status and deviations (2026-11-03)

Built on the branch with tests (Go: `remote/access`, `remote/logfields`, `internal/db`, `imbot/core`, `imbot/platform/feishu`, `remote/control/{bot,remoteagent}`; frontend: `@tingly/ui`, the Projects package, `imHost`). `openapi.json` was regenerated in an isolated HOME and `pnpm gen:api` run; the generated file also picked up routes that were missing from the committed one before this work.

| Phase | State | Notes |
|---|---|---|
| P0 bug fixes | done | delete confirm no longer shows the internal uuid; picker input/option agree; aliases reach the Notify graph; `chat_title` is sent on bind; the host implements the chat lookup the transcript already called |
| P1 naming model | done | `alias_source` / `display_name_source`, one-time backfill, N1–N5 enforced and tested; a direct chat takes its peer's platform name as default alias on the first inbound message |
| P1b bot names | done | no `"<platform> Bot"` defaults (QR flows, CLI, UI); explicit empty name clears; an unnamed bot takes the adapter's account name on connect (Telegram, Discord, Slack) |
| P2 resolver + API | **changed** | see deviations |
| P3 shared UI | done | `EntityLabel` (with a `compact` form for tags), `shortId`, `entityText` in `@tingly/ui`; `useEntityLabels` in the app; applied to the Bots table, Remote and Notify cards, Access dialog, notify picker/confirm/graph nodes, Projects tags, "From …" lines, IM settings and the bind dialog; ~12 toasts name the entity |
| P4 text channels | done | `/help` and `/status` name the bot (`/status` also the Claude Code profile); pairing confirmation names the bot; every imbot adapter line carries `[imbot <platform>/<short id>]`; the stderr pairing banner includes the uuid; CLI says `/pair` and labels the index column `#`; `remote/logfields` is the one key set (`bot_uuid`, `bot_name`, `chat_id` = external id, `chat_alias`, `access_target_id` / `access_actor_id` for internal ids, `ilink_bot_id`) |
| D1 Feishu names | done | contact (raw request) and chat lookups behind a cache; a missing scope is logged once and not retried for 30 minutes; Weixin/WeCom stay id-only |
| D2 deleted entities | done (display) | a binding whose chat or bot is gone shows its saved alias and id with `(deleted)`, decided only from a complete listing; nothing is cascade-deleted |
| P5 persisted audit events | not started | audit events are still log-only |

Deviations from the plan above (decided while building):

- **No server `DisplayRef` / `*_ref` API fields (§5).** Once platform names are stored as default aliases (P1), the alias is already on `DirectChat` / `Group`, so the client resolves labels from lists it already loads (`useEntityLabels`, and the host's lazily filled chat index for Projects). Only `alias_source` / `display_name_source` were added to the API.
- **Bots have no `name_source`.** A bot's name is only filled when empty and never refreshed, so provenance would carry no information.
- **Projects `chatKey` still ignores the bot.** A binding is unique per `(platform, chat_id)` on the server, so adding the bot to the key would hide a real conflict instead of fixing one.
- **Graph nodes and tags use compact forms.** In the Notify graph the platform is the preceding node, so the node shows alias over short id; project tags (`compact`) drop the id when there is an alias and keep the full label as tooltip and accessible name, so the alias is not squeezed in a narrow header.
- **Log keys: `user_id` stays.** About 20 pairing/auth lines log the external sender under `user_id`; it already means the external id, so it was not renamed to `actor_id` (churn without a gain). Free-text log lines in the imbot manager use the shared `platform/…tail` tag.
- **`/status` shows no chat line.** The bot and profile lines were added; a chat alias would be visible to a whole group (D3: alias only in DMs) and in a DM it is the user's own chat.
- **Bot account name after a failed first connect.** The name is read once, right after the first connect. If that connect fails and an auto-reconnect later succeeds, the bot stays unnamed until the next restart (display falls back to the id). Not worth a polling loop.
- **Feishu webhook path** (`Adapter.AdaptWebhook`) still carries no name; only the WebSocket path looks names up.
- **No dangling-reference cleanup.** Per D2 nothing is deleted when a bot or profile is removed; references render as `(deleted)` (bindings in Projects, profile in `/status`) instead.
