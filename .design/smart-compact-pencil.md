# Smart Compact — Pencil Graph

Visual companion to `smart-compact-design.md`. Shows what `claude-code-compact`
(**通路 B**, the vmodel Claude Code actually hits) produces: a **flattened
narrative** with key tools preserved.

> **Mental model: information flattening.** `claude-code-compact` is a CC adapter.
> Its output is **not** a standard Anthropic message structure validated upstream —
> it is a flattened narrative CC reads as context. Standard-protocol rules
> (tool_use↔tool_result pairing, role alternation, tool_use_id) do not apply. For
> a tool, the only thing that matters is **positional correctness**: its call and
> result sit adjacent, in order, in the flattened flow. There is no 400 risk from
> shape — only information-loss / wrong-order risk.

---

## The example input

A Claude Code conversation. Round 1 has a **subagent (Task) call** and a file read;
round 2 is the `/compact` trigger (current round).

```
round 1 (historical):
  user      : "帮我用子 agent 调研一下 X，再读 a.py"
  assistant : [text "好"] [tool_use#1 Task(subagent,"调研X")] [tool_use#2 Read(a.py)]
  user      : [tool_result#1 "X 的调研结论是 …"] [tool_result#2 "<a.py 内容>"]
  assistant : [text "综合来看，结论如下 …"]

round 2 (current, triggers /compact):
  user      : "<command>compact</command>"
```

---

## Flattened output (one assistant text message)

`XMLCompactTransform` replaces **all** messages with one assistant text message.
`buildConversationXML` walks messages once, in order:

- **key tool** (`Task`, enabled in `keyToolsPreserve`) → inlined as
  `<tool name="Task">…</tool>` then `<tool_result>…</tool_result>`, adjacent,
  inside the assistant turn that issued the call.
- **non-key tool** (`Read`) → file path goes to a **per-turn** `<tool_calls>`.
- `tool_result` text is **never** folded into `<user>` — it appears only via its
  tool's inline/summary path, so call/result stay positionally correct.
- real user text stays in `<user>`.

```
[assistant]
 "<summary>
    <conversation>
      <user>帮我用子 agent 调研一下 X，再读 a.py</user>
      <assistant>
        好
        <tool name="Task">{"description":"调研X",...}</tool>     ← key tool: call
        <tool_result>X 的调研结论是 …</tool_result>             ← result, adjacent
        <tool_calls><file>a.py</file></tool_calls>               ← non-key: per-turn
      </assistant>
      <assistant>综合来看，结论如下 …</assistant>
    </conversation>
  </summary>"
```

Errored key-tool results carry an `[error]` prefix inside `<tool_result>`.

---

## Key tool whitelist (internal, not user-facing)

`internal/smart_compact/xml_builder.go` package-level `keyToolsPreserve` map.
The map **value is the switch**: `true` = inline-preserve, `false` = summarize
(non-key). Listing a candidate at `false` documents the decision and makes future
enablement a one-line flip.

```
  ENABLED (true) — load-bearing facts, not re-derivable:
    Task             subagent conclusions
    AskUserQuestion  user decisions/choices
    WebFetch         externally fetched info (costly/volatile)

  DISABLED CANDIDATES (false) — re-derivable or redundant:
    WebSearch / TodoWrite / TaskCreate / TaskUpdate / Bash / ExitPlanMode

  All other tools (Read/Glob/Grep/LS/Edit/Write/.../MCP): non-key by default.
```

---

## The three constraints (flattening model)

```
  1. FIDELITY       key tool's call request AND conclusion are both kept
  2. POSITION       a tool's call and its result are adjacent, call-first,
                    inside the assistant turn that issued the call
  3. ATTRIBUTION    non-key tool files are emitted per-turn (not global-once)

  NOT constraints (standard-structure only, N/A to a flattened narrative):
  × tool_use ↔ tool_result pairing by id
  × user / assistant strict alternation
  × tool_use_id stability
  × "broken pair → upstream 400"
```
