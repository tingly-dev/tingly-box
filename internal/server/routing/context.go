package routing

import (
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
	// Priority: Anthropic metadata.user_id > X-Tingly-Session-ID header > ClientIP
	SessionID typ.SessionID

	// GinContext provides access to HTTP headers and client info
	GinContext *gin.Context

	// Scenario identifies the request type (openai, anthropic, etc.)
	Scenario typ.RuleScenario

	// MatchedSmartRuleIndex tracks which smart routing rule matched (-1 if none)
	// This is set by SmartRoutingStage and used for smart_rule-scoped affinity
	MatchedSmartRuleIndex int
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
// Priority: Anthropic metadata.user_id > X-Tingly-Session-ID header > ClientIP
func ResolveSessionID(c *gin.Context, req interface{}) typ.SessionID {
	// 1. Extract from Anthropic request metadata.user_id
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		if r.Metadata.UserID.Valid() && r.Metadata.UserID.Value != "" {
			return typ.SessionID{Source: typ.SessionSourceUser, Value: r.Metadata.UserID.Value}
		}
	case *anthropic.BetaMessageNewParams:
		if r.Metadata.UserID.Valid() && r.Metadata.UserID.Value != "" {
			return typ.SessionID{Source: typ.SessionSourceUser, Value: r.Metadata.UserID.Value}
		}
	}

	// 2. X-Tingly-Session-ID header
	if id := c.GetHeader("X-Tingly-Session-ID"); id != "" {
		return typ.SessionID{Source: typ.SessionSourceHeader, Value: id}
	}

	// 3. Fallback: client IP
	return typ.SessionID{Source: typ.SessionSourceIP, Value: c.ClientIP()}
}
