# Harness — Planning

Forward-looking work for the harness, focused on **Tier B (`replay`)**. See
[README.md](./README.md) for the current design.

Status legend: `TODO` not started · `WIP` in progress · `DONE` shipped ·
`BLOCKED` waiting on something.

---

## 1. Close the known-defect registry

`skipSourceScenarios` in `internal/protocoltest/matrix.go` is the **single**
registry of real gateway bugs; the matrix reads it directly and replay
derives its skips via `KnownDefectReason`. The goal is an empty registry.

| entry                                 | root cause                                              | status |
|---------------------------------------|----------------------------------------------------------|--------|
| `openai_responses\|tool_use` (+ streaming) | Not a gateway defect after all: the harness's OpenAI stream assembly fell back from the Responses assembler to the Chat assembler whenever there was no text content, which discarded the tool calls of a tool-call-only Responses stream (`assembleFromEvents`, testenv.go). | closed |

The registry is empty. Both matrix cells and every `codex/tool_use` replay run
are back in the cross-product.

**Done when:** `replay batch --upstream {virtual,vmodel,real}` is fully green
with an empty registry.

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
1. an entry in `replayScenarios` (matrix scenario ctor + `defaultVModel` —
   both assertion tiers already live on the Scenario itself:
   content `Assertions` for the virtual upstream, upstream-independent
   `Structural` for vmodel/real),
2. a fixture per API style under `testdata/fixtures/<style>/<scenario>.json`,
3. an entry in `replayScenarioOrder`.

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

Wired: `.github/workflows/harness-matrix.yml` runs every hermetic mode in
parallel legs — matrix (single / transitive / idempotent / flags /
content_shapes / cache_controls / cache_prefix / vendor), one matrix leg per
client driver (gosdk / python / node / aisdk), `replay batch` on the virtual
and vmodel upstreams, `lb --all`, `duo --skip-memory`, and `routing` — gated
by a single required `Harness result` status. `DONE`.

New matrix sections must add a leg here too — nothing enforces the mapping,
so it silently drifts (see cache_controls / cache_prefix / vendor, which
shipped without a CI leg for weeks).

Deliberate carve-outs:

- The duo **memory phase** stays out of shared-runner CI — noisy neighbors
  make retention-slope thresholds flaky. `TestDuoMemoryRegression` guards the
  slope in the Go suite (same `DuoDefaultMaxSlopeKB`).
- `--upstream real` stays **manual / nightly** — it needs `providers.yaml`
  with live credentials and is non-deterministic.

Open (policy, not wiring):

- `TODO` the workflow triggers on `ci/**` pushes, `v*` tags, and manual
  dispatch — not on PRs to the default branch. Decide whether the fast legs
  (matrix http + replay, ~seconds) should also gate PRs, leaving the
  toolchain-heavy client-driver legs on the current triggers.

---

## 5. Broader upstream coverage

- `TODO` `--upstream vmodel` currently uses `echo-model` (shared) and
  `web-search-example` (tool). Add per-scenario vmodel IDs that exercise more of
  the vmodel registry (thinking models, multi-block responses).
- `TODO` `--upstream real`: allow running **all** runnable config entries, not
  just `firstRunnableEntry`, so replay can sweep a provider matrix the way
  `agent --config` does.

---

## 6. Full single-process e2e run exhausts file descriptors

`go test -tags e2e ./internal/protocoltest/` (every e2e test in ONE process,
~1500 envs) fails on low-ulimit machines with `too many open files`; each
section run individually (the documented usage, and what CI runs) is green.
Reproduced identically on the pre-refactor baseline — pre-existing, surfaced
by running the whole suite at once.

Evidence from an fd probe (one TestEnv, `/proc/self/fd`): an env holds ~8
db fds; `Config.CloseStores()` (added, closes the store-manager and
provider-model gorm pools) returns no error but releases only the
provider-model connection — the store-manager pool's tingly.db connections
stay open after `sql.DB.Close`, i.e. they are held in-use somewhere in the
init path (Migrate / InsertDefaultRule / HydrateRules are the suspects), and
the guardrails `ProtectedCredentialStore` pool (guardrails.db, opened by
`server.NewServer`) has no close path at all.

- `TODO` audit the store-manager query paths for whatever keeps connections
  checked out (leaked Rows / Tx / prepared stmt), so `CloseStores` actually
  drains the pool.
- `TODO` give `ProtectedCredentialStore` a Close and call it on server
  teardown.
- **Done when:** the fd probe shows 0 remaining fds after `TestEnv.Close`,
  and the full single-process `-tags e2e` run passes at `ulimit -n 4096`.

---

## Out of scope (tracked elsewhere)

- Tier D (`provider`) live provider-API conformance tests — placeholder in
  `provider.go`, not part of replay.
- vmodel usage/quota tracking — the `IsVirtual()` short-circuit intentionally
  skips outbound dispatch helpers; tracked in the vmodel roadmap.
