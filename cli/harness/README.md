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

Simulates **load-balancing dynamics** — tier selection, mid-request failover, the breaker (trip + *timed*
recovery), and affinity stickiness / re-pinning — over a **request sequence** against programmable fake
upstreams. It runs the real `ServiceSelector.Select → dispatchWithPriorityFailover` path with a
deterministic breaker clock, so recovery is exercised without sleeping.

```bash
./harness lb --example cascade        # cascade | flat | grid | single | regression | ratelimit | authflip | crossmodel | halfopen | degrade | inactive | withintier | multiaffinity
./harness lb --file scenario.yaml     # your own shape
./harness lb --example grid --table   # compact table instead of the default graph
./harness lb --example grid --json    # machine-readable trace
```

### Output

The default is a pencil graph — per request, the failover hops plus a state line (each svc's breaker/health
+ the affinity pin), so the trip, health exclusion, and pin movement are visible step by step:

```
#3  s1   openai/gpt-5.4 ✗500  →  openai/gpt-5.5 ✓200   →  client=200
       state: openai/gpt-5.4=open/unhealthy   openai/gpt-5.5=closed/healthy   pin=openai/gpt-5.4
#4  s1   openai/gpt-5.5 ✓200   →  client=200
       state: openai/gpt-5.4=open/unhealthy   openai/gpt-5.5=closed/healthy   pin=openai/gpt-5.5
```

`--table` gives a compact one-line-per-request form; `--json` the same data structurally. The engine
(`internal/server.LBSimulator`) is shared with the Go scenario tests, so the CLI and CI assert the same thing.

### HTTP status semantics

The fault scripts are status codes, and the sim classifies them into the **two
production feedback channels** exactly as a real request would (mirroring
`Server.reportHealthStatus` + the breaker recorder), so the *special* codes
behave faithfully:

| status | failover (real loop) | breaker | health monitor |
|--------|----------------------|---------|----------------|
| 2xx | commit | success | `ReportSuccess` |
| 429 | retry | failure | **rate-limited** (unhealthy for a window) |
| 500/502/503/504 | retry | failure | error (3-strike) |
| 401/403 | **terminal** | failure | **auth — immediately unhealthy** |
| 400/404/other | **terminal** | failure | error (3-strike) |

So a single **429** or **401/403** marks the service unhealthy on the *first* hit (health channel) and skips
it next request — distinct from the breaker's 3-strike trip; `final health` / JSON `final_health` show it.
Try `--example ratelimit` (429 → skipped → recovers) and `--example authflip` (401 → terminal + excluded).

### Cross-model failover and strict affinity TTL

Two behaviours the sim also models faithfully, matching recent fixes on the routing path:

- **Cross-model failover** — when tiers carry different models, a failover hop dispatches the *fallback's own*
  model, not the primary's. `--example crossmodel` shows it in the attempt column (`openai/gpt-5.4 → anthropic/claude-opus`).
- **Strict affinity TTL** — a lock expires exactly at `LockedAt + affinity_secs`; an in-window request is
  honored but does **not** extend it. The affinity TTL rides the same fake clock as the breaker/health
  (one `advance` moves all three), so the Go scenario suite asserts expiry-and-re-lock deterministically.

### Self-check (`expect`)

Built-in examples include an optional `expect` block that self-checks expected outcomes after the program runs. On mismatch, the CLI exits non-zero — useful for CI and for validating user `--file` scenarios. Fields (all optional):

- `final_status` — last request's `FinalStatus`
- `attempts` — last request's `Attempts` (exact, in order)
- `attempts_contain` / `attempts_exclude` — set membership checks
- `pin` / `pins` — final affinity pin (single-session or per-session)
- `breaker` / `health` — final snapshot subsets
- `distinct_first_attempts` — set of first-attempt serviceIDs across ALL request steps (within-tier load sharing)

All 13 built-in examples self-verify. The `expect` block is also available in `--file` scenarios, so users can self-check their own rules.

### Within-tier sub-tactic

The `tier` tactic supports a `within_tier_tactic` field (`random` by default; also `token_based`, `latency_based`, etc.) to choose how load is shared among services in the same tier. `--example withintier` demonstrates it with `random`, asserting that both top-tier peers appear as first attempts across 20 requests.

### Scenario file

A small YAML describes the rule *shape*, per-service fault scripts, and a program
of requests / clock-advances (see `testdata/lb/cascade.yaml`):

```yaml
rule_uuid: cascade
tactic: tier                 # tier | random
within_tier_tactic: random   # optional: within-tier sub-tactic (default: random); tier tactic only
affinity_secs: 1800          # 0 = off
services:
  - { provider: openai, model: gpt-5.4, tier: 0 }
  - { provider: openai, model: gpt-5.5, tier: 1 }
faults:                      # serviceID -> per-call status sequence (last entry repeats)
  openai/gpt-5.4: [500, 500, 500, 200]
  openai/gpt-5.5: [200]
seed_pin: { session: s1, provider: anthropic, model: claude-sonnet }   # optional: pre-lock a stale pin
expect:                      # optional: self-check assertions (all fields optional)
  final_status: 200
  attempts: [openai/gpt-5.4]
  pin: openai/gpt-5.4
  breaker: { openai/gpt-5.4: closed, openai/gpt-5.5: closed }
program:
  - { request: s1 }          # request on session s1 (omit/"" = no affinity)
  - { advance: 31s }         # move the breaker clock forward
```

The shapes map to the **"Rule config shapes" taxonomy** in
`.design/tier-routing.md` (Single / Flat / Cascade / Grid). The
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
                     .design/tier-routing.md)
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
