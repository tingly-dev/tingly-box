# tingly (Python SDK) — v1 framework

Two classes, zero dependencies. See [`.design/python-sdk.md`](../../.design/python-sdk.md)
for the design rationale and scope cuts.

- `tingly.Server` — be a tb provider. Register a handler, run an
  OpenAI-compatible HTTP server, plug it into tb like Ollama.
- `tingly.Client` — call tb from Python. Point it at a running tb and a
  gateway token, ask it to run any scenario/model.

## Quick start

```python
from tingly import Server

srv = Server("relay", tb_base_url="http://localhost:12580", tb_token="...")

@srv.chat
def handle(req):
    # req.model, req.messages, req.raw are available; here we just relay
    # everything to a different tb model and hand the answer straight back.
    return srv.tb.chat(model="claude-opus-4-8", messages=req.raw["messages"])

srv.run(port=8765)
```

Register it with tb: **Connect AI -> Self-hosted -> Custom endpoint**,
base URL `http://localhost:8765/v1`, no key required — the same flow used
for Ollama or vLLM. From then on tb calls it like any other provider.

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
