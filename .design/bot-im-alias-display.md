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

If the platform supplies a name for an entity it is shown as that entity's **default alias**; if it supplies none the id is shown until the user sets an alias. Applies to direct chats (the peer's display name), groups (the platform title) and actors.

The simplest model that gives this: **the alias stays the operator's alone, the platform's name is kept beside it, and the display is `alias || name`.** No provenance flag, no backfill.

| Entity | Operator's | Platform's | Shown |
|---|---|---|---|
| Group | `alias` | `name` (existing column) | `alias \|\| name` |
| Direct chat | `alias` | `name` (new column) | `alias \|\| name` |
| Actor | — | `display_name` | as today |

Invariants, enforced in every write path and tested:

| # | Invariant |
|---|---|
| N1 | An empty inbound name never overwrites a stored name. |
| N2 | An inbound platform name never touches the user's alias (they are different columns). |
| N3 | A later, different platform name replaces the stored platform name (a renamed group). |
| N4 | A "name" that is just the entity's external id counts as no name (`access.UsableName`; Feishu's adapter now returns none instead of the sender id). |
| N5 | Clearing the alias falls back to the platform name, or the id, for free. |

### 3.1 Migration

One nullable column, `remote_direct_chats.name`, added with the existing optional-column idiom (`HasColumn` / `AddColumn`), **no backfill**: a direct chat gets its name from the next message its peer sends, until then it shows the id (or its alias). Groups already had `name`.

Considered and dropped: provenance columns (`alias_source`, `display_name_source`) with a one-off, per-table transactional backfill (existing aliases → `user`, empty ones adopt the platform name, id-echo actor names cleared). It let a platform name become a *stored* alias that a rename could refresh, but the same result is a read-time `alias || name`, and the backfill was the riskiest part of the change. Not taken with it: existing Feishu actor names that echo the id are left as they are (the label already drops a name equal to the id).

## 4. Frontend

- **`@tingly/ui`**: `EntityLabel` (presentational; `compact` for tags, `platformAs="icon"`, `copy`, `resolved`), `entityText` (the same order as plain text), `shortId`, `bareId`.
- **App adapters**: `useEntityLabels` (bot / chat / group / Notify target → props; `text`, `botText`), and in the Projects package `ImChatLabel` / `useImChatText` (the package cannot import app code).
- **Projects host**: `imHost` keeps every bot's latest listing, rebuilds a chat lookup from them (`chatInfo`), refreshes on a 60 s stale tick and after an access-dialog change (a refresh requested while one is running runs again), and reports `deleted` per §2.3.
- Applied to the Bots table, Remote and Notify cards, the Access dialog (rename dialog names the chat), notify test picker / delete confirm / graph nodes, Projects tags, "From …" source lines, IM settings and the bind dialog; ~12 toasts name the entity. Binding a chat saves its alias as `chat_title` (the last-known name for when the chat is gone).


## 5. Decisions taken

| Item | Decision |
|---|---|
| Display order | `平台 · alias · id`; group marker merged into the platform: `平台群 · alias · id` (user) |
| Platform name = default alias; user alias never overwritten; empty name never wipes (user) | `alias \|\| name`, N1–N5 above; no provenance columns |
| Bots default to their platform account name, else empty (user, D4) | separate PR, §7 |
| Group marker is a localised pattern inside the platform segment (`{{platform}} group` / `{{platform}}群`); only the logo-only form draws a small `群` mark. No separate chip, no dedicated list-context flag (a list headed "Groups" keeps the marker) | user (merge) / decided |
| Platform as a logo under a bot header | decided |
| Feishu names without new scopes in v1 | separate PR, §7 |
| Deleted entities shown by id + saved alias + `(deleted)`, no snapshot column, no cascade | decided |
| Server-side `DisplayRef` / `*_ref` API fields | **not built**: once the platform name is stored beside the alias the name is already on `DirectChat` / `Group`, so the client resolves labels from lists it already loads |

## 6. Known limitations

- A binding stores no chat kind, so once its chat is gone the label cannot show the group chip.
- Weixin and WeCom do not provide sender names; those chats are shown by id.
- Persisted audit events are still log-only.
- A name typed for a group member (`AddGroupActor`) is stored as the actor's `display_name` and a later platform name for that actor replaces it, as before; actors carry no separate alias.
- An existing direct chat shows its id until its peer next sends a message (no backfill).

## 7. Split out of this change

Built, tested and kept on the branch `tp/audit-and-design-bot-im-alias-di-c6b317`, not part of this PR:

| Piece | State |
|---|---|
| Bot takes its platform account name; no `"<platform> Bot"` default; a name can be cleared | own PR |
| Feishu sender-name / group-title lookup (cache, scope back-off) | own PR |
| One log key set (`remote/logfields`) and a per-bot `[imbot <platform>/<id>]` log prefix | deferred |
| `/help`, `/status`, pairing replies name the bot and profile; CLI wording | deferred |
