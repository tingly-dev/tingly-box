# tingly — Python SDK for tingly-box

Write an LLM experiment or plugin in a handful of lines and reuse the
tingly-box gateway's power: provider routing, fallback, guard rails, quota and
logging. You write the idea; the box handles the plumbing.

## Install

```bash
pip install tingly
```

Ships with the `openai` and `anthropic` SDKs so `tb.openai` / `tb.anthropic`
give you full fine-grained control out of the box.

## Experiment ASAP

```python
import tingly

tb = tingly.connect(scenario="experiment")   # auto-discovers your local tb

# One-shot, transport picked for you, model routed by tb:
print(tb.ask("Summarize tingly-box in one line", model="auto"))

# Or use the SDK objects directly — already pointed at the gateway:
resp = tb.openai.chat.completions.create(
    model="auto",
    messages=[{"role": "user", "content": "hi"}],
)
resp = tb.anthropic.messages.create(
    model="claude-sonnet-4-6",
    max_tokens=256,
    messages=[{"role": "user", "content": "hi"}],
)
```

Every call above flows through tingly-box, so guard rails, quota, logging and
fallback apply automatically — your experiment never has to know.

## How `connect()` finds your box

In order: explicit args → `TINGLY_BOX_URL` / `TINGLY_BOX_TOKEN` env →
`~/.tingly-box/sdk.json` → `~/.tingly-box/config.json` + localhost probe.

The token is your **admin** token (tb's `UserToken`); the SDK uses it once to
mint a session, then uses the returned model token for the LLM calls.

## Diagnose

```bash
tingly doctor            # traverses the real path and prints what works
tingly doctor --link     # save gateway URL + token to ~/.tingly-box/sdk.json
```

A green `tingly doctor` is a guarantee your code will run.

## Write a plugin (an AI server tb can route to)

A plugin is an OpenAI-compatible upstream. Write one handler, serve it, register
it — then any tb client can select it as a model.

```python
from tingly import Plugin

plugin = Plugin(name="my-rag")          # model id: plugin/my-rag

@plugin.chat
def handle(req):
    docs = retrieve(req.last_user_text())
    return plugin.llm.ask(f"Using {docs}, answer: {req.last_user_text()}")

if __name__ == "__main__":
    plugin.serve()                      # http://127.0.0.1:8765/v1
```

```bash
tingly plugin init my-rag                 # scaffold module + tingly.toml
tingly plugin run my_rag_plugin.py        # serve
tingly plugin register my-rag \           # one step: provider + rule
   --url http://127.0.0.1:8765/v1 --model-id plugin/my-rag --scenario experiment
```

The server is stdlib-only (no FastAPI), supports streaming, and `plugin.llm`
calls back into tb so the plugin reuses the gateway for its own LLM work.

## Status

- **Layer 1** (consume tb): `connect()` → `Client`. Done.
- **Layer 2** (be an AI server): `tingly.Plugin` + manifest + `register`. Python
  side done; tb-side supervisor/lifecycle UI pending.
- **Layer 3** (tb routes to the plugin as a model): via provider-as-upstream.

See `.design/python-sdk.md` in the repo for the full design and diagrams.
