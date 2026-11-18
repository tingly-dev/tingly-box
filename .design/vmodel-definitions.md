# VModel definitions and shared runtime

Status: implemented in #2618, with strict-format cleanup in #2624.
This document owns the file contract and
execution boundaries. Field examples and Go usage live in
[`vmodel/spec/README.md`](../vmodel/spec/README.md); the design index is
[`vmodel.md`](./vmodel.md).

The VModel scene in #2621 provides a fixed direct address for tb's builtin
catalog. The CLI remains standalone and never registers gateway rules. See
[`vmodel-scenario.md`](./vmodel-scenario.md).

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
| `kind` | Required behavior discriminator; omission is an error. |
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
  → Definition → normalize/validate → spec.Runtime
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

Recordings choose structured `body` or exact `raw_body`, and SSE lines
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
tb vmodel --config model.json --export models.yaml
tb vmodel --export catalog.yaml
```

In #2618, `tb vmodel` is standalone by default: bind `127.0.0.1:0`, report the
selected address and IDs, serve HTTP, and shut down on interrupt/SIGTERM. It
does not open the application database or require a running gateway. JSON
readiness includes `url`, `openai_base_url`, `anthropic_base_url`, `models`,
and `google_base_url`. OpenAI uses `/v1`, Anthropic uses the root URL
as its SDK base; Google uses the root URL. Export exits without starting a
listener. No `--standalone` flag is needed.

Harness `--config` (`--fixture` alias) uses the identical loader and requires
`--mode=single` plus test assertions. Existing built-in matrix behavior remains
the default when no file is supplied. `--config` here is a VModel definition;
agent/replay live-provider configuration remains a separate contract.

## Completed migration and cleanup

All model definition examples and executable test definitions use the explicit
`id`/`kind` envelope. The old `legacy/` examples duplicated canonical examples
and have been removed. Alternate flat mock/sequence files and
`name`/`mock_responses` fixtures are rejected; there is no automatic name-to-id
mapping, kind inference or flat-field promotion.

`spec.Load` / `spec.Decode` are the only file loaders. Separate mock/fixture
loaders, fixture file envelopes and protocol-specific `MockScenario`
constructors have been removed. Common Go mock and sequence structs remain
runtime behavior configuration; `FromMock` / `FromSequence` supply their
canonical envelope. Recording compilation uses a shared response builder
without a second file contract. Single and multiple tool calls both use
`tool_calls`; singular-field dispatch branches have been deleted.

Protocol requests, golden wire outputs and LB simulation programs keep their
own contracts: they are inputs/evidence or simulator configuration, not model
definition files. Named Go factories preserve dynamic behavior that cannot be
serialized. Export always emits the canonical shape; loading a builtin reference
requires a binary with that behavior registered.

## Scope and verification

The fixed builtin-model endpoint and the VModel UI/power-up belong to PR
#2621. They are not required by this file/runtime contract; neither entry point
automatically registers gateway resources.
A Python construction SDK remains future work.

Validation covers strict schema rejection and normalization, catalog round trips, fresh
runtime state, exact wire responses, protocol restrictions, native checks and
CLI lifecycle. The full harness validation covers matrix-all, all
four SDK drivers, virtual/vmodel replay, LB, duo including default memory
checks, routing and catalog checks, plus the harness/protocoltest package
suites. Registered skips and two existing FP3 recording gaps remain visible;
live-provider credentials and missing real agent CLIs were outside that run.
