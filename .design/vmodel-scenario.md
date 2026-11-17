# VModel scenario and gateway registration

Status: implemented in PR #2621, rebased onto main after #2618 and #2624.
The shared definition and standalone runtime contract is owned by
[`vmodel-definitions.md`](./vmodel-definitions.md); this document owns the
additional gateway lifecycle and UI behavior. See [`vmodel.md`](./vmodel.md)
for the full design index.

## Decision and implementation boundary

Expose file-defined VModels through tb's existing scenario, provider and rule
mechanisms. The process still runs the shared VModel HTTP runtime, but also
registers temporary gateway resources so requests exercise normal protocol
conversion, rule flags, routing, logging and usage handling.

| Behavior | #2618 | #2621 |
| --- | --- | --- |
| `tb vmodel --config model.yaml` | Standalone provider | Provider plus registration in the running tb instance |
| Direct-only mode | Default behavior | Explicit `--standalone` |
| Format, loader, export and harness adapters | Implemented | Reused unchanged |
| Scenario, frontend and management API integration | Absent | Implemented |

The CLI examples in the base definition design describe #2618. With this
integration applied, start tb first or add `--standalone`. Definitions use
the strict id/kind contract from #2624; no legacy parsing is reintroduced.
Export still exits without gateway registration. No Python SDK or separate `serve` command is
introduced.

## Scenario and UI

The internal scenario ID is `vmodel`; the gateway prefix is `/tingly/vmodel`.
It accepts OpenAI and Anthropic client protocols through the normal scenario
descriptor and conversion pipeline. Model names remain definition IDs, which
become request-model names in the generated rules. This differs from the
existing `/virtual/*` built-in model endpoints and does not replace them.

The frontend route is `/agent/vmodel`, using the common `AgentPage` with
connection credentials, model routing and a `tb vmodel --config model.yaml`
startup hint. The existing Virtual Models power-up controls both the scenario
navigation entry and the virtual-model Credentials entry through the same
`vmodel` visibility key. It is hidden by default; existing visibility choices
are preserved without a defaults-version bump. This is a UI visibility switch,
not a service stop or authorization control: hiding it does not remove
providers, rules or running VModel processes.

## Registration lifecycle

```text
tb vmodel
  → find the running tb instance for the selected config directory
  → load definitions and start the standalone HTTP listener
  → check existing vmodel request-model rules for collisions
  → read the listener's protocol-specific model lists
  → create temporary providers, then active per-model rules
  → emit readiness, including gateway_base_url and scenario
  → serve until shutdown
  → delete owned rules, then owned providers
```

Discovery uses the config-directory lock and runtime port. Management requests
use the user token read from that configuration and authenticated HTTP APIs;
the subprocess never opens the gateway SQLite database. Model clients use the
normal model credentials shown in the scenario page, not the management token.
Readiness is emitted after registration succeeds. Attached JSON readiness adds
`gateway_base_url: http://127.0.0.1:<tb-port>/tingly/vmodel` and
`scenario: vmodel` to the direct-listener fields.

Resources are ordinary providers and rules created through the existing APIs:
`/api/v2/providers` and `/api/v1/rule`. Names/descriptions contain a unique
process-instance identifier; cleanup retains the exact returned UUIDs. Each
rule has one selected provider service and preserves the model ID. Existing
rules with the same scenario/request-model cause startup to fail instead of
being replaced. The preflight check is not an atomic reservation against
concurrent CLI starts.

## Protocol selection

Registration reads model capabilities from the running listener instead of
assuming all definitions support both Chat and Responses. Shared models prefer
OpenAI, then Anthropic, then Google as available. OpenAI providers are grouped
by `openai_endpoint_mode` (`both`, `chat`, `responses`); this field is carried
through the provider create/update API and OpenAPI schema. Responses-only
recordings therefore stay on their supported upstream endpoint. Anthropic
and Google providers use their respective direct base URLs. A model with
multiple capabilities receives one gateway rule, not one rule per protocol.

Native-only decision definitions stay accessible through the direct runtime;
they are not exposed as chat gateway rules. Loading a definition does not
grant additional protocol capabilities or turn captured wire data into
another format.

## Failure and cleanup semantics

If registration fails partway, delete resources already created in reverse
order (rules before providers), and fail startup. On normal interrupt/SIGTERM,
the same cleanup runs with its own bounded context, so canceled serving
contexts do not prevent deletion. Cleanup errors are returned rather than
hidden; deleting an already-absent resource is tolerated. The standalone HTTP
listener is closed with the process lifecycle as well.

This is temporary-resource cleanup, not a lease/reconciliation service. A
forced kill or unavailable gateway can leave resources behind; those require
manual removal. The CLI does not overwrite user rules, auto-enable the UI
power-up, or persist definitions into a new gateway model store.

## Verification

- Scenario registry and frontend tests cover protocol families, navigation and
  the shared visibility toggle.
- CLI/gateway tests cover registration, collisions, partial rollback, cleanup,
  protocol-specific provider selection and readiness.
- A real tb process plus a VModel process verifies default injection, Chat and
  Anthropic requests, SSE, duplicate-ID rejection and resource removal after
  shutdown.
- `--standalone` retains the direct provider path and the same file/harness
  contract from #2618.

Implementation: `internal/command/vmodel_gateway.go`,
`internal/command/vmodel.go`, `internal/typ/scenario_registry.go`, provider
management API types/handlers, and the frontend scenario registry/AgentPage.
