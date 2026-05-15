# smart_routing

Rule-based request router that picks a service for an LLM request by matching
operations against an extracted request context. Used by the gateway's
SmartRoutingStage to choose between multiple upstream services for a single
scenario.

## Design

Two-pass evaluation, single source of truth:

1. **Extract** request context once via `ExtractContext(req)` — funnels
   OpenAI / Anthropic v1 / Anthropic Beta requests through
   `internal/protocol/request/` converters into Anthropic Beta and runs one
   canonical extractor against it.
2. **Evaluate** rules in order via `Router.Evaluate(ctx)` — first rule whose
   ops all match wins. Returns matched services, rule index, matched flag,
   and a per-rule trace in one pass.

```
                  ┌────────────────────────────────────────────────┐
                  │                                                │
   wire request   │   ExtractContext(req)  ───►  RequestContext    │
   (OpenAI /      │       │                                        │
    Anthropic v1/ │       │ uses                                   │
    Anthropic Beta)       ▼                                        │
                  │   internal/protocol/request/                   │
                  │   converters → Anthropic Beta                  │
                  │                                                │
                  └────────────────────────────────────────────────┘
                                                  │
                                                  ▼
                  ┌────────────────────────────────────────────────┐
                  │   Router.Evaluate(ctx)                         │
                  │     for rule in rules:                         │
                  │       for op in rule.Ops:                      │
                  │         res = evaluateOp(ctx, op)              │
                  │         if not res.Matched: break              │
                  │       if all matched: pick rule, return        │
                  │                                                │
                  │   returns (services, idx, matched, trace)      │
                  └────────────────────────────────────────────────┘
```

### Why one canonical protocol

Earlier the package carried its own protocol-specific extractors (one per
wire format), duplicating logic that already lived in
`internal/protocol/request/`. We picked **Anthropic Beta** as the canonical
because:

- It has the richest expressivity (system blocks, content blocks, tool_use,
  thinking, cache_control).
- Free converters from OpenAI already exist (`ConvertOpenAIToAnthropicRequest`).
- Anthropic v1 is structurally a subset (`ConvertAnthropicV1ToBetaRequest`
  is a thin field copy).

The extractor reads only what routing needs (model, system text, user text,
tool names, thinking flag, image presence, latest role/content type).

### Why one evaluator

The trace path used to be a parallel re-implementation of the boolean fast
path — the same per-position switch written twice, plus
`stage_smart_routing.go` calling both back-to-back per request. That's
double the CPU and a permanent source of fast/verbose drift (every new
position risked being added to only one side).

Now `evaluateOp` returns `OpEvalResult { Matched, Reason, Actual, … }`. The
boolean wrappers (`EvaluateRequest`, `EvaluateRequestWithIndex`) read
`.Matched`. `TraceEvaluation` and `Evaluate` read the rest. Adding a new
position means writing one method.

## Concepts

A **rule** (`SmartRouting`) is `[]Op + []Service`. The router iterates rules
in order and the first rule whose ops *all* match wins.

An **op** (`SmartOp`) is `Position + Operation + Value`:

- **Position** — what aspect of the request to inspect.
- **Operation** — how to compare it (contains / equals / >= / regex / glob).
- **Value** — the comparison target.

| Position | Reads | Operations |
|---|---|---|
| `model` | request model name | `contains`, `glob`, `equals` |
| `thinking` | thinking-enabled flag | `enabled`, `disabled` |
| `context_system` | concatenated system messages | `contains`, `regex` |
| `context_user` | concatenated user messages | `contains`, `regex` |
| `latest_user` | latest user message + content type | `contains`, `type` |
| `tool_use` | tool names from assistant messages | `equals` |
| `token` | estimated token count (chars/4) | `ge`, `gt`, `le`, `lt` |
| `service_ttft` | rule services' TTFT stats (ms) | `avg_le`, `avg_ge`, `max_le`, `max_ge` |
| `service_capacity` | rule services' seat utilization (%) | `util_le`, `util_ge`, `util_lt`, `util_gt` |
| `agent.claude_code` | detected Claude Code request kind | `equals` (`main` / `subagent` / `compact`) |
| `proxy_vision` | latest user content type (image?) | `enabled` (toggle — see Op-level processors) |

`service_ttft` and `service_capacity` are seeded per-rule before evaluation
(`collectRuleStats` / `filterCapacityForRule`); both **pass** when the
underlying data is empty so cold-start traffic isn't blocked.

### Claude Code request-kind detection

When the scenario is `claude_code`, the SmartRoutingStage populates
`RequestContext.ClaudeCodeRequestKind` by fingerprinting the system prompt
(`agent_detect.go`). Precedence is `compact` → `subagent` → `main`
(most-specific first). The `agent.claude_code` SmartOp surfaces this as a
routable position.

### Op-level processors and implicit bypass

Most ops are pure predicates: they read the request and return matched/not.
A few ops carry **side-effect behavior** in addition to the predicate.
These are *processor-bearing ops*. When SmartRoutingStage matches a rule
that contains one, it:

1. Looks up each op in the processor registry
   (`smartrouting.RegisterProcessor` / `LookupProcessor`).
2. Runs every collected processor's `Process(*ProcessorContext)` in op
   order. The processor receives the typed request (`*BetaMessageNewParams`,
   `*MessageNewParams`, `*ChatCompletionNewParams`) and may mutate it in
   place. The matched rule's `Services` are passed as the processor's
   upstream candidate pool — these are the providers the processor itself
   can call, NOT the downstream selection set.
