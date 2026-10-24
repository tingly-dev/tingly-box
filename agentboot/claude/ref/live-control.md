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
