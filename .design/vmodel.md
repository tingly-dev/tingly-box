# vmodel — design index

`vmodel` ships production-grade, protocol-compliant synthetic LLM behavior: it
backs the public `/virtual/v1/*` endpoint (onboarding, demos, dry-runs without
a real upstream) and doubles as the in-process LLM substitute reused by test
packages (`internal/protocoltest`, `internal/servertest`, `cli/harness`).

This file is the **entry point** for vmodel design notes. Per-topic rationale,
trade-offs, and wiring details live in the linked documents below; the
**code-level usage guide is [`vmodel/README.md`](../vmodel/README.md)** (layout,
interfaces, registration discipline, how to add a model).

## Design documents

| Document | What it covers |
| -------- | -------------- |
| [`vmodel-sequence.md`](./vmodel-sequence.md) | The `sequence` virtual model — a configurable program of per-request outcomes (e.g. `200, 200, 429`) that simulates a flaky upstream. Covers the per-request resolver pattern and why the cursor is a shared atomic. |
| [`vmodel-benchmark.md`](./vmodel-benchmark.md) | Elevating `vmodel` into the single shared real-world test-bench for `*test` packages, plus the reusable preset check-logic layer. |

## Related design notes

These live outside the `vmodel-*` namespace but intersect with it:

- [`test-infrastructure.md`](./test-infrastructure.md) — how the test packages consume vmodel primitives.
- [`stream-usage-tracking.md`](./stream-usage-tracking.md) — usage emission exercised by the stream-test mocks.

> Adding a new vmodel design doc? Name it `vmodel-<topic>.md`, drop a one-line
> summary in the table above, and link back here from any code that references it.
