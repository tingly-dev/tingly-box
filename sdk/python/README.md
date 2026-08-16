# tingly — Python SDK for tingly-box

Two independent halves:

- **Consume the box.** `tingly.connect()` gives you OpenAI/Anthropic clients
  already pointed at your gateway, so every call inherits provider routing,
  fallback, guard rails, quota and logging.
- **Be a provider.** `tingly.Server` is a protocol-compliant model server you
  write in Python. tingly-box consumes it exactly the way it consumes Ollama or
  vLLM — a self-hosted provider with a base URL and no key.

Either half stands alone. Using both is the interesting case: a provider whose
handler calls back into the box can orchestrate every other rule you have.

## Install

```bash
pip install tingly
```

Requires Python 3.10+. Ships with the `openai` and `anthropic` SDKs so
`tb.openai` / `tb.anthropic` give you full fine-grained control out of the box.

### From a checkout

The typed models are generated from tingly-box's `openapi.json` and are not
committed, so a source checkout needs one step first:

```bash
task gen:py          # from the repository root
cd sdk/python && pip install -e '.[dev]' && pytest
```

Re-run `task gen:py` after any backend API change (`task codegen` does it as
its last step). A wheel from PyPI already contains the models — CI generates
them before building it.

## Consume the box

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

### How `connect()` finds your box

In order: explicit args → `TINGLY_BOX_URL` / `TINGLY_BOX_TOKEN` env →
`~/.tingly-box/sdk.json` → `~/.tingly-box/config.json` + localhost probe.

The token is your **admin** token (tb's `UserToken`); the SDK uses it once to
mint a session, then uses the returned model token for the LLM calls.

### Diagnose

```bash
tingly doctor            # traverses the real path and prints what works
tingly doctor --link     # save gateway URL + token to ~/.tingly-box/sdk.json
```

A green `tingly doctor` is a guarantee your code will run.

### Anything else the gateway can do

The two views above are the common cases. Everything else in tingly-box's
control plane — ~195 operations — is reachable with the same typed models,
which are generated from the gateway's own `openapi.json`:

```python
from tingly._generated.models import ProvidersResponse, UsageStatsResponse

tb.api.get("/api/v2/providers", ProvidersResponse)
tb.usage.raw(group_by="rule", sort_by="total_tokens", limit=5)  # -> UsageStatsResponse
```

A response that doesn't match its model raises `SchemaMismatchError` naming the
fix, rather than quietly returning a default — the SDK never guesses at a field
name.

## Be a provider

```python
from tingly import Server

srv = Server(name="router", scenario="openai")   # serves model id: router

@srv.chat
def handle(req):
    question = req.last_user_text()
    # Route each request to a different rule in the openai scenario.
    target = "claude-opus-4-6" if len(question) > 4000 else "claude-haiku-4-5"
    return srv.use("openai").ask(question, model=target)

if __name__ == "__main__":
    srv.run()                                    # http://127.0.0.1:8765
```

```bash
python my_server.py
```

That's the whole authoring surface. The server answers **both**
`POST /v1/messages` (Anthropic) and `POST /v1/chat/completions` (OpenAI), plus
`GET /v1/models` and `GET /health`; which one tb calls is decided by the
provider's API style on the tb side, so there is nothing to configure here. It
is stdlib-only (no FastAPI) and streams real SSE on both routes.

### Wiring it into tingly-box

There is no registration protocol, no manifest, and no plugin lifecycle. You
add it the same way you'd add Ollama:

> tingly-box → **Connect AI** → **Self-hosted** → **Python Server (tingly)**

The base URL is pre-filled for the default port; the startup banner prints the
exact values your process is actually using. Then bind a rule to the model
name, and it's selectable from Claude Code, Cursor, the tb UI, or another
`tingly.connect()` experiment.

Everything else follows from it being an ordinary provider:

- Put it in tier 0 with a real model in tier 1 and tb fails over automatically.
- When the process dies, the same per-service circuit breaker that protects
  every provider trips on the next failed request.
- Retiring it is deleting the provider in the UI.

### `srv.tb` — calling back into the box

`srv.tb` is a lazily-connected `Client` bound to the server's scenario;
`srv.use("<scenario>")` targets a different rule-set. This is what makes a
Python provider more than a static model: it never hard-codes an upstream or a
key, and it can originate as many calls as it likes, against as many rules as
you've configured, before answering once.

`srv.tb.rules(scenario)` lists what those rules actually are, so a provider can
dispatch to what this box has rather than to model names you hoped were there:

```python
for r in srv.tb.rules("openai"):
    print(r.request_model, r.active, [s.model for s in r.services])
```

`srv.tb.quota` answers the other half of "where should this go" — how much
headroom each upstream account has left:

```python
for usage, left in srv.tb.quota.usable("codex", min_headroom=5):
    print(usage.provider_name, f"{left:.0f}% left")
```

Every provider reports quota in its own currency — percentages, request counts,
credits, dollars — so the view reduces each to its *tightest countable window*,
the one that runs out first, mirroring what tb does internally. An account
whose usage is unknown reports `None` rather than 0%, because "no data" is not
"nothing used" and would otherwise win every comparison.

### Examples — one model id in, many rules out

Every example in `sdk/python/examples/` is the same shape: a provider that
connects back to tb and **dispatches each request to a different rule**. They
differ only in the dispatch policy. Targets are rules in the `openai` scenario,
because that is where a stock install already has real models bound — and each
discovers what exists via `srv.tb.rules("openai")` rather than hard-coding
model names that may not be configured on your box.

- **`router_server.py`** — dispatch by classification: short prompts to a cheap
  rule, long ones to a strong rule, code to a coding rule. The baseline shape,
  and the one to copy.
- **`critic_server.py`** — dispatch *away from the caller*: forwards the thing
  to review to a different rule and returns a structured verdict. Self-critique
  is unreliable (Huang et al., ICLR 2024: LLMs can't reliably self-correct
  without external feedback), so routing to a different model is the point, not
  a detail. The pattern behind
  [Zen MCP](https://github.com/jray2123/zen-mcp-server),
  [Consult7](https://github.com/szeider/consult7), and aider's architect/editor
  split.
- **`fusion_server.py`** — dispatch to *many* rules at once: polls a panel
  concurrently, skips the judge call when they already agree, otherwise
  synthesizes. Mirrors Consult7's 2026 Fusion feature. The panel members are
  discovered and de-duplicated, since a panel of clones is latency without a
  second opinion.
- **`quota_router_server.py`** — dispatch by **remaining quota**: with several
  Codex accounts connected, it reads each one's headroom, skips the nearly
  spent, and sends the request to whichever has the most room left.

Every hop is a real tb rule, so guard rails, quota, logging and tier-failover
apply to the inbound request *and* to each call the provider originates.

`rag_experiment.py` is the other half — a plain script that only consumes the
box, serving nothing.

## Two implementations of one concept

tingly-box has no separate notion of "plugin". A thing that answers LLM
protocol is a **provider**, and there are two ways to be one:

| | `AuthType=vmodel` (in-process) | `tingly.Server` (out-of-process) |
|---|---|---|
| code | compiled into tb, Go, `vmodel/` | your own process, Python |
| tb sees | a provider | a provider |
| added by | seeded builtin | Connect AI → Self-hosted |
| can call back into tb | no (same process) | **yes** (`srv.tb`) — the reason it exists |

See `.design/python-sdk.md` for the full design.
