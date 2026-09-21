# Decision protocol

## Context

Jev is not a chat-completion model. Its native `POST /api/v1/decisions`
surface accepts a model identifier, shared state, and a map of typed questions
(`choice`, `score`, or `noul`) and returns structured answers and calibrated
probabilities. Treating that payload as OpenAI chat would lose its schema and
encourage callers to parse generated prose.

## Contract

Tingly Box exposes the native protocol at:

```text
POST /tingly/decision/v1/decisions
```

The route uses the existing model-token authentication and routing-rule
pipeline. The request `model` selects a `decision` scenario rule; the selected
service model replaces it only on the upstream request. The response is passed
through unchanged. Providers used by this route must declare
`api_style: decision`; their `api_base` may end in `/api/v1` or
`/api/v1/decisions`. The provider token is sent as a Bearer credential.

Only the native endpoint is included in this first slice. Jev's workflow
preset endpoints and MCP tools are application-level conveniences built on top
of the native protocol and can be added independently without changing this
wire contract.

## Safety and compatibility

* Request bodies are size-limited and validated before routing.
* `state`, instructions, criteria, and answer bodies stay opaque JSON so new
  Jev fields remain forward-compatible.
* Decision providers are not accepted by chat, embeddings, or image handlers.
* Upstream status, content type, and response body are preserved; hop-by-hop
  headers are not forwarded.
* Decision results are advisory. This endpoint does not grant permissions or
  bypass existing approval requirements.
