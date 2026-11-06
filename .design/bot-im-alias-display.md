# Bot / IM display convention: `平台 · alias · id` (groups: `平台群 · alias · id`)

> Status: implemented for the UI and the naming model (this document describes what shipped).
> Follow-ups that were split out are listed in §7.
> Judged against [`ux-principles.md`](./ux-principles.md) (§3 one word per concept, §5 show the concrete value, §9 reduce noise).
> Related: [`bot-capability-access-control.md`](./bot-capability-access-control.md), [`projects-im.md`](./projects-im.md).

## 1. Problem

Bots and IM chats have an ID *and* a human name, but the product showed them inconsistently:

- The same chat had up to five different labels across surfaces; Projects showed the raw platform chat id even after the user named the chat.
- Wrong fields: the Notify delete confirm printed an **internal uuid**; the test-send picker showed a name in the input and a raw id in its options; the Notify graph dropped chat aliases.
- Names were empty or wiped at the source: every inbound message overwrote the actor's `display_name`, even with an empty value (`internal/db/bot_access_store.go`), Feishu reported the raw sender id as a "name", and direct chats had no name column at all.
- Where users need the id (paste into `/join`, the notify API, compare with the IM app) it was often hidden.

Screenshots of the old state: `images/bot-im-alias-display/current-1…6`.

## 2. The display format (decided by the user)

```
平台 · alias · id          direct chat, bot
平台群 · alias · id        group chat: "Feishu group · Dev team · oc_a1b…d4e5", "飞书群 · Dev team · oc_a1b…d4e5"
```

| Segment | Content | Rule |
|---|---|---|
| 平台 | platform **display name** ("Telegram", "Feishu"), never the raw id (`telegram`) | For a group chat the segment carries the marker: "Feishu group" / 飞书群 / Группа Feishu. Under a bot header the platform is a logo alone (`platformAs="icon"`); a group row then keeps a small `群` / `Group` mark right after the logo, with no separator. |
| alias | the entity's alias | Omitted when empty; never replaced by a placeholder. |
| id | the id the user can use next: external chat id, bot uuid | Always present, monospace. May be shortened; the **full id is always reachable** (tooltip, copy button). |

Any empty or inapplicable segment is dropped with its separator. The group marker is **never a segment of its own**.

Why the marker lives in the platform segment (history: `平台 · alias · id [type]` → `平台 · 群 · alias · id` → `平台 · alias · 群 · id` → this): "group" says what *kind* of conversation it is, not a fact about it like the alias or id, so as a dot-separated peer it read oddly wherever it sat (end: too far from the name; before the alias: interrupts platform → name; after the alias: wedged between name and id). Joined to the platform it reads as one noun phrase ("Feishu group"), keeps the important information first, works as plain text for toasts and aria-labels without a chip, and costs the single-character-wide `群` only on group chats. Cost: the wording is a per-locale pattern (`entity.group` = `{{platform}} group` / `{{platform}}群` / `Группа {{platform}}`) rather than a fixed word, and the logo-only form needs the small mark.

Examples: `Telegram · Alice · 123456789`, `Feishu group · Dev team · oc_a1b…d4e5`, `Telegram · …456789` (an unnamed bot).

### 2.1 Which entities

Platform-scoped entities (bot, direct chat, group, actor) use the format. Routes, Claude Code profiles and projects keep their own simpler labels.

### 2.2 Shortening and the full id

Display shortening is width-driven and done in one helper (`shortId`):

- **Bot ids** are generated as `<unixnano>-imbot`; the head digits are nearly identical for bots created close together and the suffix is constant, so only the tail is shown: `1730600000123456789-imbot` → `…456789`.
- **External chat / actor ids**: up to 20 characters whole, longer as `first 6 … last 4`.

The copy value is always the id the user can use next: the **external** chat id (what `/join` and the notify API take) or the bot uuid — never the internal access uuid, which only appears as a labelled "Internal target UUID" line in tooltips. Plain-text uses (toasts, confirms, aria-labels) use the same order; the accessible name always carries the full id.

### 2.3 Unresolved and deleted entities

The id is the only thing guaranteed to exist, so it is the universal fallback. A Projects binding whose chat or bot is gone shows its saved alias and id with a muted `(deleted)`. "Deleted" is only claimed after a **complete** listing of every bot and all its chats, and a chat missing from one listing is re-checked by a second one first, so a chat that appeared a moment ago is never mislabelled. Nothing is deleted when a bot is removed.

## 3. Naming model (decided by the user)

