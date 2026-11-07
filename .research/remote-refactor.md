---
date: 2026-11-07
---

# Remote refactor: where Remote and Projects overlap today

Input to [`.design/remote-refactor.md`](../.design/remote-refactor.md). Facts come from reading the tree at
`refactor/1107-remote` (after #2461) and from `tingly-go` (binary only, used for dry-run moves). Items marked
**verify** were not checked in code.

## 1. What Remote is now

[`remote-into-projects.md`](../.design/remote-into-projects.md) finished the big move: chat execution (`@cc`/`@tb`,
`remote/session`, SmartGuide, executors) is deleted. Remote is the **access layer** (bots, pairing, ACL) plus
the **notification middle layer** (hook-driven Flow A). Everything else is delegated to Projects.

| Area | Path | Non-test LOC | Role |
|---|---|---|---|
| `remote/control/bot` | bot manager, consumers, chat store, pairing, authorization | 1705 | access + bot lifecycle |
| `remote/control/remoteagent` | inbound message handler, commands, `/cd` `/dir`, `ProjectRouter` | 2579 | chat entry, routes to Projects |
| `remote/control/ask` | IM prompter, tool handlers (AskUserQuestion etc.) | 680 | approvals/questions for hook path |
| `remote/control/feature` | action menu, Telegram dir browser | 455 | chat UI helpers |
| `remote/control/adapter` | host-to-remote persistence glue | 116 | glue |
| `remote/access` | capability vocabulary, evaluator | 340 | authorization |
| `remote/channel` (+`imchannel`, `autochannel`), `interaction`, `scenario` (+`claudecode`), `binding` | notification middle layer | ~1000 | Flow A |
| Host side | `internal/server/module/{imbot,notify,projects}`, `internal/db/remote_*` | imbot 3.0k, notify 1.0k, projects/chatbridge 409 | HTTP APIs, wiring, bridge |
| Total `remote/` | | 8.5k (imbot lib is 17.9k, agentboot 10.7k, projects module 15.6k) | |

`remote/` is imported only by `internal/server` (+ `module/{imbot,notify,projects}`), `internal/db`, and
`internal/command/bot.go`. `projects/` is its own Go module and never imports the root; the root imports it
through the `projects` facade. **`remote/` does not import `projects` at all today.**

## 2. How the two halves talk

```
IM platform ─ imbot ─ remoteagent.BotHandler ─ ProjectRouter (iface, defined in remote)
                                                    │
                                   server/module/projects.ChatBridge   (409 lines, the translator)
                                                    │
                                   projects.App.{PostFromChannel, ChannelCommand, AnswerFromChannel, ChannelToFolder…}
                                                    │
 reply / report / approval  ◄── ChannelSink / ChannelFileSender / ChannelPrompter (ifaces, defined in projects)
```

Two interface families describe the same conversation, one per direction, each owned by a different side, and a
third package (`ChatBridge`, living in a module named `projects` although half of it is IM) translates between them.

## 3. Overlaps found

| # | Overlap | Evidence |
|---|---|---|
| S1 | **Mirrored contracts.** `remoteagent.ProjectInbound/ProjectFile/ProjectChoice/ProjectRouter` mirror what `projects.App` already exposes (`PostFromChannel(platform, chatID, text, files)`, `RegisteredFolders`, `RegisteredProjects` → `ProjectChoice`, `ChannelToFolder`, …). Note `ProjectChoice` exists in both packages. | `remote/control/remoteagent/project_route.go:19-79`, `projects/projects.go:151-283` |
| S2 | **"Where does this chat work" lives in two stores.** `remote_chats.project_path` + `project_history` (JSON MRU) vs Projects `ChannelBinding.Folder/target_*`. Doc says `/dir` history "only affects ordering". Both written on every `/cd`. | `internal/db/remote_chat.go:17-25`; remote-into-projects.md §"第 3 期 a" |
| S3 | **Two command layers.** Remote registry (9 commands, drives the platform menu) forwards the session commands by *name* (`sessionCommands`) to `Service.ChannelCommand`; help text exists in `project_help.go` (remote) and in Projects `chat_commands.go` (267 lines). Adding a command touches both. | `remote/control/remoteagent/command.go:13-31`, `projects/internal/project/chat_commands.go` |
| S4 | **Two approval/question paths.** Flow A uses `ask.Prompter` + `imchannel` (+ `interaction.Interaction`, JSON-neutral). Projects uses `ChannelPrompt`/`ChannelPrompter` + text-reply parsing in `chat_answer.go` (162 lines). Both do "buttons or text fallback, restate after settle". **verify** whether y/n/always parsing and button building are duplicated or already shared. | `projects/internal/project/channels.go:23-60`, `remote/control/ask`, `remote/channel/imchannel` |
| S5 | **Layout debt.** `remote/control/*` nesting is from when "remote control" meant Claude Code execution; it now holds access, bot lifecycle and chat entry. `control/bot` mixes manager, consumers, chat store, pairing and privileges. Host side is split three ways (`imbot`, `notify`, `projects`) plus `db/remote_*`. The product calls it IM (sidebar), the CLI calls it `bot`, the code calls it `remote`. | file listing; remote-into-projects.md §第 4 期 |
| S6 | **Leftovers.** `internal/db/remote_import.go` (legacy `bot_chats.json` import), unused chat columns (`BashCwd`, `CurrentAgent`, `Verbose`), `_smart_guide` scenario type, `scenario/autochannel` (exists "to validate the abstraction"; headless only). Some are deliberately kept for old configs. | remote-into-projects.md §第 4 期 b; `remote/channel/autochannel` |
| S7 | **Same host pattern, reinvented.** Projects' `Host` + optional capability interfaces (`Launcher`, `ChannelSink`, …) is the same shape as Remote's `adapter` package + `Registry` of channels. Not wrong, but two vocabularies for "embedder supplies X". | `projects/projects.go:36-80`, `remote/control/adapter` |

## 4. What is already shared / needs no work

- Wire/translation layer is `imbot` (platform adapters, keyboards, `ParseSlashCommand`); both halves use it.
- Access control is cleanly one side: Projects never sees bots, pairing or capability (`bot-capability-access-control.md`).
- The decision "Remote = who may come in, Projects = what happens after" (remote-into-projects.md) holds in code; the seam is just thicker than it needs to be.

## 5. Tooling (`tingly-go`, binary only)

Commands: `move` (package/file/func/type/range, rewrites all references, `-n` dry-run, `--alias` leaves a shim),
`rename`, `prune` (references/usages, dead-code check, `--production` ignores test-only use), `weight`, `diff`, `stage`.

Dry-run proof that mechanical moves are cheap: `tingly-go move -n -v remote/control/ask remote/ask` reports
**88 references in 6 files** (module loads 293 packages). `ask` is imported only by `remote/control/bot` and
`remote/channel/imchannel`. The `prune` positional/usage form for dead-code checks still needs a first working
invocation (my `prune usage <pkg> --production -v` printed its help; **verify** the exact form before R0).
`move` works across `go.mod` boundaries only with `--ignore-go-mod`, relevant because `projects/` is a separate module.

## 6. Constraints that shape the design

- `projects/` must not import the root (`task projects:boundary`). The reverse is allowed.
- Standalone `tingly-project` has no IM: `ChannelSink` and friends stay *optional* capabilities.
- UX-first: chat commands, replies and `/help` text must not change visibly in a pure refactor.
- Old DB columns are kept, not dropped, unless the design says so (precedent: 第 4 期 b).
