package routing

import (
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SelectionContext encapsulates all input needed for service selection.
// It is created once per request and passed through the selection pipeline.
type SelectionContext struct {
	// Rule is the routing rule being evaluated
	Rule *typ.Rule

	// Request is the parsed API request (OpenAI/Anthropic params)
	Request interface{}

	// SessionID is the resolved session identifier for affinity
	// Priority: Tingly header > native client header > Anthropic metadata.user_id > ClientIP
	SessionID typ.SessionID

	// GinContext provides access to HTTP headers and client info
	GinContext *gin.Context

	// Scenario identifies the request type (openai, anthropic, etc.)
	Scenario typ.RuleScenario

	// MatchedSmartRuleIndex tracks which smart routing rule matched (-1 if none)
	// This is set by SmartRoutingStage and used for smart_rule-scoped affinity
	MatchedSmartRuleIndex int

	// BypassedSmartRules records smart-routing rules whose processors have
	// already run and bypassed the stage (returned (nil, false) to let the
	// pipeline continue). SmartRoutingStage skips these on re-evaluation so
	// processors do not loop on residual matchable content.
	BypassedSmartRules map[int]struct{}
}

// NewSelectionContext creates a new selection context with resolved session ID
func NewSelectionContext(
	rule *typ.Rule,
	req interface{},
	c *gin.Context,
	scenario typ.RuleScenario,
) *SelectionContext {
	return &SelectionContext{
		Rule:                  rule,
		Request:               req,
		SessionID:             ResolveSessionID(c, req),
		GinContext:            c,
		Scenario:              scenario,
		MatchedSmartRuleIndex: -1, // default: no match
	}
}

// ResolveSessionID returns the best available session identifier from the request.
// Priority: X-Tingly-Session-ID header > native client header > Anthropic metadata.user_id > ClientIP.
// The client IP is always stored in IPBackup as a fallback for rate limiting or logging.
func ResolveSessionID(c *gin.Context, req interface{}) typ.SessionID {
	clientIP := c.ClientIP()

	// 1. The explicit Tingly header always wins, allowing callers and trusted
	// gateways to override identifiers supplied automatically by a client.
	if id := c.GetHeader("X-Tingly-Session-ID"); id != "" {
		return typ.SessionID{Source: typ.SessionSourceHeader, Value: id, IPBackup: clientIP}
	}

	// 2. Native coding-agent headers. Codex uses session_id while OpenCode uses
	// x-opencode-session.
	if id := nativeClientSessionID(c.Request.Header); id != "" {
		return typ.SessionID{Source: typ.SessionSourceHeader, Value: id, IPBackup: clientIP}
	}

	// 3. Extract from Anthropic request metadata.user_id only when no explicit or
	// native client session header was provided.
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		if r.Metadata.UserID.Valid() && r.Metadata.UserID.Value != "" {
			v := r.Metadata.UserID.Value
			return typ.SessionID{Source: typ.SessionSourceUser, Value: v, IPBackup: clientIP}
		}
	case *anthropic.BetaMessageNewParams:
		if r.Metadata.UserID.Valid() && r.Metadata.UserID.Value != "" {
			v := r.Metadata.UserID.Value
			return typ.SessionID{Source: typ.SessionSourceUser, Value: v, IPBackup: clientIP}
		}
	}

	// 4. Fallback: client IP (IPBackup not needed since Value is the IP)
	return typ.SessionID{Source: typ.SessionSourceIP, Value: clientIP}
}

// nativeClientSessionID extracts session identifiers emitted by supported coding agents.
// Header.Get is case-insensitive and also handles Codex's underscore-bearing header name.
func nativeClientSessionID(header http.Header) string {
	for _, name := range []string{"session_id", "x-opencode-session"} {
		if id := header.Get(name); id != "" {
			return id
		}
	}
	return ""
}
