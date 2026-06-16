# Tingly-Box Protocol Validation Harness

A standalone CLI (`harness`) that validates the tingly-box gateway end-to-end:
protocol transforms, routing rules, scenario dispatch, and real agent-CLI
compatibility — without needing a deployed server.

> Forward-looking work — new scenarios, fixture capture, CI integration,
> closing the defect skip list — is tracked in [PLANNING.md](./PLANNING.md).

```
                          ┌─────────────────────────┐
                          │   harness  (this CLI)   │
                          └────────────┬────────────┘
                                       │
        ┌──────────────────────────────┼──────────────────────────────┐
        │                              │                              │
   ┌────┴─────┐                  ┌─────┴─────┐                  ┌──────┴─────┐
   │  matrix  │                  │   replay  │                  │   agent    │
   │  Tier A  │                  │  Tier B   │                  │   Tier C   │
   └────┬─────┘                  └─────┬─────┘                  └──────┬─────┘
        │                              │                               │
  protocol x-form              fixture-driven                   real agent CLI
  cross-product                gateway replay                   subprocess run
  (no gateway HTTP)            (in-proc gateway,                (in-proc gateway,
                                no CLI spawn)                    real CLI spawn)
```

The three tiers form a **cost / fidelity ladder** — cheap and exhaustive at the
bottom, slow and realistic at the top:

```
   fidelity
      ▲
      │   Tier C  agent    real CLI, real I/O      slow, flaky-prone, few runs
      │   ─────────────────────────────────────
      │   Tier B  replay   real gateway pipeline   fast, hermetic, broad
      │   ─────────────────────────────────────
      │   Tier A  matrix   pure transform funcs    instant, exhaustive
      └──────────────────────────────────────────▶ coverage
```

---

## Quick start

```bash
# Build
go build -o harness ./cli/harness

# Tier A — exhaustive protocol-transform matrix
./harness matrix
./harness matrix --scenario text --source anthropic_v1 --target openai_chat

# Tier A through real client stacks (--client; see .design/harness-matrix.md
# "Client drivers"): official Go SDKs in-process, or real Python/Node SDKs
# via subprocess drivers under tests/clients/.
./harness matrix --mode=single --client=gosdk
./harness matrix --mode=single --client=python   # pip install -r tests/clients/python/requirements.txt
./harness matrix --mode=single --client=node     # npm install --prefix tests/clients/node
./harness matrix --mode=single --client=aisdk    # npm install --prefix tests/clients/aisdk (AI SDK by Vercel)

# Tier B — replay captured fixtures through the gateway
./harness replay batch --upstream virtual     # deterministic mock upstream
./harness replay batch --upstream vmodel      # in-process vmodel dispatch
./harness replay claude --upstream real --config providers.yaml

# Tier C — run a real agent CLI through the gateway
./harness agent claude   --mock
./harness agent batch    --config providers.yaml

# Generate a providers config template for Tier B/C real mode
./harness init-config --output providers.yaml
```

---

## Tier A — `matrix`

Validates **protocol transformation** as a pure cross-product. No HTTP, no
gateway server — it drives the transform functions directly and asserts on the
converted payloads.

```
   sources          targets             scenarios            streaming
   ───────          ───────             ─────────            ─────────
   anthropic_v1  ┐   anthropic_v1  ┐     text         ┐       false ┐
   anthropic_beta│   anthropic_beta│     tool_use      │      true  ┘
   openai_chat   ├─x─ openai_chat  ├──x──tool_result   ├──x──
   openai_resp.  ┘   openai_resp.  │     thinking      │
                     google        ┘     multi_turn    │
                                         streaming_*   ┘

            every cell = one TestResult (pass / fail / skip)
```

- Defined by `protocoltest.DefaultMatrix()`; filter with
  `--scenario`, `--source`, `--target`, `--streaming`, `--non-streaming`.
- Known-broken cells are centralized in
  `protocoltest.skipSourceScenarios` (e.g. `openai_responses|tool_use`).
