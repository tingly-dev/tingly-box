package server

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// detectAndApplyContext1MFromIncomingRequest checks if the incoming request
// contains the context-1m beta header and updates the rule accordingly.
// This handles Claude Code/Desktop automatically triggering 1M context mode.
//
// For Claude Code/Desktop scenarios:
// - If incoming request has context-1m beta header → update rule name + flag
// - Rule name gets [1m] suffix appended
// - Rule.Context1M flag is set to true
//
// For Codex scenario:
// - Only updates the flag (no name suffix)
// - Codex manages model names independently
func detectAndApplyContext1MFromIncomingRequest(c *gin.Context, rule *typ.Rule) {
	if rule == nil {
		return
	}

	// Check if this is a scenario where we should auto-detect context-1m.
	// Match on the base scenario so profile rules (e.g. "claude_code:p1")
	// get the same treatment as the main scenario.
	autoDetect := false
	var nameSuffix string

	switch rule.Scenario.Base() {
	case typ.ScenarioClaudeCode, typ.ScenarioClaudeDesktop:
		// Claude Code/Desktop: auto-detect + name suffix
		autoDetect = true
		nameSuffix = "[1m]"
	case typ.ScenarioCodex:
		// Codex: auto-detect only (no name suffix)
		autoDetect = true
		nameSuffix = ""
	default:
		// Other scenarios: no auto-detection
		return
	}

	if !autoDetect {
		return
	}

	// Check for incoming context-1m beta header
	betaHeader := c.GetHeader("anthropic-beta")
	if betaHeader == "" {
		return
	}

	// Check if context-1m is present in the beta header
	hasContext1M := strings.Contains(betaHeader, client.AnthropicContext1m) ||
		strings.Contains(betaHeader, "context-1m-2025-08-07")

	// Update the rule based on detected context-1m
	if hasContext1M {
		// Set the flag
		if !rule.Flags.Context1M {
			rule.Flags.Context1M = true

			// Update rule name with suffix for Claude Code/Desktop
			if nameSuffix != "" && !strings.HasSuffix(rule.RequestModel, nameSuffix) {
				rule.RequestModel = rule.RequestModel + nameSuffix
				if rule.ResponseModel != "" {
					rule.ResponseModel = rule.ResponseModel + nameSuffix
				}
			}

			// Log the automatic detection
			c.Set("context_1m_detected", true)
		}
	} else {
		// If context-1m was previously enabled but not in this request,
		// we could optionally disable it, but for now we leave it as-is
		// to avoid flipping back and forth on requests that don't specify it
	}
}

// shouldApplyContext1MToRule checks if we should apply context-1m to a rule
// based on the current rule state and incoming request.
// This is used by transforms and client code to determine if beta header injection is needed.
func shouldApplyContext1MToRule(c *gin.Context, rule *typ.Rule) bool {
	if rule == nil {
		return false
	}

	// If rule flag is explicitly set, use that
	if rule.Flags.Context1M {
		return true
	}

	// Check if context-1m was detected from incoming request
	if detected, exists := c.Get("context_1m_detected"); exists {
		if detectedBool, ok := detected.(bool); ok && detectedBool {
			return true
		}
	}

	return false
}

// getModelWithContext1MSuffix returns the model name with [1m] suffix if context-1m
// is enabled for the rule, otherwise returns the model name unchanged.
func getModelWithContext1MSuffix(model string, rule *typ.Rule) string {
	if rule == nil || model == "" {
		return model
	}

	// Only add suffix for display purposes - the actual model sent to Anthropic
	// should remain unchanged (context-1m is handled via beta header)
	if rule.Flags.Context1M && !strings.HasSuffix(model, "[1m]") {
		return model + "[1m]"
	}

	return model
}

// stripContext1MSuffix removes the [1m] suffix from a model name if present.
// This is used to get the actual model name for API calls.
func stripContext1MSuffix(model string) string {
	if strings.HasSuffix(model, "[1m]") {
		return strings.TrimSuffix(model, "[1m]")
	}
	return model
}
