# Python SDK (`tingly`) — Pencil Graph

Visual companion to `python-sdk.md`. Four pictures, each answering one
question. For exact endpoints / field names / file:line references, that
doc is the source of truth — this page is just the shape of things.

Contents:

- The one idea
- A request, start to finish
- Two ways to pick a provider
- `router_plugin.py` in one picture

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

## Two ways to pick a provider

A model can have more than one provider behind it (tiers, fallback). Normally
tb picks. `pin_provider=` lets the caller pick instead — but only from what
that rule already offers.

```
  default                              pin_provider=B
  ───────                              ──────────────
  tb.ask(model="x")                    tb.ask(model="x", pin_provider=B)
       │                                    │
       ▼                                    ▼
  rule "x" → services [A, B]           rule "x" → services [A, B]
       │                                    │
       ▼                                    ▼
  tb picks ONE                         must B be in [A, B]?
  (tiers / affinity / load)             yes → use B, guaranteed
                                         no  → 400, rejected
```

Why this exists: a plugin that checks quota for provider A and then calls
`.ask(model="x")` is only *guessing* A will be used — tb might pick B
instead. `pin_provider=` turns the guess into a guarantee.

Only two modes — no "skip the rule entirely" third one. No rule for a
provider yet? Create a one-service rule for it (cheap, one-time), don't
bypass rule resolution to reach it — same reason `X-Tingly-Probe-Service`
(tb's internal, unauthenticated bypass) never got exposed to the SDK.

## `router_plugin.py` in one picture

```
  question in
       │
       ▼
  for each candidate model:
    keep it only if it maps to exactly ONE provider
    (a model with several providers can't be quota-picked — tb decides that one)
       │
       ▼
  check quota for each provider kept
       │
       ▼
  pick the one with the most headroom
       │
       ▼
  tb.ask(model=picked, pin_provider=picked's provider)   ← the guarantee, above
       │
       ▼
  answer out
```

Everything else — `critic_plugin.py` (ask a different model to review),
`fusion_plugin.py` (ask several, then a judge), `rag_plugin.py` (ask one,
with retrieved context) — is the same "call back into tb" from *A request,
start to finish* above, just with different logic in the handler.
