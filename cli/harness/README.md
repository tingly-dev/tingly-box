# Tingly-Box Protocol Validation Harness

A standalone CLI (`harness`) that validates the tingly-box gateway end-to-end:
protocol transforms, routing rules, scenario dispatch, and real agent-CLI
compatibility — without needing a deployed server.

> Forward-looking work — new scenarios, fixture capture, CI integration,
> closing the defect skip list — is tracked in [PLANNING.md](./PLANNING.md).

The commands cover **three orthogonal axes**, each a small ladder of its own:

```
  axis 1: protocol-conversion correctness          axis 2: routing & dispatch behavior
  ─────────────────────────────────────            ────────────────────────────────────
   matrix   synthetic request cross-product         lb        LB dynamics simulator
            through an in-process gateway                     (fake upstreams, fake clock:
   replay   captured agent fixtures through                    breaker, failover, affinity)
            the in-process gateway                  routing   smart-routing e2e on the duo
   agent    real agent CLI spawned against                    topology (rule API → trace)
            the in-process gateway
                                                   axis 3: resource health
   (fidelity rises down each list;                 ─────────────────────────
    every rung shares one assertion                 duo       per-instance memory
    vocabulary: vmodel/benchmark/check)                       observation across two
                                                              full server processes
```

All single-process rungs (matrix / replay / agent) run **the full gateway
pipeline over real HTTP** — an in-process `httptest.Server` wired to a
VirtualServer mock provider; they differ in where the *request* comes from
(synthetic cross-product vs. captured fixture vs. a real CLI's own
construction). The duo-topology rungs (duo / routing) trade speed for
process-level fidelity: two full `server.Start()` child processes and real
TCP between them.

### Scope

The harness guards the **AI data plane** — protocol conversion, traffic
routing/dispatch, agent-client compatibility, and the resource health of that
path — because that is the product's core. This boundary is deliberate:

- **In scope:** everything a request touches between an agent client and an
  upstream model — transforms, rules, smart routing, LB/breaker/affinity,
  streaming, memory behavior under agentic traffic.
- **Out of scope (by design):** the management plane (CRUD APIs, GUI,
  frontend), remote control, and other non-gateway subsystems — those are
  covered by their own unit/integration tests. duo/routing intentionally
  *cross* two management surfaces (the rule API and the `/api/v1/requests`
  trace) because user configuration and explainability are part of the data
  plane's contract, but the harness does not aim to e2e the management plane.
- **Known open axes** (product-decision-gated, not accidental gaps): the
  Google target path, multimodal (image-block) scenarios, and automated
  real-provider conformance (Tier D). Roadmapped work lives in
  [PLANNING.md](./PLANNING.md).

Every hermetic mode runs in CI — see
[`.github/workflows/harness-matrix.yml`](../../.github/workflows/harness-matrix.yml)
and PLANNING §4 for the leg list and the deliberate carve-outs (duo memory
phase, real upstreams).

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

Validates **protocol transformation** as an exhaustive cross-product. Each
cell boots (or reuses) an in-process gateway + VirtualServer mock, sends a
minimal synthetic request as the source protocol over real HTTP, and asserts
on the round-trip result — so the full pipeline (routing, transform,
dispatch, response conversion) is on the tested path, not just the transform
functions.

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
   (Scenario.Assertions) (Scenario.Structural) (Scenario.Structural)
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
| `virtual`    | built-in rule → in-process `VirtualServer` mock | `Scenario.Assertions` (content-level — response is deterministic) |
| `vmodel`     | built-in rule → seeded builtin vmodel provider, dispatched in-process via `provider.IsVirtual()` short-circuit | `Scenario.Structural` (upstream-independent) |
| `real`       | built-in rule → live provider from `--config`   | `Scenario.Structural` (upstream-independent) |

