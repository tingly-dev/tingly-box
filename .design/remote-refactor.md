---
date: 2026-11-07
---

# Remote refactor

> Status: **R0–R4 done (one commit per step); R5 checked and deliberately not done (see below).** Proposal revised after review (R6 dropped; facade decision, R3 compat and R4 detail added).
> Evidence and overlap list: [`.research/remote-refactor.md`](../.research/remote-refactor.md) (S1–S7).
> Builds on [remote-into-projects.md](./remote-into-projects.md) (chat execution already moved) and
> [bot-arch.md](./bot-arch.md) / [bot-capability-access-control.md](./bot-capability-access-control.md).

## Goal

Finish what "Remote into Projects" started: Remote is **the access layer + notifications** and nothing else, and the
seam to Projects is **one contract with one owner**, instead of two mirrored interface families and a 409-line translator.
No user-visible change in any phase, except the small wording unification in R4.

## Decided (YZ, 2026-11-07)

- No rename of "remote" → "im" (R6 dropped). Package paths still change in R1 (drop the `control/` level only).
- `remote` may import the `projects` facade.
- Exactly one facade (SSOT) at the Remote↔Projects boundary — see "Facade" below for which style.
- R3 does compat, not a risky cutover; migration only if simple and robust.
- Projects is not a third party: Remote and Projects are one codebase and both sides may change together. Projects' chat
  layer (`project/chat_commands.go`, `Channel*`) has no standalone consumer (the standalone app has no IM), so it can be
  reshaped now without compatibility shims.
