# Virtual sequence models

## Problem

The `vmodel` package already ships always-fail error models
(`virtual-fail-429`, `virtual-fail-500`, …) that reproduce a single failure
shape on every request. That is enough to assert "the gateway surfaces a 429",
but it cannot reproduce the *temporal* behaviour of a real upstream — a
provider that succeeds a few times, rate-limits, then recovers. Failover,
retry, and backoff logic only becomes observable when a model returns
`200, 200, 429, 200, …` across successive requests.

Standing up an ad-hoc `httptest.Server` per scenario reintroduces exactly the
duplication the `vmodel` error models were meant to remove. We want the same
deterministic, in-process, wire-correct substrate, but driven by a configurable
*program* of outcomes.

## Design

### A step is success-or-failure, keyed on status

`vmodel.SequenceStep`:

| Field     | Meaning                                                                 |
| --------- | ---------------------------------------------------------------------- |
| `Status`  | `0`/`200` → success (return content); anything else → pre-content error |
| `Content` | success body; falls back to `SequenceConfig.DefaultContent`             |
| `Message` / `Type` | error envelope overrides; derived from `Status` when empty     |
| `Repeat`  | serve this step N consecutive times (default 1)                        |

`SequenceConfig` is the ordered program plus identity/metadata and a `NoLoop`
flag (default loops back to the first step so the model is reusable
indefinitely; `NoLoop` clamps to the last step once exhausted).

Error steps map onto the **existing** `ErrorInjection` pre-content path — the
status's conventional `Type`/`Message` mirror the always-fail mocks
(`429 → rate_limit_error`, `503 → overloaded_error`, …). Mid-stream failures
are intentionally **out of scope** for v1: the "200, 200, 429" use case is
pre-content, and folding mid-stream into the sequence step would complicate the
config for a case the dedicated `virtual-fail-midstream-*` mocks already cover.
A future `SequenceStep` could grow `Stage`/`MidStream*` fields without breaking
the wire shape.

### Per-request resolution — the key decision

The registry holds **one** shared model instance, but a sequence inherently
carries a cursor. The handler touches `vm` at several points in a single
request (`ExtractErrorInjection` for the pre-content check, then
`HandleAnthropic`/`midStreamInjection` on the body path), so advancing a cursor
inside any one of those would desync the others.

Instead, `SequenceModel` implements a new per-protocol optional interface:

```go
type RequestResolver interface { ResolveRequest() VirtualModel }
```

The handler calls `vm = ResolveRequest(vm)` **once**, immediately after registry
lookup. For a sequence model this atomically advances the cursor
(`atomic.Uint64`) and returns a plain, stateless `MockModel` snapshot carrying
that step's content or `ErrorInjection`. For every other model `ResolveRequest`
is the identity. After this single line, **all existing dispatch machinery
works unchanged** — there is no sequence-specific code anywhere in the handler.

Consequences:

- **Concurrency-safe by construction.** The atomic cursor is the only shared
  mutable state; each request operates on its own snapshot local.
- **Global sequence semantics.** The cursor is shared across all callers, which
  is exactly right for "simulate a provider returning 200, 200, 429" — the
  schedule is a property of the upstream, not of any one client.
- **Zero blast radius.** Pre-content errors, streaming, the mid-stream gate, the
  models list, and the builtin-provider seed are all reused as-is.

`SequenceModel` still implements the protocol `Handle*` methods (delegating to a
freshly resolved snapshot) so direct, non-handler consumers — `protocoltest`,
benchmarks — get one step per call without going through `ResolveRequest`. The
two paths are mutually exclusive, so the cursor advances exactly once per
request either way.

### Engine lives in the root package

`vmodel.Sequence` (flattening + atomic cursor + status→error mapping) is
protocol-neutral and lives in `vmodel/sequence.go`. The thin `SequenceModel`
wrappers in `anthropic/` and `openai/` only adapt a resolved step into their
protocol's `MockModel`, mirroring how the always-fail specs are shared via
`defaults_shared.go`.

## Registration

`virtual-sequence-429` (`200, 200, 429`, looping) ships in **both** default
registries via `DefaultSequenceConfigs()`, registered from each protocol's
`RegisterDefaults`. It is a genuine user-facing demo: it lets onboarding/dry-run
users watch the gateway react to an intermittently rate-limited upstream
without configuring a real provider — consistent with the registration
discipline in `vmodel/README.md` (protocol-compliant, clearly named, useful for
demos). Bespoke programs are constructed directly via `NewSequenceModel`.

## Files

- `vmodel/sequence.go` — `SequenceStep`, `SequenceConfig`, `Sequence`,
  `ResolvedStep`, `defaultErrorMeta`, `DefaultSequenceConfigs`.
- `vmodel/types.go` — `VirtualModelTypeSequence`.
- `vmodel/{anthropic,openai}/sequence_model.go` — `SequenceModel`,
  `RequestResolver`, `ResolveRequest`.
- `vmodel/virtualserver/handler.go` — one `ResolveRequest` call per entrypoint.
- `vmodel/{anthropic,openai}/defaults.go` — demo registration.
