# Claude Code live control — observed behaviour

Run against Claude Code CLI 2.1.302 (2026-10-18), `claude -p --input-format
stream-json --output-format stream-json --verbose --replay-user-messages
--permission-prompt-tool stdio`. Reproduce with `live-control-spike.py
{inject,control,adddir <withupdate|plain>}` (needs `claude` and credentials)
or `go test -tags e2e -run TestE2E_Live ./agentboot/`.

## Mid-turn message injection

`sleep 8` Bash call running; a second user message written to stdin 4s in:

- The CLI keeps reading stdin during the turn; nothing is rejected.
- The message is **not** consumed at write time. It is taken up right after
  the running tool call returns (the next model request), and only then is it
  echoed (`isReplay: true` with `--replay-user-messages`).
- The model sees it: the later tool call and the final answer follow the
  injected instruction. One `result` for the whole turn, not one per message.
- If the turn ends before another step (a reply with no tool call), the CLI
  runs the message as its own next turn: `result`, then a fresh `system/init`,
  the message echo, a second `result` (`live-control-spike.py notool`).
- AgentBoot does not pass `--replay-user-messages`, so no echo reaches the
  transcript; the host records the injected message itself.

## Control requests

`{"type":"control_request","request_id":ID,"request":{...}}`; the CLI answers
every one with `{"type":"control_response","response":{"subtype":"success"|"error","request_id":ID,...}}`.

| request | answer | effect |
| --- | --- | --- |
| `set_model` `model` (`"default"` resets) | success | next turn's `system/init` reports the new model |
| `set_permission_mode` `mode` | success + `{mode}`; also a `system/status` carrying `permissionMode` | immediate |
| `apply_flag_settings` `settings` | success | layers the keys over the process's start-up settings, effective from the next tool call (see "Settings" below) |
| `get_settings` | success + `{effective, sources, applied}` | the merged settings the process is running with |
| `set_max_thinking_tokens` | success | not used by AgentBoot |
| unknown subtype | `error`: `Unsupported control request subtype: X` | none |

## Adding a directory

No standalone request. `/add-dir` is interactive-only. A `can_use_tool`
reply may carry `updatedPermissions`:

```json
{"behavior":"allow","updatedInput":{...},
 "updatedPermissions":[{"type":"addDirectories","directories":["/x"],"destination":"session"}]}
```

Read of `/x/a.txt` (outside cwd) asks for permission. Allowed **with** the
update, a later Read of `/x/b.txt` in the next turn runs without asking.
Allowed **without** it, the later Read asks again. The request's own
`permission_suggestions` offers an `addRules` rule instead; either works.

## Settings (CLI 2.1.259, `live-settings-spike.py`)

The file given with `--settings` is read **once, when the process starts**.
Editing it later, in place or by rename, changes nothing in the running
process (checked for several seconds, idle, with no tool call in between).

`{"subtype":"apply_flag_settings","settings":{...}}` layers settings over what
the process started with, with no restart:

- It is a per-top-level-key replace of what an earlier request set: sending
  `{"permissions":{"allow":["A"]}}` and then `{"permissions":{"allow":["B"]}}`
  leaves only B in the layer; keys the request does not mention
  (`autoMode`, ...) are untouched; `{"permissions":{}}` clears that key's layer.
- The layer sits over the start-up settings, and arrays are unioned with the
  file's. So rules can be added live, but a rule
  that was in the file at start cannot be taken back before a restart.
  Start-up rules of kinds the request does not mention (a `permissions.ask`
  entry beside a pushed `permissions.allow`) stay.
- `get_settings` shows the result under `effective`.
- Enforcement follows: with the process in `default` mode and a scripted
  model calling Bash, a call with no rule raises `can_use_tool`; after an
  `apply_flag_settings` allow rule for that exact command the same call runs
  with no prompt, a different command still prompts, and clearing the layer
  makes it prompt again.
- An older CLI answers with `error: Unsupported control request subtype`.
