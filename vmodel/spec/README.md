# Unified VModel definitions

`spec.Definition` is the file contract for model identity, behavior, supported
protocols and test expectations. `spec.Load` accepts JSON or YAML, one object
or a list. `spec.New` compiles those definitions once for both HTTP serving
and the existing scenario/matrix adapters.

```sh
# Start a standalone provider; no running tb instance is required.
tb vmodel --config model.yaml
# The identical file supplies the upstream response and its assertions.
go run ./cli/harness matrix --mode single --config model.yaml
```

A small shared definition:

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

Common fields are `id`, optional `name`/`description`, `kind`, optional
`protocols`, `tags`, `expect`, `structural` and `skip_transitive`. Exactly one
behavior block is allowed. Both `id` and `kind` are required; identity and
metadata live only in the outer envelope.

| Kind | Behavior block | Meaning |
| --- | --- | --- |
| `mock` | `mock` | Content, multiple `tool_calls`, chunks, usage, completion reasons and synthetic errors |
| `recording` | `responses` | Per-protocol JSON bodies and SSE recordings |
| `sequence` | `sequence` | Ordered `steps`, repetitions, default content and exhaustion policy |
| `decision` | `decision` | `first`/`stable` strategy and confidence |
| `transform` | `transform` | A registered builtin transform, e.g. `compact-round-only` |
| `builtin` | `builtin` | A stable reference to a registered Go behavior, e.g. `scenario/tool_result` |

Durations in mock/sequence blocks accept strings such as `50ms` or integer
nanoseconds; export uses readable strings. The protocol names are
`openai_chat`, `openai_responses`, `anthropic`, `google`, and `decisions`.
Omitting `protocols` exposes the protocols the behavior actually supports;
explicit selections are checked against those capabilities. An unknown model
or unsupported protocol returns 404, and missing stream/nonstream recordings
return 400. A request never falls back to another model or response format.

A recording uses the same identity and expectations:

```yaml
id: recorded
kind: recording
responses:
  openai_chat:
    status: 200
    body:
      id: chatcmpl-recorded
      object: chat.completion
      model: recorded
      choices:
        - index: 0
          message: {role: assistant, content: Hello.}
          finish_reason: stop
expect:
  http_status: 200
  content_equals: Hello.
```

`body` stores structured JSON. `raw_body` stores exact response bytes,
including whitespace or intentionally malformed JSON. `stream` retains the
SSE line representation. `raw_stream` retains exact SSE bytes,
including comments, CRLFs, IDs, multiline data and event separators. Body
alternatives and stream alternatives are mutually exclusive. Completion
frames are never appended to a recording. `stream_http_error` selects a
pre-content HTTP failure; its status must agree with `status` if both are set.

`expect` uses the existing assertion library. Alongside content/status/tool
checks, `body_contains` and `json_fields` work on the raw response body for
native endpoints. Shared matrix expectations must apply to the client-facing
protocol after conversion: for example, completion reasons differ between
Chat (`stop`), Anthropic (`end_turn`) and Responses (`completed`). A native
recording should use portable content/status checks when testing all clients:

```yaml
id: choose
kind: decision
decision: {strategy: first}
expect:
  http_status: 200
  json_fields:
    answers.pick.choice: a
```

In Go, pass an HTTP result with `HTTPStatus` and `RawBody` to
`runtime.Check(id, result)`. Chat-specific checks additionally use the parsed
fields of the existing `check.RoundTripResult`. A missing expectation is an
error when checking, rather than an automatic pass. The chat protocol matrix
uses `runtime.Scenario(id)`; native-only decisions are exercised through
`/v1/decisions` and `Check`. Missing matrix protocols/modes are skipped.

## Existing data and Go behavior

All model files use the common envelope. Flat mock/sequence documents,
`name`/`mock_responses` documents and singular `tool_call` are rejected.
Use `tool_calls`, including for a single call. The separate `LoadMockSpec`,
`DecodeMockSpec`, `scenario.LoadFixture` and `scenario.DecodeFixture` APIs
and protocol-specific `MockScenario` wrappers have been removed. Runtime
mock/sequence structs remain typed Go behavior configuration, not alternative
file formats.

```sh
# Export the complete core inventory, including opt-in test models/scenarios.
tb vmodel --export catalog.yaml
# Combine canonical definitions into one list without starting a provider.
tb vmodel --config model.json --config recorded.yaml --export models.yaml
# Load the exported inventory again.
tb vmodel --config catalog.yaml
```

`DefaultDefinitions` covers every model shipped by the existing default
registries. Plain mocks and sequences become data; registered transforms keep
behavior references. `AllDefinitions` additionally includes usage mocks,
extended errors and every `scenario.AllScenarios()` validation fixture.
Test-only definitions are not added to production defaults.

Go functions cannot be serialized as JSON/YAML. Request-dependent builtin
fixtures keep their original builders and Go assertions through named
references. Their responses are not captured once and flattened into static
payloads. Custom Go code registers a name before loading:

```go
catalog := spec.NewCatalog()
err := catalog.Register("company/tool-round", func(d spec.Definition) (scenario.Scenario, error) {
    s := makeFreshToolRoundScenario() // retains NonStreamFor/StreamFor and Go assertions
    s.Name = d.ID
    return s, nil
})
// Handle err, then compile with the same definitions used by files.
runtime, err := spec.NewWithCatalog(definitions, catalog)
```

`RegisterModels` similarly adapts custom OpenAI/Anthropic models, decision
choosers and transform chains. Each factory must return fresh instances for a
new runtime. Files refer to these names with `kind: builtin` and
`builtin: company/tool-round`; a binary without the registration fails clearly.

`FromMock` and `FromSequence` wrap common typed Go values in the canonical
envelope. Protocol-specific scenario constructors have been consolidated into
the same shared mock configuration, with `tool_calls` in both protocols.
Anonymous application-specific callbacks must be given a name by their owning
package; they are not automatically discovered or exported. Captured requests
and golden output files remain test inputs/evidence, rather than being
misidentified as complete model definitions.