- `--json` for CI; `-v` / `-vv` to raise log verbosity; `--record-dir` to dump
  request/response pairs; `--batch N` for stability runs.

**Use it for:** catching transform regressions exhaustively and instantly.

---

## Tier B — `replay`

Replays a **captured agent request body** (a "fixture") through a real,
in-process gateway. Hermetic and fast: it boots the gateway router in-process,
wires the agent's built-in rule to a chosen upstream, POSTs the fixture, and
runs assertions on the round-trip result. **No agent CLI is spawned.**

```
  testdata/fixtures/<style>/<scenario>.json
            │
            │  rewrite "model" → built-in rule's RequestModel
            ▼
  ┌───────────────────────────────────────────────────────────┐
  │  in-process gateway  (server.NewServer + httptest.Server)  │
  │                                                            │
  │   POST /tingly/<agent>/v1/{messages,responses}             │
  │        │                                                   │
  │        ▼                                                   │
  │   built-in rule  (builtin:claude_code:cc / codex / opencode)        │
  │        │  rule.Services[0] → provider                      │
  │        ▼                                                   │
  │   ┌─────────────┬──────────────┬─────────────────┐         │
  │   │  virtual    │   vmodel     │     real        │         │
  │   │  upstream   │   upstream   │     upstream    │         │
  │   └─────┬───────┴──────┬───────┴────────┬────────┘         │
  └─────────┼──────────────┼────────────────┼──────────────────┘
            │              │                │
   VirtualServer    in-process vmodel   live provider
   mock responses   registry dispatch   (from --config)
   (deterministic)  (IsVirtual short-   (real network I/O)
            │         circuit)               │
            ▼              ▼                  ▼
     content-level    structural         structural
     assertions       assertions         assertions
   (scenario.Assertions)  (sc.structural)   (sc.structural)
```

### Fixtures

`testdata/fixtures/<style>/<scenario>.json` — the on-the-wire request body an
agent CLI sends to the gateway, embedded via `//go:embed`.

| style              | agents            | gateway endpoint                       |
|--------------------|-------------------|----------------------------------------|
| `anthropic`        | claude, opencode  | `/tingly/<agent>/v1/messages`          |
| `openai_responses` | codex             | `/tingly/codex/v1/responses`           |

Scenarios: `text`, `tool_use`, `streaming_text` (see `replayScenarios`).

The fixture's `model` field is rewritten to the built-in rule's `RequestModel`
at replay time, so fixtures stay **upstream-agnostic** — the rule, not the
fixture, decides which provider+model the request resolves to.

### Upstreams

| `--upstream` | wiring                                          | assertions checked        |
|--------------|-------------------------------------------------|---------------------------|
| `virtual`    | built-in rule → in-process `VirtualServer` mock | `scenario.Assertions` (content-level — response is deterministic) |
| `vmodel`     | built-in rule → seeded builtin vmodel provider, dispatched in-process via `provider.IsVirtual()` short-circuit | `sc.structural` (upstream-independent) |
| `real`       | built-in rule → live provider from `--config`   | `sc.structural` (upstream-independent) |

Only the `virtual` upstream controls the response byte-for-byte, so only it can
run the scenario's **content** assertions. `vmodel` and `real` responses aren't
test-controlled, so they get **structural** checks (HTTP 200, non-empty content,
tool-call count, stream-event count).

### Skip list

`replaySkip` maps `"<upstream>/<agent>/<scenario>"` → reason. Each entry is a
**real defect** surfaced by replay, not a test artifact. Remove an entry once
the underlying bug is fixed. Currently:

