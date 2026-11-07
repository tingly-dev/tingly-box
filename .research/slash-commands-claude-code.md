# Claude Code slash commands over stream-json: what the CLI really does

Observed on **Claude Code 2.1.259**, real binary, scripted local Anthropic mock
(`projects/showcase/e2e-demo/mock_anthropic.py`), isolated `HOME` /
`CLAUDE_CONFIG_DIR`. No key, no network. Reproduce:

```
python3 .research/slash-command-probe.py claude ctx cmds        # resident process, local commands
python3 .research/slash-command-probe.py claude ids etc skill   # session ids, "/etc/hosts", skills
RUN_MOCK=1 python3 .research/slash-command-probe.py claude mid  # "/compact" written during a tool call
python3 .research/slash-command-probe.py claude initkeys        # initialize / get_context_usage / list_models
python3 .research/slash-command-oneshot-probe.py                # claude -p "/compact" --resume ID
SUMMARY=1 ...                                                   # one-line digest per event
```

The mock model is not Claude, so token figures (`pre_tokens` / `post_tokens`)
and the summary text are meaningless here. Event shapes, ordering and which
commands reach the model are real. Re-check the numbers against a real model
before building UI on them.

## 1. A slash command in a user message is honoured (verified)

`{"type":"user","message":{"role":"user","content":"/compact"}}` on stdin of
`claude -p --input-format stream-json` runs the command. The same works as the
one-shot prompt (`claude -p "/compact" --resume ID`). There is no separate
"command" message type and the model is not called (mock log shows no request)
except for `/compact` itself (the summary call) and for skills.

Every local command answers with the same envelope:

| order | event | notes |
| --- | --- | --- |
| 1 | `system/init` | a **new init for every command**, like a normal turn |
| 2 | `assistant` with `message.model == "<synthetic>"`, zero usage | the command's text output. Absent for `/compact` and `/clear` |
| 3 | `result` `subtype:"success"`, `is_error:false`, `num_turns:0`, `result:<same text>` | `num_turns:0` is what marks "no model turn". Usage all zero |

## 2. Per command

| command | result in stream-json mode |
| --- | --- |
| `/compact [instructions]` | `system/status{status:"compacting"}` → `system/status{status:null, compact_result:"success"}` → `init` → `system/compact_boundary` → `user{isSynthetic:true}` (the summary prompt) → `user{isReplay:true, "<local-command-stdout>Compacted </local-command-stdout>"}` → `result{result:""}`. `compact_metadata`: `trigger:"manual"`, `pre_tokens`, `post_tokens`, `duration_ms`, `cumulative_dropped_tokens`, `preserved_segment{head,anchor,tail uuid}`, `preserved_messages{uuids,all_uuids}`. Session id unchanged. |
| `/compact` with too little history | `status{compact_result:"failed", compact_error:"Not enough messages to compact."}`; `assistant "Not enough messages to compact."`; `result.is_error` is **false**. Failure is visible only in `compact_result` / `compact_error`. |
| `/clear` (aliases `reset`, `new`) | `{"type":"conversation_reset","new_conversation_id":…}` → `init` with a **new `session_id`** → `result{result:""}`. The next turn runs under the new id. |
| `/cost` (aliases of `/usage`: `cost`, `stats`) | assistant text: "Total cost: $… / Usage by model: …". Priced at Anthropic list prices ("costs may be inaccurate due to usage of unknown models"). |
| `/context` | assistant markdown table: tokens by category, free space. |
| `/model` | no argument: "Current model: `x`. Usage: /model <name>. Available: sonnet, opus, …". |
| `/model sonnet` | "Set model to `sonnet[1m]` for this session only"; the next `init.model` is the new model. The CLI's idea of the model now differs from whatever the host believes. |
| `/effort`, `/config`, `/mcp`, `/rename x` | answered as text ("Usage: /effort <low\|…>", "Session renamed to: x", "No MCP servers are configured…"). They change CLI state. |
| `/agents` | "The /agents wizard has been removed." |
| `/status`, `/help` | "`/status` isn't available in this environment." |
| `/foo-nonexistent`, `/Compact` | "Unknown command: /foo-nonexistent" (case-sensitive; "Did you mean /compact?"). Synthetic assistant + `result` with `num_turns:0`; **the model is not called and the text is dropped**. |
| skill / custom command (`.claude/skills/hello/SKILL.md`) | `/hello world` is expanded and sent to the model (`num_turns:1`) **after `reload_skills`**; a skill added after process start is "Unknown command" until then. |
| `/doctor`, `/color` | listed in `init.terminal_slash_commands`: terminal-only. |

## 3. What counts as a command (verified)

- `"/etc/hosts is broken, why?"` and `"/ look"` are ordinary prompts (model called).
- `" /compact"` (leading space) is an ordinary prompt: the model was called.
  That is a cheap, verified way to send literal text that starts with a slash.
- `"//compact"`, `"hello /compact"` are ordinary prompts.
- `/Compact` is "Unknown command", not a prompt. So any `/word` the CLI does
  not know is swallowed, not answered: a message like `/usage-report is broken`
  never reaches the model.

## 4. Mid-turn (verified on 2.1.259, contradicts a report on 2.1.247)