3. Marks the rule index in `SelectionContext.BypassedSmartRules` and
   returns `(nil, false)` so the LoadBalancer stage (the global fallback)
   picks an upstream from the parent rule's top-level `Services` with the
   mutated request. This is the *implicit bypass* contract. The bypass is
   strictly one-shot — the mutated request is not re-evaluated against
   smart-routing rules, keeping post-processor behavior predictable.

Processor implementations live in `internal/server/processor/`; they
register at server boot via `processor.RegisterAll`. Only
`proxy_vision.enabled` ships in the first cut — see Use cases below.

### Trace

`Router.Evaluate` returns `[]RuleEvalResult` describing every rule that was
considered (in order, stopping at the first match). Each entry contains the
ops evaluated, their `Matched` flag, a human-readable `Reason`, and a
compact `Actual` snippet of the inspected value.

For text positions (`context_*`, `latest_user`), `Actual` is built by
`snippetAround` — a window around the matched needle decorated with `…` on
trimmed sides — so the trace stays small even for huge prompts. When there
is no match, it falls back to a short head snippet.

The trace is consumed by `internal/server/routing/stage_smart_routing.go`
and emitted as a structured log line so operators can see *why* a request
matched a rule (or didn't).

## Use cases

### Route haiku-class models to a cheaper provider

```go
SmartRouting{
    Description: "haiku → low-cost provider",
    Ops: []SmartOp{
        {Position: PositionModel, Operation: OpModelContains, Value: "haiku"},
    },
    Services: []*loadbalance.Service{cheapProviderHaiku},
}
```

### Send long prompts to a high-context provider

```go
SmartRouting{
    Description: "≥ 1M tokens → long-context provider",
    Ops: []SmartOp{
        {Position: PositionToken, Operation: OpTokenGe, Value: "1000000",
         Meta: SmartOpMeta{Type: ValueTypeInt}},
    },
    Services: []*loadbalance.Service{longContextProvider},
}
```

### Steer Claude Code subagents to a fast/cheap model

```go
SmartRouting{
    Description: "Claude Code subagents → haiku",
    Ops: []SmartOp{
        {Position: PositionAgentClaudeCode, Operation: OpAgentClaudeCodeEquals,
         Value: ClaudeCodeKindSubagent},
    },
    Services: []*loadbalance.Service{haikuProvider},
}
```

### Avoid services with degraded TTFT

```go
SmartRouting{
    Description: "skip slow providers",
    Ops: []SmartOp{
        {Position: PositionServiceTTFT, Operation: OpServiceTTFTAvgLe, Value: "2000",
         Meta: SmartOpMeta{Type: ValueTypeInt}},
    },
    Services: []*loadbalance.Service{primary, fallback},
}
```

### Drain a near-saturated provider

```go
SmartRouting{
    Description: "≥80% seats used → spillover provider",
    Ops: []SmartOp{
        {Position: PositionServiceCapacity, Operation: OpServiceCapacityUtilGe,
         Value: "80", Meta: SmartOpMeta{Type: ValueTypeInt}},
    },
    Services: []*loadbalance.Service{spillover},
}
```

### Make a text-only model accept image-bearing requests

Use the top-level `proxy_vision.enabled` op. Its services list is the
*upstream* the vision-proxy processor will call to describe images; the
rule's match returns `(nil, false)` so the LoadBalancer picks the actual
downstream model. Image content blocks are replaced in place with
`[image: <description>]`; on any failure they are stripped with
`[image: (description unavailable)]` so the downstream never sees an
unsupported block.

```go
SmartRouting{
    Description: "describe images so cheap text-only model can answer",
    Ops: []SmartOp{
        {Position: PositionProxyVision, Operation: OpProxyVisionEnabled},
    },
    // Vision-capable upstream (Anthropic-style provider).
    Services: []*loadbalance.Service{visionProvider},
}
```

### Combine multiple ops (AND semantics)

All ops in a rule must match — there is no OR, model OR with multiple rules:

```go
SmartRouting{
    Description: "thinking-enabled sonnet → reasoning provider",
    Ops: []SmartOp{
        {Position: PositionModel, Operation: OpModelContains, Value: "sonnet"},
        {Position: PositionThinking, Operation: OpThinkingEnabled},
    },
    Services: []*loadbalance.Service{reasoningProvider},
}
```

## Adding a new position

1. Add a `Position*` constant in `op.go` and the matching `Op*` constants.
2. Register the (position, operation, value-type) tuples in the `Operations`
   slice in `op.go` so `ValidateSmartOp` accepts them.
3. Add a field to `RequestContext` if the position needs new request data,
   and populate it in `ExtractContextFromBetaRequest`.
4. Add a `case PositionXxx:` arm in `Router.evaluateOp` and a
   `evaluateXxxOp(ctx, op) OpEvalResult` method.

That's it — there is no separate trace evaluator to also update.

## Files

| File | Purpose |
|---|---|
| `routing.go` | Router, evaluator dispatch, per-position evaluators |
| `eval_trace.go` | Trace types (`OpEvalResult`, `RuleEvalResult`), snippet helpers, `TraceEvaluation` wrapper |
| `context.go` | `RequestContext`, `ExtractContext` unified entry, Beta extractor |
| `agent_detect.go` | Claude Code request-kind fingerprinting |
| `op.go` | Position + Operation constants, `Operations` registry, `SmartOp` value parsing |
| `type.go` | `SmartRouting`, `SmartOp`, `SmartOpMeta` types |
