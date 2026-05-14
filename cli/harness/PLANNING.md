# Harness — Planning

Forward-looking work for the harness, focused on **Tier B (`replay`)**. See
[README.md](./README.md) for the current design.

Status legend: `TODO` not started · `WIP` in progress · `DONE` shipped ·
`BLOCKED` waiting on something.

---

## 1. Close the `replaySkip` defect list

`replaySkip` in `replay.go` is a list of **real gateway bugs** surfaced by
replay, not test artifacts. The goal is an empty skip list.

| entry                 | root cause                                                              | status |
|-----------------------|-------------------------------------------------------------------------|--------|
| `*/codex/tool_use`    | Responses-API source path's tool_call conversion is incomplete; mirrors Tier A's `skipSourceScenarios["openai_responses\|tool_use"]` | TODO   |

When the Responses→{Anthropic,Chat} tool_call conversion is completed, remove
the three `*/codex/tool_use` entries **and** the corresponding
`skipSourceScenarios` entry in `internal/protocol_validate/matrix.go` together —
they describe the same defect at two tiers.

**Done when:** `replay batch --upstream {virtual,vmodel,real}` is fully green
with an empty `replaySkip`.

---

## 2. Expand scenario coverage

Replay currently runs `text`, `tool_use`, `streaming_text`. Tier A's matrix
defines more scenarios that replay should also exercise through the real
gateway pipeline:

| scenario             | matrix `Scenario` ctor          | fixture work needed                  | status |
|----------------------|---------------------------------|--------------------------------------|--------|
| `tool_result`        | `ToolResultScenario()`          | multi-block fixture w/ `tool_result` | TODO   |
| `thinking`           | `ThinkingScenario()`            | fixture w/ thinking enabled          | TODO   |
| `multi_turn`         | `MultiTurnScenario()`           | fixture w/ assistant+user history    | TODO   |
| `streaming_tool_use` | `StreamingToolUseScenario()`    | streaming fixture, tool-call assert  | TODO   |
| `error`              | `ErrorScenario()`               | fixture that should 4xx; assert it   | TODO   |

Each new scenario needs:
1. an entry in `replayScenarios` (matrix scenario + `structural` assertions +
   `defaultVModel`),
2. a fixture per API style under `testdata/fixtures/<style>/<scenario>.json`,
3. an entry in `replayScenarioOrder`.

Reuse the matrix `Scenario.Assertions` for the `virtual` upstream so the same
content checks run at Tier A and Tier B.

---

## 3. Fixture capture mode

Fixtures under `testdata/fixtures/` are currently **hand-authored**. They should
be **captured from real agent CLI runs** so they stay faithful to what the CLIs
actually send (headers, system blocks, metadata, tool schemas drift over time).

Proposed: a `harness replay capture <agent> --scenario <name>` subcommand that
runs the Tier C agent path with request recording enabled, extracts the raw
gateway request body, and writes it to the right fixture path. This makes
fixture refresh a one-command operation when an agent CLI updates.

- `TODO` design the capture flow (reuse Tier C's in-process gateway + recorder).
- `TODO` decide whether captured fixtures are committed or regenerated in CI.

---

## 4. CI integration

- `TODO` run `replay batch --upstream virtual` and `--upstream vmodel` on every
  PR — both are hermetic (no network, no secrets) and fast (~1–2s total).
- `TODO` gate merges on a green replay run; surface the summary table in the
  PR check output.
- `--upstream real` stays **manual / nightly** — it needs `providers.yaml` with
  live credentials and is non-deterministic.

---

## 5. Broader upstream coverage

- `TODO` `--upstream vmodel` currently uses `echo-model` (shared) and
  `web-search-example` (tool). Add per-scenario vmodel IDs that exercise more of
  the vmodel registry (thinking models, multi-block responses).
- `TODO` `--upstream real`: allow running **all** runnable config entries, not
  just `firstRunnableEntry`, so replay can sweep a provider matrix the way
  `agent --config` does.

---

## Out of scope (tracked elsewhere)

- Tier D (`provider`) live provider-API conformance tests — placeholder in
  `provider.go`, not part of replay.
- vmodel usage/quota tracking — the `IsVirtual()` short-circuit intentionally
  skips outbound dispatch helpers; tracked in the vmodel roadmap.