If the platform supplies a name for an entity it is stored as that entity's **default alias**; if it supplies none the alias stays empty and the id is shown until the user sets one. Applies to direct chats (the peer's display name), groups (the platform title) and actors.

Invariants, enforced in every write path and tested:

| # | Invariant |
|---|---|
| N1 | An empty inbound name never overwrites an existing value. |
| N2 | An inbound platform name never overwrites a user-set alias. |
| N3 | A platform-sourced alias is refreshed by a later, different, non-empty platform name (a group renamed on the platform). |
| N4 | A "name" that is just the entity's external id counts as no name (Feishu's adapter now returns none instead of the sender id). |
| N5 | Clearing a user alias resets it to the latest platform name, or empty if there is none. |

Storage: one display field plus a provenance marker — `alias_source` on `remote_direct_chats` and `remote_groups`, `display_name_source` on `remote_actors` (`user` / `platform` / empty). The raw platform name stays in the existing columns (`Group.name`, `Actor.display_name`). An alias of **unknown origin** (non-empty, no source) is protected like a user alias.

### 3.1 Migration

Three nullable columns, added and backfilled **once**, in one transaction per table (a failed backfill rolls its column back, so the next start retries instead of silently skipping it):

- Existing non-empty aliases → `user`.
- An empty group alias takes the group's platform name (`platform`).
- An empty direct-chat alias takes its peer actor's name (`platform`).
- Actor names equal to the external id (or its part after `platform:`) are cleared (plain string comparison, not `LIKE`).

Additive and idempotent; older binaries ignore the new columns.

## 4. Frontend

- **`@tingly/ui`**: `EntityLabel` (presentational; `compact` for tags, `platformAs="icon"`, `copy`, `resolved`), `entityText` (the same order as plain text), `shortId`, `bareId`.
- **App adapters**: `useEntityLabels` (bot / chat / group / Notify target → props; `text`, `botText`), and in the Projects package `ImChatLabel` / `useImChatText` (the package cannot import app code).
- **Projects host**: `imHost` keeps every bot's latest listing, rebuilds a chat lookup from them (`chatInfo`), refreshes on a 60 s stale tick and after an access-dialog change (a refresh requested while one is running runs again), and reports `deleted` per §2.3.
- Applied to the Bots table, Remote and Notify cards, the Access dialog (rename dialog names the chat), notify test picker / delete confirm / graph nodes, Projects tags, "From …" source lines, IM settings and the bind dialog; ~12 toasts name the entity. Binding a chat saves its alias as `chat_title` (the last-known name for when the chat is gone).

The group marker follows §2 (merged into the platform). Screenshots of the result: `images/bot-im-alias-display/new-1…3` (mockup) and `ui-2`, `ui-7`, `ui-9`, `ui-10` (real UI, mock mode; `ui-10` is the Notify API guide picker, which now shares the same component instead of keeping its own formatter).

## 5. Decisions taken

| Item | Decision |
|---|---|
| Display order | `平台 · alias · id`; group marker merged into the platform: `平台群 · alias · id` (user) |
| Platform name = default alias; user alias never overwritten; empty name never wipes (user) | N1–N5 above |
| Bots default to their platform account name, else empty (user, D4) | separate PR, §7 |
| Group marker is a localised pattern inside the platform segment (`{{platform}} group` / `{{platform}}群`); only the logo-only form draws a small `群` mark. No separate chip, no dedicated list-context flag (a list headed "Groups" keeps the marker) | user (merge) / decided |
| Platform as a logo under a bot header | decided |
| Feishu names without new scopes in v1 | separate PR, §7 |
| Deleted entities shown by id + saved alias + `(deleted)`, no snapshot column, no cascade | decided |
| Server-side `DisplayRef` / `*_ref` API fields | **not built**: once names are stored as default aliases the alias is already on `DirectChat` / `Group`, so the client resolves labels from lists it already loads |

## 6. Known limitations

- A binding stores no chat kind, so once its chat is gone the label cannot show the group chip.
- Weixin and WeCom do not provide sender names; those chats are shown by id.
- Persisted audit events are still log-only.

## 7. Split out of this change

Built, tested and kept on the branch `tp/audit-and-design-bot-im-alias-di-c6b317`, not part of this PR:

| Piece | State |
|---|---|
| Bot takes its platform account name; no `"<platform> Bot"` default; a name can be cleared | own PR |
| Feishu sender-name / group-title lookup (cache, scope back-off) | own PR |
| One log key set (`remote/logfields`) and a per-bot `[imbot <platform>/<id>]` log prefix | deferred |
| `/help`, `/status`, pairing replies name the bot and profile; CLI wording | deferred |
