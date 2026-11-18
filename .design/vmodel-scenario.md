# VModel fixed address and independent CLI instances

Status: implemented in PR #2621, on top of #2618/#2624. The canonical file
contract is owned by [vmodel-definitions.md](./vmodel-definitions.md).

## Decision

Use one listener address per VModel instance and select models by request
`model` ID. Many clients can share the same address and catalog. Starting or
stopping a CLI process never creates providers or routing rules in tb.

| Entry point | Catalog | Lifetime |
| --- | --- | --- |
| tb `/tingly/vmodel` | Existing builtin VModels | tb process |
| `tb vmodel --config models.yaml` | Definitions in the supplied files | Independent CLI process |

The former automatic-registration approach has been removed, including its
management client, collision preflight, rollback and shutdown cleanup. There
are no temporary persisted resources, leases or cleanup obligations. Explicit
user-managed providers/rules elsewhere in tb keep their ordinary behavior.

## Fixed tb endpoint

`/tingly/vmodel/v1/models` lists the OpenAI builtin catalog. Chat and Responses
use `/tingly/vmodel/v1/chat/completions` and `/tingly/vmodel/v1/responses`;
Anthropic uses `/tingly/vmodel/v1/messages`. Requests name a model ID directly.
These routes reuse the builtin virtualserver handlers and tb model-token auth,
without needing enabled providers or model routing rules.

This is a direct model endpoint like the existing `/virtual/openai` and
`/virtual/anthropic` routes, which remain available. It does not perform gateway
provider selection, rule flags, plugins or gateway usage accounting. Server
HTTP access logging still applies. For full gateway behavior, users can
explicitly configure a virtual provider and rules through the usual features.
The `vmodel` scene does not allow rule binding.

## CLI address and model IDs

`tb vmodel --config models.yaml --json` reports the root URL, protocol base
URLs and model IDs after the listener starts. OpenAI uses `<url>/v1`;
Anthropic uses `<url>` (its SDK appends `/v1/messages`). Both reach the same
listener and select from the same definitions by ID. Protocol capabilities
remain definition-specific. Existing protocol-prefixed listener aliases remain
available, but the readiness output uses the common root.

Repeated `--config` arguments load multiple definitions into one instance;
IDs must be unique there. Multiple clients share an instance. Independent
instances bind different ports (`0` chooses a free port; `--port` fixes one),
and can reuse IDs without conflict. No running tb gateway is required.
`--standalone` is unnecessary and has been removed. Export remains unchanged.

Custom CLI definitions are not automatically published at the fixed tb
endpoint. Clients use that instance's printed URL and IDs. Closing the process
closes its listener and leaves no gateway configuration behind.

## Navigation

The scene page lives at `/agent/vmodel` and shows the fixed URL and model API
credentials. It does not offer rule editing, plugins, gateway request status
or gateway logs for these direct calls. Credentials remains at
`/credentials/virtual-models`; the Power-up still links there. The same
visibility key shows/hides both entries, without starting/stopping services.

## Verification

Direct-route tests cover authentication, model listing and ID selection,
Chat/Responses/Anthropic calls, SSE and unchanged provider/rule inventories.
CLI lifecycle tests cover isolated catalogs, readiness and shutdown. Navigation
visibility tests and canonical-definition tests remain in their existing suites.
