# Python SDK (`tingly`) — Pencil Graph

Visual companion to `python-sdk.md`. Two pictures. For exact endpoints /
field names / file references, that doc is the source of truth — this page is
just the shape of things.

Contents:

- The one idea
- A request, start to finish

## The one idea

```
   client                       tingly-box                     real upstreams
  ┌────────┐  model=rag  ┌───────────────────────────┐
  │ any app │────────────►│ provider: rag             │
  └────────┘             │  (self-hosted, no key,    │
                         │   base_url = :8765)       │
                         │        │                  │
                         │        ▼  ordinary HTTP   │
                         │   YOUR PYTHON PROCESS     │
                         │        │                  │
                         │        │ calls back:      │
                         │  rule y ◄──┘ srv.use(y)   │──►  Anthropic / OpenAI /
                         └───────────────────────────┘     Ollama / vmodel …
```

There is no "plugin". Your Python process is a **provider** — the same kind of
thing as Ollama, and the same kind of thing as an in-process `vmodel`. What it
can do that Ollama can't is call *back* into any other rule to compose its own
answer: same gateway, same guard rails / quota / logging, both directions.

Nothing registers itself. Someone added a provider pointing at its base URL,
in the UI, once.

## A request, start to finish

```
 1. connect()            admin token  ──►  mint a session  ──►  model token

 2. tb.ask("...")        model token  ──►  tb picks a rule  ──►  picks a service
                                                                       │
                                                                       ▼
                                                              that service answers

 3. if that service IS a Python provider:
       tb calls it over HTTP like any other upstream (step 2, inbound)
       its handler does step 2 AGAIN, on its own, to get ITS answer
       its answer becomes tb's answer to the original caller
```

Steps 1–2 are `connect()` / `Client` — the consume half. Step 3 is `Server` —
the provide half. They meet only at the arrow, and either works without the
other.

`router_server.py` (classify, then pick a rule), `critic_server.py` (always a
*different* rule than the caller's), and `fusion_server.py` (several rules at
once, then a judge) are all step 3 with different dispatch policies in the
handler — no new mechanism, and nothing tb has to know about.

The interesting consequence: one model id in your editor can fan out across
every rule you have configured, and each hop is a real tb rule, so guard rails,
quota, logging and failover apply on all of them.
