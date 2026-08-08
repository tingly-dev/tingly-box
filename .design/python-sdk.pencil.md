# Python SDK (`tingly`) — Pencil Graph

Visual companion to `python-sdk.md`. Two pictures. For exact endpoints /
field names / file:line references, that doc is the source of truth — this
page is just the shape of things.

Contents:

- The one idea
- A request, start to finish

## The one idea

```
   client                     tingly-box                    real upstream
  ┌────────┐   model=x   ┌──────────────────────┐
  │ any app │────────────►│ rule x → PLUGIN CODE  │
  └────────┘             │            │           │
                         │            │ calls back:│
                         │  rule y ◄──┘  use(y)    │
                         └─────┬──────────────────┘
                               ▼
                         Anthropic / OpenAI / local …
```

A plugin is just a rule whose upstream happens to be your code. It can call
*back* into any other rule to get its own answer — same gateway, same guard
rails / quota / logging, both directions.

## A request, start to finish

```
 1. connect()            admin token  ──►  mint a session  ──►  model token

 2. tb.ask("...")        model token  ──►  tb picks a rule  ──►  picks a service
                                                                       │
                                                                       ▼
                                                              real model answers

 3. if that model IS a plugin:
       tb calls the plugin instead of a real model (step 2, inbound)
       the plugin's handler does step 2 AGAIN, on its own, to get ITS answer
       the plugin's answer becomes tb's answer to the original caller
```

Steps 1 and 2 are all of Layer 1 (`Client`). Step 3 is Layer 2 (`Plugin`) —
same request, plugin just sits in the middle and calls back once.
`critic_plugin.py` (ask a different model to review), `fusion_plugin.py`
(ask several, then a judge), and `rag_plugin.py` (ask one, with retrieved
context) are all step 3 with different logic in the handler — no new
mechanism.