Only the `virtual` upstream controls the response byte-for-byte, so only it can
run the scenario's **content** assertions. `vmodel` and `real` responses aren't
test-controlled, so they get the scenario's **structural** checks (HTTP 200,
non-empty content, tool-call count, stream-event count) — both tiers are
defined on the Scenario itself (`vmodel/benchmark/scenario/builtins.go`), so a
new replay scenario needs only the matrix ctor and a vmodel id. Streaming runs
additionally assert the full client-facing SSE frame shape
(`StreamShapeForAgent`) on every upstream.

### Skip list

Known gateway defects are registered **once**, in protocoltest's
`skipSourceScenarios` (the matrix reads it directly; replay derives its skips
via `KnownDefectReason` + the agent's source protocol). Each entry is a
**real defect**, not a test artifact — fixing one is a one-line deletion in
one place. Currently:

- `openai_responses|tool_use` (+ streaming variant) — the Responses-API
  source path's tool_call conversion is incomplete; skips Tier A's
  openai_responses-source cells and every `codex/tool_use` replay run.

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

- `--mock` — virtual-model mode: gateway wired to an in-process mock upstream
  serving the shared text scenario. Exercises protocol translation + rule
  matching with zero external calls. Success requires more than a zero exit
  code: the CLI's output must contain the mock's fixed answer
  (`protocoltest.VirtualMockAnswerMarker`), so a CLI that prints an error yet
  exits cleanly is a FAIL.
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

The `tier` tactic supports a `within_tier_tactic` field (`random` by default) to choose how load is shared among services in the same tier. `--example withintier` demonstrates it with `random`, asserting that both top-tier peers appear as first attempts across 20 requests.

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
faults:                      # serviceID -> per-call status program
  # Shorthand: a bare status list. The last entry repeats once exhausted.
  openai/gpt-5.5: [200]
  # Structured form (same engine, vmodel.Sequence): `repeat` compacts runs and
  # `on_exhaust` picks loop | clamp (default, = last entry repeats) | fail.
  # The block below is equivalent to [500, 500, 500, 200].
  openai/gpt-5.4:
    steps:
      - { status: 500, repeat: 3 }
      - { status: 200 }
    on_exhaust: clamp
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

## Tier Duo — `duo`

> Design rationale, fidelity ledger, and observation contract:
> [`.design/harness-duo.md`](../../.design/harness-duo.md)

Two full tingly-box instances as **separate server processes**, verified
end-to-end over **real HTTP**: `tb2` (the gateway under test) routes
anthropic-scenario rules to `tb1`'s production vmodel endpoints, and the
harness drives Claude-Code-shaped conversations (megabytes of context per
turn) through tb2's protocol-conversion paths.

```
                 harness (parent process — drives requests, never measured)
                    │
                    │ Anthropic v1/beta stream          per-instance sampling
                    ▼                                   /api/v1/debug/{memstats,pprof/heap}
   ┌──────────────────────────────┐    real HTTP    ┌──────────────────────────────┐
   │  tb2  (gateway under test)   ├────────────────▶│  tb1  (vmodel upstream)      │
   │  own process, server.Start() │                 │  own process, server.Start() │
   └──────────────────────────────┘                 └──────────────────────────────┘
```

Each instance is booted through the full production path (`server.Start`:
background refreshers, config watcher, production `http.Server` timeouts) by
re-executing the harness binary itself, and — because each has its own Go
runtime — memory is observed **per instance** over the production
`/api/v1/debug/memstats` + `/api/v1/debug/pprof/heap` endpoints. A retention
slope therefore attributes directly: tb2 slope → gateway/conversion path,
tb1 slope → vmodel serving path. (The same endpoints work against any live
deployment for incident diagnosis.)

Every anthropic-source route the vmodel can back is wired:

| route            | source          | target                      | tb1 endpoint                       |
|------------------|-----------------|-----------------------------|------------------------------------|
| `beta-chat`      | Anthropic beta  | OpenAI Chat                 | `/virtual/openai/v1/chat/completions` |
| `beta-responses` | Anthropic beta  | OpenAI Responses            | `/virtual/openai/v1/responses`     |
| `beta-anthropic` | Anthropic beta  | Anthropic (passthrough)     | `/virtual/anthropic/v1/messages`   |
| `v1-chat`        | Anthropic v1    | OpenAI Chat                 | `/virtual/openai/v1/chat/completions` |
| `v1-responses`   | Anthropic v1    | OpenAI Responses            | `/virtual/openai/v1/responses`     |
| `v1-anthropic`   | Anthropic v1    | Anthropic (passthrough)     | `/virtual/anthropic/v1/messages`   |

(The Google target is deliberately not covered — the vmodel surface skips it
for now.)

Every route also has a **`-slow` backpressure variant** (e.g.
`beta-chat-slow`): tb1 answers from a slow/large stream vmodel
(`--stream-kb` KB over roughly 2×`--stream-ms` wall time) while the harness
reads the SSE body slowly (`--read-delay-ms` between reads), so buffering
and pinning under real TCP backpressure — invisible with instant mock
responses — are on the measured path.

Two phases, both on by default:

- **Functional** — SSE event shape (`message_start` … `message_stop`),
  assembled content, `stop_reason`, usage propagation, and the non-streaming
  response body.
- **Memory** — per instance: allocation churn per request, **post-GC
  retention slope** across two sequential batches (a leak shows as a
  positive slope; transient spikes do not), peak live heap under a
  concurrent burst, and goroutine counts. This is the setup that pinned down
  #1255: 823 KB/request retained on the gateway before the fix, ~0.5 KB
  after. The run fails if **either instance's** slope exceeds
  `--max-slope-kb` (default 32). Default routes: `beta-chat` (fast) +
  `beta-chat-slow` (backpressure).

```bash
./harness duo                                   # functional: all routes; memory: beta-chat fast + backpressure
./harness duo --mem-routes all                  # memory slope on every fast route
./harness duo --mem-routes beta-chat-slow --stream-kb 1024 --stream-ms 2000 --read-delay-ms 50   # heavy backpressure
./harness duo --routes beta-responses,v1-responses --skip-memory
./harness duo --body-mb 4 --batch 30            # heavier sweep
./harness duo --skip-func --profile-dir /tmp    # memory only, write per-instance pprof heap profiles
go tool pprof -top -inuse_space /tmp/duo-beta-chat-tb2-final.pb.gz
./harness duo --json                            # machine-readable report
./harness duo -v                                # relay both children's server logs
```

The engine lives in `internal/protocoltest/duo*.go` and is shared with the
`TestDuoFunctional` / `TestDuoMemoryRegression` / `TestDuoBackpressure` Go
tests (whose `TestMain` re-executes the test binary as the child servers), so
CI guards the same per-instance slope threshold.

## Tier Routing — `routing`

> Design rationale and trace-surface contract:
> [`.design/harness-duo.md`](../../.design/harness-duo.md) §"Routing scenarios"

Smart-routing e2e verification on the duo topology. Each scenario is a rule
shape (base services + smart-routing partitions) plus a request program;
rules are created through **tb2's production rule API** — the same path a
user's configuration takes — and every decision is asserted on two
independent surfaces:

- **wire level** — tb1 hosts a pool of service-identity vmodels
  (`duo-svc-a` … `duo-svc-f`), each answering with its own marker, so which
  service a request landed on is read directly from the response body;
- **explanation level** — tb2's `/api/v1/requests/:id` timeline joins the
  `smart_routing` evaluation trace by the harness-supplied `X-Request-Id`;
  the engine asserts `outcome`, the matched partition description, and the
  final `routed_model` — the same surface a user debugging their routing
  config reads.

```bash
./harness routing                      # all built-in scenarios
./harness routing --list               # catalog with descriptions
./harness routing --scenarios pipeline-health-before-smart-routing,pipeline-smart-routing-before-affinity,pipeline-affinity-before-load-balancer,pipeline-smart-routing-before-load-balancer
./harness routing --file my-rules.yaml # user-defined scenarios (see testdata/routing/example.yaml)
./harness routing --json               # machine-readable report
```

Built-ins cover the smart-routing position catalog (token, thinking,
context_user, model, time_range, agent.claude_code), **first-match ordering**,
and four explicit pipeline invariants: **health before smart-routing**,
**smart-routing before affinity**, **affinity before load-balancer**, and
**smart-routing before load-balancer**. Each pipeline scenario asserts the
opt-in debug routing source and exact evaluated-stage path in addition to the
wire responder and smart-routing timeline. Time-range windows are built
relative to the wall clock — hours-wide margins, no clock seam needed.

Semantics worth knowing when authoring scenarios:

- When **no partition matches**, the LB falls back to the **union** of base
  + partition services, so `expect.svc` is only deterministic for matched
  requests; miss requests assert `outcome: no_match` on the trace instead.
- Pipeline expectations can assert `source`, `selected_model`, and the exact
  ordered `stages`. `svc` remains the independent final wire responder, so a
  failover scenario can distinguish the initially selected service from the
  service that ultimately answered.
- `service_ttft` / `service_capacity` (stats-driven, pass on empty data)
  and `proxy_vision` (processor-bearing bypass op) are not covered yet.

**Use it for:** answering "given this rule config and this request shape,
where does the request go and why" — machine-checked, over the full
pipeline (rule API → extraction → smart stage → affinity → LB → conversion
→ upstream).

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
  replay.go          Tier B command — fixture replay, upstream selection
  duo.go             Tier Duo command — wraps protocoltest.DuoEnv (function +
                     per-instance memory); child mode via MaybeRunDuoServe in main.go
  routing.go         Tier Routing command — smart-routing scenarios on the duo
                     topology (built-ins, --file YAML, wire + trace assertions)
  duo_report.go      shared duo-topology CLI scaffolding (boot line, check
                     blocks, JSON/PASS/exit)
  testdata/routing/  example user-defined routing scenario YAML
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
  matrix.go          Tier A cross-product engine + skipSourceScenarios (the
                     known-defect registry; replay reads it via KnownDefectReason)
  testenv.go         TestEnv + gatewayCore (the shared single-process env
                     skeleton) + dispatch (real-HTTP send for both modes)
  agent_env.go       AgentTestEnv: built on gatewayCore + SetupAgent / SetupVModelAgent
  replay.go          SetupVirtualAgentScenario, ReplayFixture, stream-shape
                     assertions (AnthropicStreamShape / ResponsesStreamShape)
  aliases.go         re-exports of the check/scenario foundation (the canonical
                     Assertion library and Scenario fixtures live in
                     vmodel/benchmark/{check,scenario})
  real_model.go      providers.yaml parsing for --upstream=real / --config
  duo.go             Tier Duo parent: routes, child spawning, request driving
  duo_serve.go       Tier Duo child: full server boot + gateway/vmodel seeding
  duo_checks.go      Tier Duo functional phase
  duo_memory.go      Tier Duo memory phase (per-instance sampling over HTTP)
  duo_routing.go     Tier Routing engine: scenario model, rule-via-API seeding,
                     wire + smart_routing-trace assertions
  duo_routing_scenarios.go  built-in scenario catalog + YAML loader
```

---

## How the tiers share code

The commands are thin CLI shells over one engine (`internal/protocoltest`),
which itself consumes the shared check/scenario foundation:

```
   vmodel/benchmark/check      Assertion + RoundTripResult — the ONE assertion
                               vocabulary (matrix, replay, duo functional)
   vmodel/benchmark/scenario   Scenario fixtures: mock responses + content
                               Assertions + upstream-independent Structural
                    ▲
                    │
   internal/protocoltest
     Matrix engine + known-defect registry   ◀── matrix (test + CLI, one path)
     gatewayCore → TestEnv / AgentTestEnv    ◀── matrix / replay / agent
     ReplayFixture / Setup* / stream shapes  ◀── replay
     DuoEnv (two processes) + DuoCheck       ◀── duo / routing
     RealModelEntry / providers.yaml         ◀── replay / agent
```

Notably, **replay reuses the matrix's `Scenario.Assertions`** for the
`virtual` upstream and `Scenario.Structural` elsewhere, and the duo
functional phase runs the same assertion library over its two-process
round trips — one vocabulary at every fidelity level.
