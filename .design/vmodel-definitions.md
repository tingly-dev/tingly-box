# VModel definitions and shared runtime

Status: implemented in PR #2618. This document owns the file contract and
execution boundaries. Field examples and Go usage live in
[`vmodel/spec/README.md`](../vmodel/spec/README.md); the design index is
[`vmodel.md`](./vmodel.md).

## Decision

Use `vmodel/spec.Definition` as the common envelope for model definitions,
recorded responses and executable test fixtures. JSON and YAML describe the
same values. Compile them through `spec.New` for HTTP serving and for the
existing scenario/check adapters, so serving and testing do not interpret the
same file through different loaders.

Keep the existing protocol implementations, benchmark observation layer and
assertion vocabulary. This unifies identity, configuration and execution;
it does not require every behavior to become a static response recording.

## Common envelope

```yaml
id: hello
kind: mock
mock:
  content: Hello.
  delay: 10ms
expect:
  http_status: 200
  content_equals: Hello.
```

| Field | Contract |
| --- | --- |
| `id` | Required model identity; unique within the compiled runtime. |
| `name`, `description` | Optional display metadata; name defaults to id. |
| `kind` | Behavior discriminator; the loader can infer it from the behavior block. Canonical examples and exports use it explicitly. |
| `protocols` | Optional restriction to supported formats; cannot add a capability the behavior lacks. |
| `tags` | Optional metadata. |
| `expect`, `structural` | Optional declarative assertions using the existing check layer. |
| `skip_transitive` | Matrix metadata for cases unsuitable for two-hop comparisons. |

Exactly one behavior block is allowed. The outer shape is identical for every
kind; only the behavior block changes:

| Kind | Block | Execution |
| --- | --- | --- |
| `mock` | `mock` | Semantic content, tool calls, usage, latency and error simulation. |
| `recording` | `responses` | Per-protocol JSON and SSE responses. |
| `sequence` | `sequence` | Ordered outcomes, repetitions and exhaustion policy. |
| `decision` | `decision` | Native decision strategy (`first` or `stable`). |
| `transform` | `transform` | Registered builtin transform. |
| `builtin` | `builtin` | Stable name of a Go behavior factory. |

`spec.Load` accepts a single object or a nonempty list. Repeated CLI `--config`
arguments concatenate definitions; duplicate IDs fail rather than overwrite.
No schema-version envelope or additional `serve` command is introduced.
Mock/sequence durations accept strings or integer nanoseconds; export writes
readable strings.

## Compilation and state

```text
JSON/YAML or existing Go values
  → import adapters → Definition → normalize/validate → spec.Runtime
      ├ HTTP handler → tb vmodel
      ├ Scenario(id) → protocoltest.Matrix → gateway round trip
      └ Check(id, result) → shared assertions, including native endpoints
```

The runtime starts with an empty service and installs only the supplied
definitions. Default CLI startup chooses `DefaultDefinitions`; supplying files
replaces that inventory. Export without files uses `AllDefinitions`, which
also includes opt-in test mocks and validation scenarios (37 core definitions
at implementation time). Exporting those definitions does not add them to the
production default registries.

Sequence cursors and dynamic Go behavior belong to a runtime. Catalog factories
must return fresh instances for each compilation. Registered Go callbacks keep
request-dependent builders and assertions; they are not executed once during
export and flattened into a recording. Anonymous callbacks require an explicit
catalog name; loading an unavailable name fails.

## Response fidelity and protocol limits

Supported format names are `openai_chat`, `openai_responses`, `anthropic`,
`google`, and `decisions`. These names describe capabilities, not a promise
that every model implements every endpoint. Unknown models and unsupported
protocols return 404. A missing recording mode (stream/nonstream) returns 400;
there is no fallback to another model, protocol or mode.

Recordings choose structured `body` or exact `raw_body`, and legacy SSE lines
in `stream` or exact `raw_stream`. Each pair is mutually exclusive. Raw SSE
preserves framing, comments, IDs and CRLFs; serving does not append completion
frames. `stream_http_error` describes a pre-content HTTP error and must agree
with `status` when both are supplied. Semantic mocks support multiple tool
calls and preserve explicit IDs and raw argument JSON, including large integer
values.

Serving does not require expectations. Testing does: `Runtime.Check` rejects
missing checks. `Runtime.Scenario` adapts definitions to the chat matrix;
unsupported protocols or recording modes are skipped. Native-only decision
cases use their HTTP endpoint and `Check` with status/raw-body results instead.
`body_contains` and `json_fields` allow checks without chat normalization.

## CLI lifecycle and test entry points

```sh
tb vmodel --config model.yaml --json
go run ./cli/harness matrix --mode=single --config model.yaml
tb vmodel --config old-model.json --export normalized.yaml
tb vmodel --export catalog.yaml
```

In #2618, `tb vmodel` is standalone by default: bind `127.0.0.1:0`, report the
selected address and IDs, serve HTTP, and shut down on interrupt/SIGTERM. It
does not open the application database or require a running gateway. JSON
readiness includes `url`, `openai_base_url`, `anthropic_base_url`, `models`,
and `google_base_url`. OpenAI uses `/openai/v1`, Anthropic uses `/anthropic`
as its SDK base; Google uses the root URL. Export exits without starting a
listener. There is no `--standalone` flag in this base PR.

Harness `--config` (`--fixture` alias) uses the identical loader and requires
`--mode=single` plus test assertions. Existing built-in matrix behavior remains
the default when no file is supplied. `--config` here is a VModel definition;
agent/replay live-provider configuration remains a separate contract.

## Existing data migration

| Existing input | Import into the common contract |
| --- | --- |
| Flat `SharedMockSpec` | Identity/metadata plus `kind: mock` and `mock`. |
| Flat `SequenceConfig` | Identity/metadata plus `kind: sequence` and `sequence`. |
| `name` / `mock_responses` fixture | Name becomes id when absent; responses become `responses` under `kind: recording`. |
| Existing Go models/scenarios | `FromMock`, `FromSequence`, `FromFixture`, protocol-scenario adapters, or a named catalog factory. |
| Captured requests and golden outputs | Remain test inputs/evidence; they do not by themselves define executable model behavior. |

Canonical examples in `vmodel/testdata` share the id/kind envelope; legacy
syntax is isolated under `vmodel/testdata/legacy` for compatibility coverage.
Old Go loading APIs remain available, while export always emits the canonical
shape. A builtin reference preserves behavior by name and requires a binary
with that registration; it is not portable executable code.

## Scope and verification

Scenario routing, the VModel UI/power-up and automatic gateway registration
belong to stacked PR #2621. They are not required by this file/runtime contract.
A Python construction SDK remains future work.

Validation covers normalization/legacy import, catalog round trips, fresh
runtime state, exact wire responses, protocol restrictions, native checks and
CLI lifecycle. The complete harness run on `de0c8275` passed matrix-all, all
four SDK drivers, virtual/vmodel replay, LB, duo including default memory
checks, routing and catalog checks, plus the harness/protocoltest package
suites. The run retained registered skips and two existing FP3 recording gaps;
live-provider credentials and missing real agent CLIs were outside that run.
