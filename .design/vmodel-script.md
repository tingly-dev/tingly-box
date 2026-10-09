# vmodel scripts — scripted multi-step interactions

## Problem

`vmodel` is a good deterministic stand-in for an LLM, but a *specific
interaction sequence* — "call `Read`, get the result, call `Edit`, hit a 529,
retry, answer" — had no quick way to be written down and exercised. The
existing pieces cover fragments: always-fail mocks (one shape), `sequence`
(a status program, text only), a single-tool mock. Standing up an ad-hoc
`httptest.Server` per scenario reintroduces what vmodel exists to remove.

## Decision: engine and schema in vmodel, authoring layers on top

| Concern | Where | Why |
| --- | --- | --- |
| Simple, repeatable interactions | **vmodel**, config-driven (YAML) | Wire-correct streaming, usage, mid-stream failure for all three protocols come for free, and the config style already exists (`lb` faults, harness scenarios). |
| Logic that depends on the request | Python `tingly.Server` (a provider) | Arbitrary code belongs in a real language, not a DSL. Unchanged. |
| Writing and launching scripts from Python | `tingly.vmodel` (PR2) | A thin authoring layer that compiles to the **same** YAML schema; it adds no semantics of its own. |

A script **is** a `vmodel.SequenceConfig`: the existing sequence engine
(`vmodel/sequence.go`, see [`vmodel-sequence.md`](./vmodel-sequence.md)),
generalised from "a status per request" to "a full outcome per request".
Per-request `Snapshot()`, the atomic cursor and `on_exhaust` are unchanged.

## Schema

```yaml
id: refactor-flow          # optional; defaults to the file name
on_exhaust: clamp          # loop (default) | clamp | fail
steps:
  - say: "Let me look at the file."
    tool: {name: Read, arguments: {file_path: /tmp/a.go}}
  - tool: {name: Edit, arguments: {file_path: /tmp/a.go, old_string: foo, new_string: bar}}
  - 529                                        # bare number == {status: 529}
  - say: "Done."
    usage: {input: 1200, output: 40, cache_read: 1000}
```

One step = one request's outcome:

