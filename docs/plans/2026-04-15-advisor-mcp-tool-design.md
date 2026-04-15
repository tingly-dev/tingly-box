# Advisor MCP Built-in Tool Design

**Date**: 2026-04-15  
**Status**: Approved  
**Reference**: [Claude Advisor Strategy](https://claude.com/blog/the-advisor-strategy)

## Overview

This design implements the **Advisor Strategy** as a built-in MCP server tool inside tingly-box. When enabled, upstream AI models (the executor) can consult a more powerful advisor model at hard decision points. The advisor receives the full conversation context, a dedicated system prompt, and the executor's reason for escalation. It returns structured guidance without calling tools or producing user-facing output.

## Goals

- Provide an out-of-the-box `advisor` built-in MCP server tool
- Allow users to configure their own advisor endpoint (base URL, model, API key)
- Support both OpenAI-format and Anthropic-format advisor endpoints
- Limit consultations per request to control cost and latency
- Reuse existing tingly-box infrastructure (`client.ClientPool`, forwarding logic)

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              User Request                                    │
│  "How do I refactor this service to use CQRS?"                               │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Tingly-Box Proxy Server                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  1. Request Handler (openai_chat.go / anthropic handlers)           │    │
│  │     - Injects MCP tools into upstream request                       │    │
│  │     - Includes `tingly_box_mcp__builtin__advisor`                  │    │
│  │     - Injects `remaining_uses` into advisor tool description        │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│                                      ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  2. Upstream Executor Model                                         │    │
│  │     (e.g., gpt-4.1, claude-sonnet-4-6)                              │    │
│  │     - Receives user request + tool list                             │    │
│  │     - At some point, decides to call `advisor` tool                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│                    "I need strategic guidance"                             │
│                                      │                                       │
│                                      ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  3. MCP Tool Execution Loop                                         │    │
│  │     - Detects only MCP tool calls in response                       │    │
│  │     - Calls `BuiltinToolHandler.CallTool("advisor", args)`          │    │
│  │     - Args = { "reason": "unsure about CQRS boundaries" }           │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│                                      ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  4. Advisor Tool Handler                                            │    │
│  │     - Extracts full conversation context from request               │    │
│  │     - Prepends ADVISOR_SYSTEM_PROMPT                                │    │
│  │     - Appends worker's `reason` as final user message               │    │
│  │     - Calls advisor model via ClientPool (OpenAI or Anthropic)      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│              [worker context] + [advisor prompt] + [reason]                  │
│                                      │                                       │
│                                      ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  5. Advisor Model                                                   │    │
│  │     (e.g., claude-opus-4-6, configured by user)                     │    │
│  │     - Returns structured JSON:                                      │    │
│  │       { assessment, recommendation, unsolicited_findings }          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│                                      ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  6. Tool Result Injected Back                                       │    │
│  │     - Advisor response serialized as tool result                     │    │
│  │     - Sent back to executor model in follow-up request              │    │
│  │     - `remaining_uses` decremented by 1                             │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│                                      ▼                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  7. Executor Resumes                                                │    │
│  │     - Receives advisor guidance                                     │    │
│  │     - Continues working on the original task                        │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Components

### 1. `internal/mcp/runtime/builtin_advisor.go` (new file)

```go
package runtime

type AdvisorConfig struct {
    BaseURL           string `json:"base_url"`
    Model             string `json:"model"`
    APIKey            string `json:"api_key"`
    MaxUsesPerRequest int    `json:"max_uses_per_request"` // default: 3
}

type AdvisorTool struct {
    config     AdvisorConfig
    clientPool interface{ /* GetOpenAIClient / GetAnthropicClient */ }
}

func (a *AdvisorTool) Name() string
func (a *AdvisorTool) Description(remainingUses int) string
func (a *AdvisorTool) CallTool(ctx context.Context, args string, reqCtx *AdvisorRequestContext) (string, error)
```

### 2. `internal/mcp/runtime/builtin_handler.go` (extended)

- Add `advisorTool *AdvisorTool` field to `BuiltinToolHandler`
- Add `SetAdvisorConfig(cfg AdvisorConfig)` method
- Route `advisor` calls in `CallTool()` to the advisor tool

### 3. Request handlers (extended)

- `internal/server/openai_chat.go`
- `internal/server/mcp_anthropic_loop.go`

Changes:
- Before MCP tool injection, compute `remainingUses` for this request
- Inject the advisor tool with a dynamic description containing `remainingUses`
- During tool execution, pass conversation context through `context.Context`
- After each advisor call, decrement the counter

### 4. `internal/typ` (extended)

Extend the builtin source config to optionally hold `AdvisorConfig`.

### 5. Prompts

**`ADVISOR_SYSTEM_PROMPT`** (constant in `builtin_advisor.go`):

```
You are an advisor to a coding agent. You share the agent's full conversation context and provide strategic guidance.

Your role:
- Provide plans, corrections, or stop signals
- Be concise and actionable — the executor will act on your advice immediately
- Focus on the "why" and the "what", not the "how" (the executor handles execution)
- Flag risks, edge cases, or better approaches the executor may have missed
- IMPORTANT: If you notice issues the executor did NOT ask about — bugs, security flaws, design problems, missed edge cases — proactively report them. The executor may have blind spots; your job is to catch what they miss.

You do NOT:
- Call tools or execute commands
- Produce user-facing output
- Repeat information already in the conversation
- Ask follow-up questions (give your best guidance with available context)

Structure your response as valid JSON:
{
  "assessment": "What's the situation? (1-2 sentences)",
  "recommendation": "What should the executor do? (actionable steps)",
  "unsolicited_findings": "Anything else you noticed that the executor should know, even if they didn't ask. Skip this field if there's nothing to add."
}
```

## Data Flow

### Passing Conversation Context

We thread request state through `context.Context` using a private key:

```go
type advisorContextKey struct{}

type AdvisorContext struct {
    Messages      []Message // worker conversation history
    UsesRemaining int
}

func WithAdvisorContext(ctx context.Context, ac *AdvisorContext) context.Context
func GetAdvisorContext(ctx context.Context) (*AdvisorContext, bool)
```

In the request handler:

```go
ctx = WithAdvisorContext(ctx, &AdvisorContext{
    Messages:      extractMessages(req),
    UsesRemaining: maxUses,
})
```

### Dynamic Tool Description

Before every tool injection, regenerate the description:

```go
advisorDesc := advisorTool.Description(remainingUses)
```

Example description:

```
Consult a more powerful advisor model for strategic guidance.
Use this when facing architectural decisions, complex debugging, unclear trade-offs, or when stuck.
You have 2 advisor consultation(s) remaining this request.
```

### Advisor Request Format — OpenAI / Anthropic Adaptive

```go
func detectAdvisorFormat(cfg AdvisorConfig) AdvisorFormat {
    url := strings.ToLower(cfg.BaseURL)
    model := strings.ToLower(cfg.Model)
    if strings.Contains(url, "anthropic") || strings.HasPrefix(model, "claude-") {
        return FormatAnthropic
    }
    return FormatOpenAI
}
```

- **OpenAI path**: Build `openai.ChatCompletionNewParams`, call `clientPool.GetOpenAIClient()`, parse `choices[0].message.content`
- **Anthropic path**: Build `anthropic.MessageNewParams`, call `clientPool.GetAnthropicClient()`, parse `content[0].Text`

Both paths construct a temporary `*typ.Provider` from `AdvisorConfig` so all existing forwarding, retry, and error logic is reused.

### Response Normalization

```go
type AdvisorResponse struct {
    Assessment          string `json:"assessment"`
    Recommendation      string `json:"recommendation"`
    UnsolicitedFindings string `json:"unsolicited_findings,omitempty"`
}
```

If JSON parsing fails, return the raw text as the tool result.

## Error Handling & Edge Cases

| Scenario | Behavior |
|---|---|
| **Advisor config missing** | Tool not registered; executor does not see it |
| **Advisor returns non-JSON** | Return raw text as tool result |
| **Advisor model call fails** | Return error message as tool result (`is_error=true` for Anthropic) |
| **Max uses exhausted** | Description shows 0; if called anyway, return `"Advisor consultations exhausted for this request."` |
| **Empty conversation context** | Proceed with system prompt + reason only |
| **Circular advisor calls** | Not possible — advisor requests do not include the `advisor` tool |
| **Streaming requests** | MCP loop runs after stream completion, same logic |

## Configuration Example

```yaml
tool_runtime:
  sources:
    - id: builtin
      transport: builtin
      tools: ["web_search", "web_fetch", "advisor"]
      config:
        advisor:
          base_url: "https://api.anthropic.com/v1"
          model: "claude-opus-4-6"
          api_key: "${ANTHROPIC_API_KEY}"
          max_uses_per_request: 3
```

## Testing Strategy

1. **Unit tests for `AdvisorTool`**
   - `Description()` embeds correct `remaining_uses`
   - `detectAdvisorFormat()` identifies OpenAI vs Anthropic correctly
   - `CallTool()` builds correct messages for both formats
   - Graceful fallback on invalid JSON

2. **Integration tests for MCP tool loop**
   - Mock advisor provider returning fixed JSON
   - Verify executor receives advisor result as a tool message
   - Verify `remaining_uses` decrements across multiple calls

3. **Handler end-to-end tests**
   - `max_uses=1` with two advisor calls; second call returns "exhausted"

## Key Files to Modify / Create

### New Files
- `internal/mcp/runtime/builtin_advisor.go` — Advisor tool implementation

### Modified Files
- `internal/mcp/runtime/builtin_handler.go` — Register and route advisor tool
- `internal/mcp/runtime/runtime.go` — Wire advisor config from builtin source config
- `internal/server/openai_chat.go` — Pass context, handle uses counter
- `internal/server/mcp_anthropic_loop.go` — Pass context, handle uses counter
- `internal/typ/tool_runtime.go` (or equivalent) — Add `AdvisorConfig` to builtin config struct