- `*/codex/tool_use` — Responses-API source path's tool_call conversion is
  incomplete (mirrors Tier A's `skipSourceScenarios`).

Closing this list out — plus planned scenario expansion and fixture capture —
is tracked in [PLANNING.md](./PLANNING.md).

**Use it for:** exercising the real gateway pipeline (rules, dispatch, vmodel
short-circuit) across every agent × scenario × upstream — fast and hermetic.

---

## Tier C — `agent`

Spawns a **real agent CLI** (`claude`, `codex`, `opencode`) as a subprocess and
points it at an in-process gateway. This is the only tier that exercises the
agent's own request construction, auth headers, and output parsing.

```
   ┌──────────────┐   spawn    ┌────────────────────────────┐
   │   harness    ├───────────▶│  agent CLI  (claude/codex/  │
   │   agent cmd  │            │             opencode)      │
   └──────┬───────┘            └─────────────┬──────────────┘
          │ writes env/config                │ HTTP
          │ (internalagent.BuildClaudeCodeEnv,│
          │  BuildOpenCodeConfig, …)          ▼
          │                    ┌────────────────────────────┐
          └───────────────────▶│   in-process gateway       │
                               │                            │
                               │   --mock     → virtual      │
                               │   --config   → real provider│
                               └────────────────────────────┘
```

Two mutually-exclusive modes:

- `--mock` — virtual-model mode: gateway wired to an in-process mock upstream.
  Exercises protocol translation + rule matching with zero external calls.
- `--config <file>` — real-provider mode: for every provider×model in the YAML,
  spins an isolated gateway, binds the built-in rule to that provider, and runs
  the agent CLI against it.

Agent argument also accepts `batch` — runs every supported agent in sequence,
continues past failures, exits non-zero if any failed.

### Persistence & resume

- Per-row results append to `harness-summary.csv` (flushed immediately, so
  partial progress survives Ctrl-C / crashes).
- Full prompt + output go to markdown files under `harness-output/`.
- `--resume ""` skips every `(agent, entry)` already recorded in the summary.
- `--timeout` caps each agent invocation (default `2m`; `0` disables). On
  timeout the child is killed and the row is recorded as `TIMEOUT`.

### Agent config

Per-agent env/config is **not** hand-built here — it's delegated to the
canonical `internal/agent` package (`BuildClaudeCodeEnv`, `BuildOpenCodeConfig`,
…) so the harness and production share one source of truth.

**Use it for:** final compatibility validation against real CLIs and real
providers.

---

## Tier LB — `lb`

Simulates **load-balancing dynamics** — tier selection, mid-request failover,
the circuit breaker (trip + *timed* recovery), and session-affinity stickiness /
re-pinning — over a **request sequence**, against programmable fake upstreams.
The other tiers validate protocol/transform fidelity; this one validates *routing
behavior over time*. It runs the real path (`routing.ServiceSelector.Select` →
`dispatchWithPriorityFailover`) with a deterministic breaker clock, so recovery
is exercised without sleeping.

```bash
./harness lb --example cascade        # built-in: cascade | flat | grid | single | regression
./harness lb --file scenario.yaml     # your own shape
./harness lb --example grid --json    # machine-readable trace
```

It shares one simulation engine (`internal/server.LBSimulator`) with the Go
scenario tests (`internal/server/lb_scenario_test.go`), so the CLI and CI assert
the same behavior.

### Output

```
LB scenario "cascade"  tactic=tier  affinity=1800s
services: t0/gpt-4 (T0)  t1/gpt-4 (T1)

#    session    attempts                status  pin
1    s1         t0/gpt-4 → t1/gpt-4     200     t0/gpt-4
...
4    s1         t1/gpt-4                200     t1/gpt-4     # breaker open → pin moved to t1
     -- advance 31s --
5    s1         t0/gpt-4                200     t0/gpt-4     # recovered → snapped back to t0

final breakers: t0/gpt-4=closed  t1/gpt-4=closed
```

The `attempts` column shows the per-request failover hops; `pin` shows the
affinity pin after each request. `--json` emits the same data structurally.

### Scenario file

A small YAML describes the rule *shape*, per-service fault scripts, and a program
of requests / clock-advances (see `testdata/lb/cascade.yaml`):

```yaml
rule_uuid: cascade
tactic: tier                 # tier | random
affinity_secs: 1800          # 0 = off
services:
  - { provider: t0, model: gpt-4, tier: 0 }
  - { provider: t1, model: gpt-4, tier: 1 }
faults:                      # serviceID -> per-call status sequence (last entry repeats)
  t0/gpt-4: [500, 500, 500, 200]
  t1/gpt-4: [200]
seed_pin: { session: s1, provider: t2, model: gpt-4 }   # optional: pre-lock a stale pin
program:
  - { request: s1 }          # request on session s1 (omit/"" = no affinity)
  - { advance: 31s }         # move the breaker clock forward
```

The shapes map to the **"Rule config shapes" taxonomy** in
`.design/priority-routing.md` (Single / Flat / Cascade / Grid). The
**G1 horizontal-tactic breaker-blind gap** documented there is *not* yet modeled
here (random/token tactics ignore the breaker at selection).

**Use it for:** reproducing a customer's rule shape + outage pattern and watching
exactly how routing, failover, the breaker, and affinity behave over a sequence.

---

## Agent reference

| agent      | API style          | gateway endpoint                  | built-in rule UUID  | RequestModel       |
|------------|--------------------|-----------------------------------|---------------------|--------------------|
| `claude`   | `anthropic`        | `/tingly/claude_code/v1/messages` | `builtin:claude_code:cc`       | `tingly/cc`        |
| `codex`    | `openai` (Responses)| `/tingly/codex/v1/responses`      | `built-in-codex`    | `tingly-codex`     |
| `opencode` | `anthropic`        | `/tingly/opencode/v1/messages`    | `built-in-opencode` | `tingly-opencode`  |

---

## Code map

```
cli/harness/
  main.go            Kong CLI root: version / matrix / agent / replay / lb /
                     provider / init-config
  matrix.go          Tier A command — wraps protocoltest.Matrix
  replay.go          Tier B command — fixture replay, upstreams, skip list
  agent.go           Tier C command — agent CLI subprocess driver (+ env wiring)
  agent_real.go      Tier C real-provider mode: config iteration, per-entry runs
  lb.go              Tier LB command — load-balancing scenario simulator
                     (engine: internal/server.LBSimulator; shapes per
                     .design/priority-routing.md)
  testdata/lb/       sample LB scenario YAML files
  config.go          init-config: generates providers.yaml from provider templates
  provider.go        Tier D placeholder (live provider API tests — not impl.)
  summary.go         CSV summary persistence / resume bookkeeping
  output.go          full prompt+output markdown file writer
  output_writer.go   table / JSON result rendering
  testdata/fixtures/ embedded replay fixtures, grouped by API style
    anthropic/        text.json, tool_use.json, streaming_text.json
    openai_responses/ text.json, tool_use.json, streaming_text.json

internal/protocoltest/   shared engine used by all tiers
  matrix.go          Tier A cross-product engine + skipSourceScenarios
  scenarios.go       Scenario definitions + content-level Assertions (reused by
                     Tier B's virtual upstream)
  agent_env.go       AgentTestEnv: in-process gateway + SetupAgent / SetupVModelAgent
  replay.go          SetupVirtualAgentScenario, ReplayFixture, repointBuiltinRule
  assertions.go      reusable Assertion constructors
  real_model.go      providers.yaml parsing for --upstream=real / --config
```

---

## How the tiers share code

The three tiers are thin CLI shells over one engine (`internal/protocoltest`):

```
                    ┌───────────────────────────────┐
                    │   internal/protocoltest  │
                    │                               │
                    │   Scenario + Assertions       │◀── Tier A asserts here
                    │   Matrix engine               │◀── Tier A
                    │   AgentTestEnv                │◀── Tier B + Tier C
                    │   ReplayFixture / Setup*       │◀── Tier B
                    │   RealModelEntry / config      │◀── Tier B + Tier C
                    └───────────────────────────────┘
```

Notably, **Tier B reuses Tier A's `Scenario.Assertions`** for the `virtual`
upstream — the same content checks that validate transforms in isolation also
validate them through the full gateway pipeline.
