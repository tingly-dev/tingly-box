# tingly (Python SDK) — v1 framework

Two classes, zero dependencies. See [`.design/python-sdk.md`](../../.design/python-sdk.md)
for the design rationale and scope cuts.

- `tingly.Server` — be a tb provider. Register one handler, run a
  dual-protocol HTTP server (OpenAI `/v1/chat/completions` **and**
  Anthropic `/v1/messages`, both funneling into the same handler), plug it
  into tb like Ollama.
- `tingly.Client` — call tb from Python. Point it at a running tb and a
  gateway token, ask it to run any scenario/model.

## Quick start

```python
from tingly import Server

srv = Server("relay", tb_base_url="http://localhost:12580", tb_token="...")

@srv.chat
def handle(req):
    # req.model, req.messages, req.raw are available; here we just relay
    # everything to a different tb model and hand the answer straight back —
    # regardless of whether the caller hit us over OpenAI or Anthropic wire.
    return srv.tb.chat(model="claude-opus-4-8", messages=req.as_openai_messages())

srv.run(port=8765)
```

Register it with tb as a **dual** provider (same as a service that natively
speaks both protocols, e.g. Vertex): **Connect AI -> Self-hosted -> Dual
endpoint**, OpenAI URL `http://localhost:8765/v1`, Anthropic URL
`http://localhost:8765` (no `/v1` — the Anthropic SDK strips it if present
anyway), no key required. From then on tb calls it like any other provider,
picking whichever URL matches the inbound client's own protocol.

`/v1/messages` requests are always handled as if beta — there is no
`?beta=true` / `anthropic-version` branching, matching the simplification
tb's own vmodel virtual server already makes at its HTTP boundary.

> **No-key + Anthropic-style provider:** the vendored `anthropic-sdk-go`
> treats a genuinely empty API key as "go discover ambient credentials" and
> errors before ever sending the request — unlike the OpenAI client, which
> sends the empty header as-is. Self-hosted templates let you check "no API
> key" and still type a value in the token field (it stays editable), so
> give it any placeholder (e.g. `not-required`) rather than leaving it
> blank; the server itself never checks incoming auth.

Run the bundled examples directly:

```bash
cd sdk/python
TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=<gateway token> python examples/relay.py
TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=<gateway token> python examples/fanout.py
```

## Using `Client` standalone

```python
from tingly import Client, text_of

tb = Client(base_url="http://localhost:12580", token="...")
resp = tb.chat(model="gpt-4o", messages=[{"role": "user", "content": "hi"}])
print(text_of(resp))
```

## Tests

```bash
cd sdk/python
python -m unittest discover tests
```
