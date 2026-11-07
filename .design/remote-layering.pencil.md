---
date: 2026-11-08
---

# Remote layering: Remote as its own module (option B)

> Status: **decided direction, not started.** Decision (YZ, 2026-11-08): Remote is a library that connects IM and
> Agent, embedded in two ways, by Tingly Box and by Projects; it becomes its **own Go module**. Follows
> [remote-refactor.md](./remote-refactor.md) (R0–R5, done). Analysis and the options that were compared are in the
> conversation that produced this file; only the chosen one is kept here.

## Target

```
┌────────────── Tingly Box 宿主(根模块)───────────────────────┐
│ 接线 (server_modules.go)  HTTP API (module/imbot, notify)     │
│ CLI (internal/command/bot)  持久化实现 (internal/db/remote_*) │
│ remote ↔ projects 胶水 (bridge, adapter)                      │
└───┬───────────────────────────────┬──────────────────────────┘
    │ 使用                          │ 使用
┌───▼──── remote(独立模块)────────┐ │   ┌──── projects(独立模块)────┐
│ 接入层  bot  access  配对  鉴权  │ │   │ 实现 Remote 的 Agent 端口  │
│ 连接层  channel  interaction     │◀┼───│ (可独立带 IM)              │
│ Agent 端口  agent.Chat           │ │   │ 不 import 根模块           │
│   Post Command Answer Place      │ │   └────────────────────────────┘
│   ToFolder ToProject Folders ... │ │
│ 审批词表  ApprovalChoice/Words   │ │
│ 可选  scenario/claudecode        │ │
│ 不 import 根模块,不 import projects
└────┬───────────────┬─────────────┘
     ▼               ▼
   imbot         agentboot
```

## Dependency rules (what the module boundary enforces)

| From → to | Allowed |
|---|---|
| host (root) → remote, projects, imbot, agentboot | yes |
| projects → remote, agentboot | yes (new: it implements Remote's port and reads its vocabulary) |
| projects → root, imbot | no (`task projects:boundary`) |
| remote → imbot, agentboot | yes |
| remote → root (`internal/*`), projects | **no** (new `task remote:boundary`) |

## What changes

1. **Agent port belongs to Remote.** `projects.Chat`/`ChatRef`/`ChatPlace`/`ChatCommands` (R2/R4) move to a
   `remote/agent` package; Projects keeps the names as aliases and implements the interface on `*App`.
2. **Approval vocabulary belongs to Remote.** `ApprovalChoice`/`ApprovalWords` (R5) move out of Projects; Projects'
   `approvalReply` reads them from Remote.
3. **Host glue leaves the library.** `remote/adapter` (needs `internal/db`) and the db-backed test harness move to the
   host; `remote/bridge` (joins Remote chats to a `projects.App`) moves back to the host, thinner once Projects
   implements the port directly.
4. **Remote's own tests stop importing `internal/db`.** Four `remote/bot` tests and `chat/testharness.go` use the
   db stores today; they get an in-memory store inside Remote.

## Phases (one reviewable commit or small group each; behavior unchanged)

| # | Step | Result |
|---|---|---|
| L1 | Move `remote/adapter` into the host | no `internal/*` in non-test remote code except the test harness |
| L2 | In-memory chat/access store for Remote's tests; `remote/bot` tests and `testharness.go` use it | remote has no `internal/*` imports at all |
| L3 | Add `remote/agent` (port types + interface); `remote/chat` and `remote/ask` use it; `bridge` moves to the host and maps `projects.Chat` ↔ `agent.Chat` explicitly | remote no longer imports `projects` |
| L4 | Split `remote/` into its own module: `go.mod`, `go.work`, root `replace`/`require`, `task remote:boundary`, CI | the boundary is enforced by the build |
| L5 | Projects imports Remote: aliases for the port types, `*App` implements `agent.Chat`, vocabulary moves, the mapping in the bridge goes away | one definition per concept |
| L6 (later) | Unify the outbound vocabulary (`Channel*` vs `channel.Channel`); wire IM into standalone `tingly-project` | optional |

## Risks / watch

- L3 adds a temporary mapping layer (two near-identical type sets) that L5 removes; it exists so every commit builds
  and each step is reviewable.
- L4 touches build wiring (`go.work`, `replace`, CI, `Taskfile`); verify `go list -deps` of `remote` has no root module.
- `remote/scenario/claudecode`, `remote/ask`, `imchannel` import `agentboot` (already its own module): fine, no change.
- Any rename must not change user-visible text: chat commands, `/help`, replies stay as in remote-refactor.md.

## Open

- Does standalone `tingly-project` get IM in this effort (L6), or only become able to? (Default: only able to.)
- Who owns persistence for a standalone Projects with IM: its own stores implementing Remote's store interfaces.
