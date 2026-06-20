# Design TODO

Backlog of structural refactors agreed in principle but deliberately deferred so
each lands as its own reviewable change. Functional behaviour is already correct
without these — they are readability / cohesion improvements.

---

## Consolidate the per-transport "prologue" at the true HTTP entrypoint

**Status:** proposed (deferred). Depends on the lifted-failover change (per-attempt
re-transform + cross-style fallback) landing first.

### Why

After lifting failover so each attempt re-transforms, every transport entry point
splits into:

1. a one-time, **provider-independent prologue** (parse, validate, rule,
   vision proxy, context-1m, session, initial service selection, recorder,
   pristine template snapshot), and
2. a **provider-dependent per-attempt** body (resolve dual endpoint, pre-chain,
   guardrails, target/endpoint resolution, transform, dispatch) run inside the
   failover loop.

Today the prologue is *split across two layers*: the real HTTP entrypoint
(`HandleAnthropicMessages`, `internal/server/anthropic.go:92`) does
parse/rule/vision-proxy/initial-select, while the rest of the prologue lives one
level down in `AnthropicMessagesV1` / `AnthropicMessagesV1Beta`. That's a
half-prologue split: the initial `SelectService` sits at :92 but the failover
loop (and `selectFallbackService`) is two functions away, so the routing story
is not readable in one place.

### The durable rule (use this to decide where any new preprocessing goes)

> Does the step depend on the *chosen provider/model*?
> - **No** → it belongs in the one-time prologue at the transport entrypoint.
> - **Yes** → it belongs in the per-attempt body inside the failover loop.

Corollary on ordering: `applyVisionProxy` is pinned *before* `SelectService`
because smart routing reads (possibly vision-rewritten) message content — a real
data dependency, not a stylistic choice. `detectAndApplyContext1MFromIncomingRequest`
only mutates the rule and has no ordering constraint, so it can sit anywhere in
the prologue.

### Target shape (Anthropic path)

```
HandleAnthropicMessages (anthropic.go:92):
  parse(beta?) -> validate -> determineRule
  applyVisionProxy(reqParams)          // data dependency: before initial select
  detectContext1m(rule)
  p0, svc0 = SelectService(reqParams)
  session / SetTracking / recorder / template            // full prologue here
  attempt := beta ? betaClosure(betaReq) : v1Closure(v1Req)   // decided ONCE
  dispatchWithPriorityFailover(c, rule, p0, svc0.Model, attempt)
```

`v1Closure` / `betaClosure` are the existing `runAnthropicV1Attempt` /
`runAnthropicBetaAttempt`; the move just hoists the remaining prologue from those
functions up to the entrypoint and bakes the beta/v1 choice into the closure.

### Guardrails (what NOT to do)

- **Do not** inline the failover loop into the HTTP handler.
  `dispatchWithPriorityFailover` is the transport-agnostic shared seam (exercised
  by `lbsim`); keep calling it with a transport-specific closure rather than
  duplicating the orchestration in each handler.
- **Do not** merge the beta/v1 request types to "simplify" the lift. `?beta` is a
  request-level, one-time decision — branch it once outside the loop, not per
  attempt. The two typed paths (different prechain / guardrails adapter /
  transform) stay separate.

### Scope / sequencing

1. Land the lifted-failover change on its own.
2. Do this prologue consolidation as a *separate, pure-structure* PR — start with
   the Anthropic path as the template, confirm the shape, then apply the same
   pattern to the OpenAI Chat (`OpenAIChatCompletion`) and OpenAI Responses
   (`ResponsesCreate`) entrypoints for symmetry.

### Files

- `internal/server/anthropic.go` (`HandleAnthropicMessages`)
- `internal/server/anthropic_message_v1.go`, `internal/server/anthropic_message_beta.go`
- `internal/server/openai_chat.go`, `internal/server/openai_responses.go`
- `internal/server/failover_dispatch.go` (shared loop — unchanged seam)