| Field | Meaning |
| --- | --- |
| `status` | `200`/omitted → success; `400–599` → pre-content error envelope (type/message derived from the status). A bare number is shorthand for this. |
| `say` | Response text. Empty on a plain success falls back to `default_content`, then a module default. Empty on a `tool` step means *no* lead-in text. |
| `tool` | `{name, arguments, id?}` — one tool call. Without an `id`, each *served request* gets a unique `toolu_<script>_<n>` (so `repeat:`, loops and clamping never repeat one); an explicit `id` is used as written (1–64 of `A-Za-z0-9_-`, the shape real APIs accept; generated ids are sanitised and truncated to it). `arguments` omitted means `{}`. A tool step with no `say` has no text block — exactly what the script says. |
| `stop_reason` | Override, in a **protocol-neutral vocabulary** (Anthropic's words): `end_turn`, `tool_use`, `max_tokens`, `stop_sequence`. OpenAI renders them as `stop`, `tool_calls`, `length`, `stop`. Anything else is rejected. Defaults: `end_turn` (`tool_use` on a tool step). |
| `usage` | `{input, output, cache_read, cache_write, reasoning}` advertised on the stream of `/messages` and `/chat/completions`. The direct `/responses` endpoint estimates usage from the text and ignores `usage` / `stop_reason` (its own pre-existing rendering). |
| `midstream` | `{mode: close\|event\|eof, after_events: N}` — a success whose stream is cut. Not combinable with an error `status`. |
| `repeat` | Serve the step N consecutive times. |
| `error_message`, `error_type` | Override the error envelope of an error step. |

Top level: `id`, `name`, `description`, `delay`, `default_content`, `on_exhaust`, `steps`.

**Parsing is strict**, at every nesting level (`usage:`, `midstream:`, `tool:` included). Unknown fields (`sya:`, `args:` for `arguments:`, `input_tokens:`),
out-of-range statuses, error steps carrying a body, unknown modes, `stop_reason: tool_use` without a tool and
duplicate ids fail with a step-numbered message (`vmodel.ParseScript`). A typo
must never silently become a default step.

### What a step is *not* (yet)

No request matching (`when:`), no per-conversation cursors, no `think` blocks,
no multiple tool calls per step, no request capture. See *Phases*.

## Serving: the script directory

`<config-dir>/vmodels/*.yaml|yml` — each file becomes one model, registered
under its id in **both** protocol registries (so the same file answers
`/messages` and `/chat/completions`/`/responses`), and **each protocol runs its
own independent copy of the program** — its own cursor. This is deliberate: a
request on one wire (a probe, a second client, a stray `chat()` call) must never
consume a step of the other, or a test's outcome would depend on call order it
cannot see. Within one protocol the cursor is still shared by every client of
that model (per-conversation cursors are deferred); rewriting the file is the
reset.

**Which protocol to use.** The gateway converts between protocols, so scripts
need only one: the docs, `harness script` (default `claude`) and the Python
`Testbed` lead with the Anthropic side, and anything else rides on conversion.
The direct `/virtual/openai/*` endpoints stay, since they bypass conversion and
give an independent native-OpenAI reference when diagnosing a conversion bug.

There is no watcher and no API. `virtualserver.scriptStore.Refresh` runs at the
top of every vmodel entrypoint and compares a directory signature (names +
mtime + size) — one `ReadDir` and a few `stat`s when nothing changed. So:

- **write a file, call the model** — no restart, no registration step;
- an **edited** file is re-registered with a fresh cursor (the program restarts);
- a **deleted** file unregisters its model;
- a file that **fails to parse** (or loses an id race) serves nothing — what is
  served always matches what is on disk — and the reason is logged and appended
  to the `404 Model not found` message, so a broken script explains itself.
  Failed files are retried on every directory change (a freed id, a swap);
- readers that bypass the HTTP handlers (the management UI's model listing)
  call `Service.RefreshScripts()` first;
- a script **never shadows** a built-in model, and the first file (by name)
  owns a duplicated id.

The per-request cost is deliberate: a watcher would add a goroutine and a
platform dependency for a development convenience, and "visible on the very
next request" is the property that makes Python's `write → call` flow (PR2)
race-free. The directory defaults to the real config dir, so a script written
for a test also works against a developer's running tb.

## Phases

1. **PR1 — engine, YAML, directory serving** *(this change)*; then `harness script` (a follow-up PR) to check a script step by step through the gateway.
2. **PR2 — Python `tingly.vmodel`.** A chain builder that compiles to this
   schema plus a `Testbed` that launches a real tb with a throwaway config dir,
   drops scripts into `vmodels/`, and hands back ready base URLs — so no tb-side
   API is needed. An *attach* mode writes into an already-running tb's config
   dir.
3. **Deferred until there is a consumer:** `when:` request matching and
   per-conversation cursors (needed once several clients share one script),
   a management API / CLI / UI (remote or non-file registration), request
   capture for asserting on client behaviour, recording → script, a webhook
   step, `think` blocks and multiple tool calls per step.

## Files

- `vmodel/sequence.go` — `SequenceStep` (full outcome, `UnmarshalYAML`), `Sequence`, `ResolvedStep`.
- `vmodel/script.go` — `ParseScript`, `SequenceConfig.Validate`.
- `vmodel/types.go` — `ToolCallConfig.ID`, strict `UnmarshalYAML`.
- `vmodel/{anthropic,openai}/sequence_model.go` — `Snapshot` carries tool/usage/stop reason.
- `vmodel/virtualserver/scripts.go` — the directory store; `service.go` `SetScriptDir`; `handler.go` refresh hooks.
- `internal/server/server.go` — wires `<config-dir>/vmodels`.
