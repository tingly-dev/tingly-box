---
date: 2026-11-08
---

# Remote: what is still its own, and what is left over

Question (YZ, 2026-11-08): Workspace (Projects + WorkDir) now does chat control over IM, so does Remote still have its own
functions, and has the old content been cleaned up? Findings from reading the tree at `refactor/remote-outbound`
(after the layering work in `.design/remote-layering.pencil.md`). **Not started; to be expanded later.**

## What Remote still owns

Chat control (sessions, approvals, execution, SmartGuide) is gone from Remote: a WorkDir chat is a Projects folder thread and
`remote/chat` only hands messages to it. What remains independent of Workspace:

1. **Access layer**: IM platform connections, pairing, access control, bot lifecycle.
2. **Claude Code hook push / ask**: a Claude Code running in the user's own terminal notifies IM and asks approvals through
   `scenario/claudecode`, the `notify` module, `POST /bots/:bot/notify|interact`, and `tingly-box bot notify`. Wired in
   `server_modules.go`, with a frontend Notify page. Whether anyone uses it was not checked.
3. **CLI** `tingly-box bot` (add, list, pair, config, notify).

## Left over

| Item | Evidence | Verdict |
|---|---|---|
| `afk/` module | no importer anywhere outside `afk/` (non-test or test, all modules); root `go.mod` has `replace` but no `require`; ~2.1k lines + tests; it was SmartGuide's runtime | orphan, safe to remove after one more check of scripts / CI |
| SmartGuide config residue | `_smart_guide` scenario type, `_internal_smart_guide_<bot>` rules; 11 hits in `internal/config/migration.go`, plus `typ`, `tbclient`, `remote/ask/types.go`, `imbot/core/bot.go` | kept on purpose so old configs still load; removing needs a config migration decision |
| Handoff guard | `remote/bot/prompt_reply.go` still skips `@cc @tb @mock /cc /tb /mock` when reading approval replies | commands no longer exist; dead |
| `stripAgentPrefix` | `remote/chat/project_route.go`: "@cc fix it" is still sent as a plain message | deliberate compatibility |
| DB columns | `remote_direct_chats` and `remote_groups` create statements still carry `bash_cwd`, `current_agent`, `verbose`, `project_path`; Go no longer reads or writes them (`project_path` is only carried through `access.DirectChat/Group`) | harmless leftovers; dropping needs a migration |
| `remote/chat/file_store.go` | downloads to `<dir>/.agent/…`, a naming from when files went into the user's project; now `<dir>` is a temp dir | rename |
| Docs | `docs/remote-coder-setup.md` (2026-02) describes "Remote Coder", `enable_remote_coder`, Telegram setup; `docs/images/create_gif.sh` | stale |
| Frontend | page dir `pages/remote-agent` / `RemoteAgentPage`; `/remote-coder` only redirects (`LegacyRemoteCoderRedirect`); `@tb/@cc` strings in `types/bot.ts` and `components/notify/BotNotifyGroup.tsx` | naming and redirect leftovers |

## Not verified

- Dead exported symbols inside `remote/` (no tool run that enumerates them).
- Whether the `enable_remote_coder` flag still exists anywhere.
- What the frontend pages actually show to users.

## Suggested order (safest first)

1. Remove `afk/` (module, `go.work`, `replace` lines, CI mentions).
2. Delete or rewrite the stale docs.
3. Remove the handoff guard; rename the `.agent` download dir.
4. Frontend naming and redirects.
5. DB columns and SmartGuide config residue last: both need a migration plan.