- Chat commands keep their current design and behavior (`/cd /dir /project /session /new /stop /clear /status /verbose /yolo`,
  the start-level vs "Any folder and files" tiers, the single `imbot.ParseSlashCommand`, all from #2428 and its follow-ups).
  This refactor moves where they are declared, not what they do.
- `bot_chats.json` legacy import is removed (R0).

## Principles

1. Remote decides *who may come in and through which bot*; Projects decides *what happens*. No new logic crosses that line.
2. One owner per concept: contract types, "where the chat works", command metadata, approval rendering.
3. `projects/` never imports the root; Remote may import the `projects` facade.
4. Every phase is behavior-preserving, mechanical where possible (`tingly-go move`), one PR each.

## Facade (SSOT): Projects style, with Remote's discipline

Remote and Projects are one codebase, so this is a choice of *where the single definition lives*, not of how to protect
ourselves from an outside party. Two styles exist today:

| | Projects style | Remote style |
|---|---|---|
| Shape | concrete `App` methods + `Host` and optional capability interfaces (`ChannelSink`, `ChannelPrompter`, `Launcher`…) type-asserted by the other side | `channel.Channel` + registry + JSON-neutral `interaction.*` types, host glue in `adapter` |
| Strength | plain typed Go; already how the boundary works; standalone binary just lacks the optional capability | transport-neutral (RPC-able), one renderer for hooks and anything else |
| Limit | types are Projects' own | `projects/` is a separate Go module that must not import the root (`task projects:boundary`), so it cannot use `interaction`/`channel` types unless they move into a shared module |
| Fit for inbound chat (post, command, answer, move) | natural | not designed for it (it models notify/ask, not commands or bindings) |

**Decision: Projects style is the SSOT.** It is the only style whose types Projects can compile against, and it
already owns the outbound half. Remote's `ProjectRouter`/`ProjectInbound`/`ProjectFile`/`ProjectChoice` are deleted;
no shared "neutral" module (it would only add a package to keep in sync). From Remote style we keep two rules and apply them to the Projects facade:

- **Data-only, string-keyed types.** No `imbot` types, no `context`-smuggled values: the origin of a message is an
  explicit `ChatRef`, not `ChannelBot(ctx)`.
- **Remote's `interaction.*` stays the renderer-side format** (Flow A). Where Projects' `ChannelPrompt` needs the same
  buttons/text-fallback, `bridge` converts it to `interaction.Interaction` once (R5); Projects never learns about it.

Proposed facade (in `projects/`), implemented by `*App`; inbound side:

```go
type ChatRef struct{ Platform, ChatID, BotUUID, SenderID, Profile string }   // origin of an inbound event

type Chat interface {
    Post(ctx, ref ChatRef, text string, files []ChannelFile) (handled bool, err error)
    Command(ctx, ref ChatRef, name, arg string) (handled bool, reply string, err error)
    Answer(ctx, ref ChatRef, threadID, requestID, value string) (reply string, err error)
    Place(ctx, ref ChatRef) (ChatPlace, error)             // folder|project, path/id/name, session tag; replaces Channel{Bound,Folder,Project}
    ToFolder(ctx, ref ChatRef, folder string, ifNew bool) error
    ToProject(ctx, ref ChatRef, projectID string) error
    Folders(ctx, ref ChatRef) ([]string, error)            // registered, ordered for this chat (R3)
    Projects(ctx) ([]ProjectChoice, error)
    Commands() []CommandSpec                               // R4: Projects' own command specs
}
```

Because the Projects-side chat layer has no standalone user, this is a straight rename-and-delete, not an additive layer: the
old `Channel*` entry points on `App` (`PostFromChannel`, `ChannelCommand`, `AnswerFromChannel`, `ChannelBound`,
`ChannelFolder`, `ChannelProject`, `ChannelToFolder`, `ChannelToProject`) and `ChannelBot(ctx)` are replaced by `Chat`,
and the `Chat*` command constants and the name `switch` in `chat_commands.go` are reshaped around R4's specs. Projects'
own tests (`channels_test.go`, `chat_commands_test.go`, `chat_project_answer_test.go`, the Tingly IM harness) move with them.

Outbound stays as is (`ChannelSink`, `ChannelFileSender`, `ChannelPrompter`), still optional. `chat.BotHandler`
holds a `projects.Chat`; the standalone harness passes a fake.

## Target shape

```
remote/
  access/            capability vocabulary                        (unchanged)
  bot/               bot manager, consumers, pairing, chat store  (was control/bot)
  chat/              inbound handler, Remote-owned commands       (was control/remoteagent)
    feature/         action menu, dir browser                     (was control/feature)
  ask/               IM prompter for the hook path                (was control/ask)
  adapter/           host persistence glue                        (was control/adapter)
  channel/ interaction/ scenario/ binding/                        (notification layer, unchanged)
  bridge/     ChatBridge (was internal/server/module/projects/chatbridge.go)
```

## Phases

### R0 — Hygiene
Run `tingly-go prune` (production mode) over `remote/` and `internal/db/remote_*`; delete what has no production
caller and is not a deliberate compatibility keep (S6). Decided: **delete the legacy `bot_chats.json` import** (`internal/db/remote_import.go`, its test, and the single call
`importLegacyRemoteJSON` in `store_manager.go:200`); anyone still holding that file has long since been through a release
that imported it. To decide, not auto-delete: unused `remote_chats` columns (code already ignores them), `autochannel`.

### R1 — Drop `control/`
Mechanical `tingly-go move`, one reviewed dry-run (`-n --json`) per move, leaf first: `control/ask → ask`,
`control/adapter → adapter`, `control/feature → chat/feature`, `control/bot → bot`, `control/remoteagent → chat`.
(`ask`: 88 references / 6 files.) Then fix package docs and `.design` references.

### R2 — One facade
Add `projects.Chat` and `ChatRef` as above. `chat.BotHandler` depends on it directly; delete the mirror types in
`project_route.go`. `ChatBridge` moves to `remote/bridge` and keeps only what the host alone knows: the
enabled flag, per-chat bot choice (`bots` map), blocked-target check, and the outbound sink over `channel.Registry`.
`internal/server/module/projects` is left with the Projects `Host` (models, launch env). Implement `Chat` on `*App` by
delegating to existing methods first (no behavior change), then retire the old `Channel*` entry points that `Chat` replaces.

**Blast radius (checked).** R2 stays at the facade layer (`projects/projects.go`) and does not touch the coordinator or thread engine:
- The `App.Channel*` / `PostFromChannel` / `AnswerFromChannel` / `RegisteredFolders` methods have exactly one caller, `ChatBridge`
  (`chatbridge.go`), plus `remote/chat` through an interface. Renaming them into `Chat` affects those two only.
- The `Service` methods behind them (`bind.go`, `relay.go`, `chat_commands.go`, `threads.go`) are also used by the coordinator, thread
  reports and the web binding module (`module/channel`). R2 does **not** change their signatures; `Chat` delegates to them.
- `ChannelBot(ctx)` stays as the internal plumbing in R2 (bind and relay read the bot from `ctx`); `ChatRef` is turned into that `ctx`
  at the facade. Making it explicit inside `Service` is a separate, optional step, not part of R2.
- No schema change in R2. Outbound (`ChannelSink`, relay, decision cards) is unchanged.
- Existing internal tests (`send`, `report_outlet`, `decision_im`, `chat_*`, `channels`) call `Service` directly and stay valid; only
  App-level tests and the Tingly IM harness move.

### R3 — "Where the chat works": compat, no cutover
Findings (this changes the earlier plan): the live path is already Projects-first. `GetProjectPath` reads Projects'
`CurrentFolder`; `/dir` already filters by Projects' registered folders. `remote_chats.project_path` is read only to
seed a chat's *first* folder (`adoptChat`), and `project_history` only to order `/dir`. A third copy,
`access.DirectChat/Group.ProjectPath`, is carried in the access tables with no reader I could find in
`internal/server/module` (**verify** before touching).

**As built (two commits, no migration, no schema change).** Deriving `/dir` order from Projects would scan
`thread_events` by an unindexed `source_chat_id` on every `/dir`, so I did not: the chat's recent-folder list stays in
`remote_chats.project_history` as a display-order preference of the access layer. What changed is who answers "where is
this chat now":
1. **Read from Projects.** The directory browser's start path and `GET /bots/:bot/chats` (`project_path`) ask Projects
   first (`Chat.Place`, via `bridge.Folder`) and fall back to the stored path only when Projects has none. Before, both
   showed whatever `/cd` last wrote, which goes stale when the chat is moved from the web.
2. **Stop writing `ProjectPath`.** `/cd` now calls `RememberFolder`, which only prepends to the MRU history.
   `ProjectPath` remains as the read-only seed for a chat with no place in Projects yet (`adoptChat`).
3. Columns stay; nothing is converted or dropped. Rollback = revert the commits.

Behavior: `/dir` order is unchanged. The one visible difference is the `project_path` in the chat listing now follows
web-side moves. The `access.DirectChat/Group.ProjectPath` copy is still unreferenced in the server modules and was left
alone.

### R4 — One command table
Details below. Short form: every command, Remote's and Projects', is one `CommandSpec`; the platform menu, `/help` and the
permission gate are all computed from the merged list, so the three hand-kept copies disappear.

### R5 — One approval/question renderer — checked, not done
Compared `projects/internal/project/chat_answer.go` with `remote/ask` + `remote/channel/imchannel` (+ `remote/bot/prompt_reply.go`).
The two do the same job for different callers, and they differ in what a reply *means*, so unifying them would change
what a user can type on one surface:

| | Flow A (hook-driven `ask` / `IMPrompter`) | Projects (`chat_answer.go`) |
|---|---|---|
| Approval words | `1 y yes` allow, `2 n no 0` deny, `3 a always` allow+remember | `y yes ok allow approve` + 是/好/允许/同意; `a always` + 总是; `n no deny reject` + 否/不/拒绝 |
| Other text | not an answer, passed on to other handlers | **deny**, and the words go to the thread as instructions |
| "Always" scope | per tool, global whitelist | this thread only |
| Question answers | positional tokens (`1 2 1`) | one line per question, `1,3` for multi-select, free text allowed |
| Prompt text | Markdown, "Option N", button hint | plain text, numbered, reply hint |
| State | blocking `Prompt` with pending map and timeout | thread request id; the thread already waits |

Only the vocabulary overlaps (`y/n/always`), and even there the sets differ. Merging means picking one set and one fall-through
rule, which silently changes either hook prompts or the Projects behavior decided on 2026-10-21
(`remote-into-projects.md`, "聊天式回答"). That is a product decision, not a refactor, so R5 stops here: no code change.
If you want it later, the smallest honest step is a shared `approvalWords` table both parsers read, with the Projects
fall-through rule kept as an explicit per-caller option.

## R4 in detail

### Today: each session command lives in three places

For `/session /new /stop /clear /status /verbose /yolo`:

| Place | File | What it holds |
|---|---|---|
| Remote registry | `remote/control/remoteagent/command.go` `sessionCommands` | name, menu description, priority (80…74) |
| Remote `/help` | `project_help.go` `helpText` | the usage line, e.g. `/verbose on|off: also see each step, not only the reply` |
| Projects dispatch | `projects/internal/project/chat_commands.go` | consts `ChatNew…`, a name allow-list `switch` at the top of `ChannelCommand`, then the behavior |

The menu description and the `/help` line are **already worded differently** (`Also see each step a session takes (on|off)`
vs `/verbose on|off: also see each step, not only the reply`), and adding a command means editing all three plus the
`RunSessionCommand` plumbing (`BotHandlerAdapter` → `ChatBridge.RouteCommand` → `App.ChannelCommand`).

Remote-owned commands (`/cd /dir /project /help /pair /join`) stay in Remote: they need imbot UI (keyboards, dir browser,
Telegram resolve) and access checks (`HasPrivilegedAccess`), which Projects must not know.

### Proposal: one table, one string per command

Scope reminder: command behavior, names, privilege tiers and parsing stay exactly as they are after #2428; only the
declaration moves.

A single descriptor type, defined in the `projects` facade (the one place both sides can import); both owners fill it, one list is merged at startup and everything else is derived.

```go
type CommandSpec struct {
    Name   string // "verbose"
    Usage  string // "/verbose on|off"
    Help   string // "Also see each step, not only the reply"  -- the ONE description
    Order  int    // position in menu and /help
    Needs  Need   // None | Privileged (free path / project list) -- gates /help and execution
    Scope  Scope  // Direct | DirectAndGroup
    Hidden bool   // /pair, /join: runnable, not listed
}
```

- **Projects** declares its seven (`/session /new /stop /clear /status /verbose /yolo`) as `[]CommandSpec` next to their
  handlers: `Commands() []CommandSpec`, each entry carrying its handler, so a spec without a handler cannot exist.
- **Remote** declares `/cd /dir /project /help /pair /join` the same way, with `Needs: Privileged` where the access
  policy applies. Their handlers stay in Remote (imbot UI, dir browser, Telegram resolve, `HasPrivilegedAccess`), but
  the *metadata* is the same type.
- At startup Remote merges the two lists (name collision = startup error) and derives from it: the `imbot.CommandRegistry`
  (so the platform `/` menu), `/help` (`Usage + " - " + Help`, filtered by the chat's `Needs`/`Scope`), and the
  gate that decides whether a chat may run a command. `sessionCommands`, `newSessionCommand`, `project_help.go` and the
  hard-coded `/help` branches (privileged / direct / group) are deleted; the allow-list `switch` at the top of
  `ChannelCommand` becomes a lookup in the same list.
- `@N <message>` is not a command; its `/help` line is a fixed footer in Remote.
- The `/help` intro ("Send a message to talk to Claude Code.") and the "Your ID / Chat ID" footer stay Remote's.

Net effect: adding or changing a command is one edit (its spec), and the menu, `/help` and permission gate cannot
disagree, because they are computed from the same value.

**Wording consequence (accepted, SSOT over byte-identity).** Today the menu description and the `/help` line differ per
command. With one `Help` string they become one. I would take the `/help` wording (it is the longer, clearer one), cut
to one sentence, and capitalise it consistently. This is a small visible change in two surfaces (the platform `/` menu and
`/help`); the new strings are reviewed in the PR as a diff table, and the golden files are updated deliberately, not
accidentally.

### Verification (golden captured before the change)

1. `/help` output for: privileged direct, non-privileged direct, group (privileged and not) -- recorded from today's code,
   then diffed against the new output; the only allowed differences are the reviewed wording changes above.
2. The registered command list (name, description, order, hidden, platforms) -- what feeds Telegram's menu.
3. Table tests: no duplicate names across the merged lists; every spec has a handler; menu and `/help` list the same
   commands for a chat with the same permissions (this is the SSOT property, tested directly).
4. `tingly_slash_commands_test.go` and `command_decorated_test.go` keep passing.

### Open sub-question

`/new` and `/clear` are folder-only today (on a project chat they reply "works in a chat bound to a folder"). With specs in
place a `Where` field could hide them from `/help` on a project chat. That is a visible improvement, not part of this refactor.

## Order and dependencies

R0 → R1 first (independent). R2 needs R1. R3 and R4 each need R2 and are independent of each other. R5 needs R2 and its verification.

## UX check (ux-principles.md)

- No new mode, toggle or name; chat commands and replies stay identical; R4 unifies menu and `/help` wording (reviewed diff, goldens updated on purpose).
- "Scope side effects to the current surface": R3 never changes other chats' targets.
- "Done ≠ locked": R3 keeps old columns and a revert path.
- Only code/package names change (R1); no user-visible renaming.

## Open questions

1. Facade: OK with the Projects-style `projects.Chat` above, replacing the old `Channel*` entry points outright (no shims)?
2. R3: OK that `/dir` ordering may shift once for chats with legacy history (then stable), in exchange for no migration job?
3. R0: `remote_import.go` goes. Is `autochannel` (headless policy channel) wired anywhere you care about, or can it go too?
4. R4: OK with one `Help` string per command (small wording change in the platform menu and `/help`, shown as a diff table in the PR)?

## Not in scope

`imbot` platform adapters, the Projects data model, access-control semantics, the CLI `bot` command, the "remote" name,
and any change to what a chat user sees.
