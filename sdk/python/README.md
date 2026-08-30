# tingly (Python SDK) — v1 framework

Two classes, zero dependencies. See [`.design/python-sdk.md`](../../.design/python-sdk.md)
for the design rationale and scope cuts.

- `tingly.Server` — be a tb provider. Register a handler per protocol you
  want to serve — `@srv.chat` for OpenAI (`/v1/chat/completions`),
  `@srv.messages` for Anthropic (`/v1/messages`) — and plug it into tb like
  Ollama. **The two are independent; nothing bridges them, and neither gets
  a typed wrapper.** Both get exactly the raw parsed request body the caller
  sent — the real OpenAI chat-completion request for `@srv.chat`, the real
  Anthropic messages request (content blocks, `system`, tool defs and all)
  for `@srv.messages`. This is a prototype: it hands you each protocol as it
  actually is on the wire, full stop — no in-between shape, no fields
  picked out on your behalf.
- `tingly.Client` — call tb from Python. Point it at a running tb and a
  gateway token, ask it to run any scenario/model.

## Quick start

```python
from tingly import Server, text_of

srv = Server("relay", tb_base_url="http://localhost:12580", tb_token="...")

@srv.chat
def handle_chat(body):
    # body is the raw OpenAI chat-completion request; here we just relay
    # everything to a different tb model and hand the answer straight back.
    return srv.tb.chat(model="claude-opus-4-8", messages=body["messages"])

@srv.messages
def handle_messages(body):
    # body is the raw Anthropic request, untouched. .tb still only speaks
    # OpenAI, so this handler is responsible for whatever translation it
    # needs — plain-string content forwards fine as-is; content blocks or
    # tool defs would need explicit handling here.
    return text_of(srv.tb.chat(model="claude-opus-4-8", messages=body["messages"]))

srv.run(port=8765)
```

Register it with tb as a **dual** provider (same as a service that natively
speaks both protocols, e.g. Vertex): **Connect AI -> Self-hosted -> Dual
endpoint**, OpenAI URL `http://localhost:8765/v1`, Anthropic URL
`http://localhost:8765`, no key required — either URL works with or without
a trailing `/v1`, since `Server` answers both (see below). From then on tb
calls it like any other provider, picking whichever URL matches the inbound
client's own protocol. Register just one decorator if you only want to
serve one protocol — the other endpoint then answers 404.

`/v1/messages` requests are always handled as if beta — there is no
`?beta=true` / `anthropic-version` branching, matching the simplification
tb's own vmodel virtual server already makes at its HTTP boundary. The one
deliberate bit of path leniency: both endpoints also answer without the
`/v1` prefix (`/chat/completions`, `/messages`), since which shape a
caller's configured base URL expects isn't worth troubleshooting by hand.

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
