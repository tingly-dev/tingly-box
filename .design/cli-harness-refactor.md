# cli/harness refactor — review and phased plan

Status: **proposal** (review of `main` at f5235a0f5, after #2618/#2621/#2624).
Nothing here is decided unless marked **Decided**. Phase 1 (this PR) is the
review, this plan, and a formatting-only commit; later phases are separate PRs
and wait for confirmation.

Related: [`vmodel-definitions.md`](./vmodel-definitions.md) (the file contract),
[`harness-matrix.md`](./harness-matrix.md), [`harness-agent-testing.md`](./harness-agent-testing.md),
[`harness-duo.md`](./harness-duo.md), `cli/harness/README.md`.

## 1. What the vmodel change actually changed

- #2618 / #2624: one `id` + required `kind` + exactly one behavior block
  (`mock | recording | sequence | decision | transform | builtin`), loaded only
  via `spec.Load` / `spec.Decode`, compiled by `spec.New` into a `spec.Runtime`
  (HTTP handler, `Scenario(id)`, `Check(id, result)`). Flat mocks, name-based
  fixtures, singular `tool_call` and the old mock/fixture loaders are gone.
  LB `faults`, captured requests and golden wire output deliberately **keep
  their own contracts** (`vmodel-definitions.md`, "Completed migration").
- #2621: builtin VModels get a fixed `/tingly/vmodel` address; `tb vmodel
  --config` is standalone and registers nothing in a gateway.

## 2. Verdict on "does the harness still use the old shape?"

**No old-shape parsing is left in `cli/harness`.** `grep` for `mock_responses`,
`legacy`, `name:`-fixtures, `tool_call` and flat-mock loaders finds nothing; the
only definition consumer is `matrix.go:175-186` and it already goes through
`spec.Load` + `spec.New` + `Runtime.Scenario`. `fixture_test.go` already uses
canonical `id`/`kind` bodies. Baseline on this branch: `go build`, `go vet` and
`go test ./cli/harness` clean; `harness replay batch --upstream vmodel` 9/9
pass (isolated HOME).

What is stale is **vocabulary and wiring around the new contract**, i.e. mixed
old/new naming that will confuse the next contributor:

| # | Mixed old/new item | Evidence |
|---|---|---|
| M1 | "fixture" still names the matrix definition input although the contract is "definition": field `Fixtures`, alias `--fixture`, errors `fixture %q requires…`, `no fixture test cases matched…`, `--config/--fixture requires --mode=single`, test names `TestMatrixFixtureValidation` / `TestUnifiedConfigAndFixtureAlias` | `matrix.go:26,169-211,259`, `fixture_test.go:11,40` |
| M2 | "fixture" has a **third** meaning in the same package: captured agent request bodies (`testdata/fixtures`, `replay.go:25-26,173-200,323`, `loadFixture`, `ReplayFixture`) | `replay.go` |
| M3 | `--config` means two different files: a VModel definition list in `matrix` (`matrix.go:26`) vs a providers/credentials YAML in `agent`/`replay` (`agent.go:26`, README:160,164,177) | flag collision; `vmodel-definitions.md:131-133` has to carve out the exception in prose |
| M4 | Upstream vocabulary: `virtual` (VirtualServer mock via `SetupAgent`/`SetupVirtualAgentScenario`) vs `vmodel` (seeded builtin provider via `SetupVModelAgent`) vs `--mock` (= `virtual`, but `harness-agent-testing.md:18,29` calls it "vmodel 虚拟上游") | `replay.go:71`, `agent.go:25`, `harness-agent-testing.md` |
| M5 | `replay --upstream vmodel` and `duo` pick vmodels by a Go-registered ID string (`replayScenarios[*].defaultVModel`, `replay.go:42-59`), not through the definition contract; harness cannot run `agent`/`replay` against a user definition file. The IDs currently exist in `spec.DefaultDefinitions()` (probed: `echo-model`, `web-search-example` → true) but nothing pins that | `replay.go:37,220-225` |
| M6 | Docs: `cli/harness/README.md` never mentions `matrix --config` (only `.design/vmodel-definitions.md:115,128` does); README:716 still describes scenarios as "mock responses + content Assertions" | README |

## 3. Other findings (not vmodel-related)

| # | Finding | Evidence | Risk to fix |
|---|---|---|---|
| F1 | Two files not gofmt-clean on `main` | `agent.go:24-36` (struct column alignment), `agent_assert.go:58` | none |
| F2 | `agent.go` 725 lines mixes: CLI + `Run` (107 lines), mock-mode runner, batch runner, timeout runner, three per-agent executors, result type, `parseAgentType` | `agent.go:24,111,219,357,431,477-700,714` | low (moves) |
| F3 | Three near-identical `executeXTest` (create env → `SetupAgent` → `executeXWithEnv`) differing only in agent type/provider name/model | `agent.go:477,551,626` (`SetupAgent` at 486,560,635) | low |
| F4 | Two result types and the same field-by-field copy done twice: `AgentTestResult` → `RealAgentTestResult` | `agent.go:431`, copy at `agent.go:264-270` and `agent_real.go:324-329`; types `agent_real.go:18` | medium (CSV/MD output must stay byte-identical) |
| F5 | "append row to summary, warn on error" repeated 4× | `agent.go:257,282,407`, `agent_real.go:240` | low |
| F6 | Global `agentRunTimeout` mutated in `AgentCmd.Run` and read by executors | `agent.go:316`, set in `Run` (`agent.go:137`) | medium (touches every executor signature) |
| F7 | `matrix.go` `Run` is ~140 lines; definition→scenario translation (`matrix.go:168-205`) is inline and only testable through `Run` | `matrix.go:158-262` | low |
| F8 | The same "load files → append → `spec.New`" loop exists in `matrix.go:175-186` and `internal/command/vmodel.go:68-84` | both | low, **cross-cutting** (adds `spec.LoadFiles`) |
| F9 | `lb.go` 655 lines: schema, fault decoding, runner, `checkExpect` (127 lines, 9 params), rendering all in one file; `--all` saves/restores `File`/`All` that are already `""`/`false` there | `lb.go:174-210,213,319,562,594` | low |
| F10 | `LbCmd --example` help hard-codes the example list that `lbExampleNames` already derives from `testdata/lb` | `lb.go:59` vs `lb.go:34` | low |
| F11 | `provider` command is a Phase 3 stub whose help cites `.sdlc/docs/cli-harness-spec-20260413.spec.md` (does not exist); tests assert it parses | `provider.go:7-24`, `main_test.go:44-45` | low but removes CLI surface → decision |
| F12 | Trivial leftovers: `fmt.Sprintf("%s", entry.Name)`, one-line wrapper `loadProvidersConfig`, `builtinRequestModel` alias, `DefaultSummaryFile` const only used by its own comment | `agent_real.go:286,106`, `agent.go:304`, `summary.go:14-17` | none |
| F13 | `runRealAgentTests` 138 lines (load → filter → validate → loop → tally); `runBatchAgentTests` 88 | `agent_real.go:121`, `agent.go:357` | low |
| F14 | `cli/harness/README.md` is 816 lines mixing quick start, per-command reference, internal file map (README:680-716 lists `internal/protocoltest` files) and a roadmap | README | none |

Cross-cutting, **not for this thread** (another thread owns `internal/`):
`internal/protocoltest/agent_env.go:342` `SetupVModelAgent` doc and the
`SetupAgent`/`SetupVirtualAgentScenario`/`SetupVModelAgent` naming (M4);
`internal/command/vmodel.go:68-84` (F8); `internal/protocoltest` still owning
`MaybeRunDuoServe`.

## 4. Phased plan (each phase = one small PR, low risk first)

Verification for every phase: out-of-tree `go.work` (replace to `libs/`), then
`go build`, `go vet`, `go test ./cli/harness`, plus
`harness replay batch --upstream=vmodel` and `harness matrix --mode=single`
under an isolated `HOME` for phases that touch run paths. Go moves use
`/home/x0079/tingly-go/tingly-go move` so references are rewritten, not
hand-edited.

| Phase | Content | Items | Risk |
|---|---|---|---|
| **P0** (this PR) | Review + this doc; `gofmt -w agent.go agent_assert.go` as its own commit (pure whitespace) | F1 | none |
| **P1** (stacked PR on P0) | Dead/trivial: drop `Sprintf("%s")`, inline `loadProvidersConfig`, drop `builtinRequestModel` alias or `DefaultSummaryFile`, derive `--example` help from `lbExampleNames`, simplify `lb --all` loop | F10, F12, part F9 | none–low |
| **P2** | Vocabulary + guard tests, no flag changes: rename `Fixtures`→`Definitions` and messages/tests "fixture"→"definition" (keep `--fixture` as alias); add a test that every `replayScenarios[*].defaultVModel` exists in `spec.DefaultDefinitions()`; README: add `matrix --config` section linking to `vmodel-definitions.md`; fix `harness-agent-testing.md:18,29` wording; one glossary in README for virtual / vmodel / mock / fixture | M1, M2, M4 (docs), M5 guard, M6 | low |
| **P3** | `matrix.go`: extract `scenariosFromDefinitions(paths, only)` into `matrix_definitions.go` with direct unit tests (same errors); optional `spec.LoadFiles` in a separate tiny PR coordinated with the `internal/` thread | F7, F8 | low |
| **P4** | `agent.go` split by `tingly-go move`: `agent_exec.go` (executors, `AgentTestResult`, timeout), `agent_mock.go` (`runVirtualAgentTest`), `agent_batch.go`; no behavior change. First add a golden test over `summaryWriter.Append` + `outputWriter.Write` so P5 can prove byte-identical output | F2, F14 (README split optional) | low |
| **P5** | De-dup: table-driven `executeXTest` (F3), one `applyAgentResult(r, inner)` (F4), one `appendSummary(writer, r)` (F5), split `runRealAgentTests` into select / run / tally (F13). Unify the two result types only if the golden test stays green | F3, F4, F5, F13 | medium (output contract) |
| **P6** | Replace global `agentRunTimeout` with a field on a small `agentRunner` passed to executors | F6 | medium |
| **P7** | `lb.go` split into `lb_schema.go` / `lb_expect.go` / `lb_render.go`; `checkExpect` takes one `lbOutcome` struct instead of 9 params | F9 | low |

## 5. Decisions needed (not started; each is user-visible)

1. **`--config` collision (M3).** Proposal: add `--providers` as the name for the
   credentials file in `agent`/`replay` and keep `--config` there as a
   deprecated alias; `matrix --config` stays the definition flag. Touches
   `tests/harness-baseline.sh:119`, README and the
   `harness-agent-testing.md` runbook (CI templates do not pass `--config`). Breaking only if the alias is later removed.
2. **`--fixture` alias.** Kept by #2618; propose keeping it through P2 and
   removing in a later release once nothing in CI/docs uses it (grep shows only
   `vmodel-definitions.md:128` and the test).
3. **`provider` stub (F11).** Delete the Phase 3 stub commands (and their two
   parse tests) or keep the placeholder and fix its dead spec link.
4. **Definition files for `agent --mock` / `replay --upstream vmodel` (M5).**
   Could accept `--config model.yaml` and serve it through a `spec.Runtime`
   instead of builtin IDs. This is a feature, needs `internal/protocoltest`
   changes (other thread's area); out of scope for the refactor unless asked.
5. **LB `faults` encoding (`lb.go:80-126`).** **Decided** by #2624 to keep its
   own contract (the shorthand `[500,500,200]` + `steps`/`on_exhaust`); not
   planned to be folded into `kind: sequence`.