`/compact` written to stdin while a Bash tool call runs: the turn finishes
normally (`result`), **then** the CLI runs `/compact` as its own turn
(`compacting` → `init` → `compact_boundary` → `result`). It is not read at the
next step like a normal injected message, and it is not treated as plain text.
GitHub issue anthropics/claude-code#76523 reports that a built-in sent mid-turn
on 2.1.247 is queued as plain text and never executed; not reproduced here
(UNVERIFIED which version changed).

## 5. Control requests (verified by `initialize`, `get_context_usage`, …)

The CLI's control request subtypes (string table of the binary + calls): `interrupt`,
`set_model`, `set_permission_mode`, `set_max_thinking_tokens`,
`apply_flag_settings`, `get_settings`, `stop_task`, `initialize`,
`get_context_usage`, `get_session_cost`, `get_usage`, `list_models`,
`reload_skills`, `reload_plugins`, `rename_session`, `set_cwd`, `rewind_files`,
`side_question`, `mcp_*`, `update_settings`, `generate_session_title`,
`cancel_async_message`, `end_session`, `get_plan`, … (called, answered
`success`: `initialize`, `get_context_usage`, `get_session_cost`, `get_usage`,
`list_models`, `reload_skills`; the rest only seen as strings).
**There is no `compact` or `clear` control request.** Compaction exists only as
the `/compact` command.

- `initialize` → `{commands:[{name, description, argumentHint, aliases?}], models, agents, output_style, account, current_permission_mode, session_state, …}`. 61 commands in the probe (built-ins plus skills). The same names are in `system/init.slash_commands`.
- `get_context_usage` → `{categories, totalTokens, maxTokens, rawMaxTokens, percentage, autoCompactThreshold, isAutoCompactEnabled, autocompactSource, messageBreakdown, apiUsage, slashCommands, skills, memoryFiles, …}`. `totalTokens` equals the last call's prompt size, so it is the number the status card needs, not an estimate.
- `get_session_cost` → `{text}` (same text as `/cost`), `get_usage` → structured session cost / model usage.
- `list_models` → `[{value, resolvedModel, displayName, supportsEffort, supportedEffortLevels, …}]`.

## 6. Auto-compaction (partly verified)

With `CLAUDE_CODE_AUTO_COMPACT_WINDOW=20000 CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=1`
the CLI starts compacting before a turn's API call: `status{compacting}` then
`status{null, compact_result:"failed", compact_error:"too_few_groups"}` (the mock
conversation was too short to succeed). Success path not exercised; the binary
contains `compact_metadata` with `trigger:"auto"`, so the success events should
match the manual ones (UNVERIFIED). Env/settings in the binary:
`DISABLE_AUTO_COMPACT`, `DISABLE_COMPACT`, `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`,
`CLAUDE_CODE_AUTO_COMPACT_WINDOW`, `CLAUDE_CODE_MAX_CONTEXT_TOKENS`.

## 7. Replays after a compaction (verified)

After `compact_boundary` the CLI re-emits the messages it **preserved**
(`compact_metadata.preserved_messages.all_uuids`) with the **same uuid** as the
first time. In `agentboot/engine/testdata/claude-2.1.259-slash-commands.jsonl`
`/compact` emits the earlier `/cost` report again (uuid `d328be96`). In a real
conversation the preserved tail can be ordinary assistant text, which would be
duplicated in a transcript that does not dedupe by uuid.

## 8. What today's engine does with this (verified, `slash_commands_test.go`)

`TestEngine_SlashCommandsToday` replays the fixture through the real
transport, resident process and converter:

- `/cost` → an assistant entry with the CLI's report (works, but list-price figures) plus an empty `usage` entry.
- `/compact` → **no entry says it compacted**; `status`, `compact_boundary` and the user messages are dropped (`accumulator.go:106`, `convert.go` has no case). The only text is the re-emitted `/cost` report, so the user sees an unrelated cost report. Context numbers are not refreshed.
- `/clear` → a `system` line with a new session id and nothing else; the engine keeps `--session-id`/`--resume` pinned to the old id.

## 9. Official docs (not fetched: WebFetch was blocked; from search summaries, UNVERIFIED wording)

- Agent SDK "slash commands" page: send `/compact`, `/clear` as the prompt string; `system/init.slash_commands` lists commands; built-ins vs bundled skills; `/clear` needs ≥ 2.1.117. Page now titled "Extend agents with skills".
- Headless (`-p`) docs: describes skills expansion; says terminal-UI-only built-ins are unavailable. An older wording ("only available in interactive mode") is on third-party sites and contradicts what was observed.
- `compact_boundary`: `compact_metadata.trigger`, `pre_tokens` (TS reference). `post_tokens`, `preserved_*` seen in the binary, not in the docs found.
- Hooks: `PreCompact` (matcher `manual|auto`, input `trigger`, `custom_instructions`), `PostCompact`, `SessionStart` source `compact`.
- TS SDK `Query`: `interrupt`, `setModel`, `setPermissionMode`, `supportedCommands`, `getContextUsage`, `reloadPlugins`, `rewindFiles`. **No `compact()` method**; the documented way is the prompt `/compact`.
