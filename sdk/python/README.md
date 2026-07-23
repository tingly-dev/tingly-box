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

A plugin is an upstream tb can call two ways: **Anthropic Messages
(`/v1/messages`, primary)** and **OpenAI chat completions
(`/v1/chat/completions`, secondary)** — both real, both always served; which
one tb actually uses is a registration choice (`api_style`, `"anthropic"` by
default). Write one handler, serve it, register it — then any tb client can
select it as a model, regardless of which protocol *that* client speaks.

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
tingly plugin run my_rag_plugin.py        # serve AND register with tb
```

`serve()` (and `tingly plugin run`) registers the plugin with tb once at
startup — an idempotent upsert-by-name, so restarting the plugin updates the
same provider instead of duplicating it. There is no heartbeat or lease:
liveness is handled by tb's existing per-service circuit breaker, the same
mechanism that protects every other provider. If the plugin goes down, the
next failed request trips the breaker and traffic tier-fails-over (when a
fallback tier is configured). Retiring a plugin is the same as retiring any
other provider — delete it in the tb UI.

The server is stdlib-only (no FastAPI), supports streaming on both routes, and
`plugin.llm` calls back into tb (Anthropic-first) so the plugin reuses the
gateway for its own LLM work.

### Example plugins

`sdk/python/examples/` has four, each demonstrating a different real-world
pattern for the same idea — a plugin composing the box by calling back into
other tb rules:

- **`rag_plugin.py`** — retrieval-augmented answers from a toy corpus, one
  call back into tb for generation.
- **`critic_plugin.py`** — cross-model critique (`model="plugin/critic"`):
  forwards the thing to review to a *different* rule/model and returns a
  structured verdict. Self-critique is unreliable (a model can't reliably
  catch its own mistakes); this is the pattern behind
  [Zen MCP](https://github.com/jray2123/zen-mcp-server) and
  [Consult7](https://github.com/szeider/consult7), and behind aider's
  architect/editor split.
- **`fusion_plugin.py`** — multi-model consensus (`model="plugin/fusion"`):
  polls a panel of rules/models concurrently, skips the judge call when they
  already agree, otherwise a judge call synthesizes. Mirrors Consult7's 2026
  Fusion feature; the clearest illustration that a plugin can freely
  originate more than one call, against more than one rule, per request.
- **`router_plugin.py`** — quota-aware dispatch (`model="plugin/router"`): a
  different shape from the three above — it generates nothing itself, it
  only *decides* which one candidate to forward to, using quota headroom
  (`tb.quota`) to pick. Same idea as LiteLLM Router's `usage-based-routing`
  strategy. Forwards with `tb.ask(..., pin_provider=<uuid>)` so the provider
  it checked quota for is *guaranteed* to be the one that serves the
  request — see "Deterministic dispatch" below for why that matters.

## Deterministic dispatch (`pin_provider`)

Normally `tb.ask(model=X)` resolves `(scenario, model)` to a rule and lets tb
itself pick which of that rule's services actually runs (affinity / smart
routing / load balancing — unchanged, still the default). When code needs to
*guarantee* a specific provider — like `router_plugin.py` above, which
already checked that provider's quota — pass `pin_provider`:

```python
tb.ask("...", model="sonnet1", pin_provider=provider_uuid)
```

tb only allows pinning to a provider that's already one of the resolved
rule's own configured services (`tb.rules.for_model(scenario, model)` lists
them) — it rejects a pin to anything else. See `.design/python-sdk.md` §"Two
connection modes" for the full mechanics.

## Status

- **Layer 1** (consume tb): `connect()` → `Client`. Done — Anthropic tried
  first when a scenario supports both transports.
- **Layer 2** (be an AI server): `tingly.Plugin` + manifest + `register`. Done —
  dual-protocol server (Anthropic primary, OpenAI secondary); tb-side
  supervisor/lifecycle UI still pending (not required to use plugins today).
- **Layer 3** (tb routes to the plugin as a model): via provider-as-upstream.
  Done, verified end-to-end including a plugin forwarding to another tb rule
  and returning the result (`sdk/python/examples/e2e_run.sh`).

See `.design/python-sdk.md` in the repo for the full design and diagrams.
