package server

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// applyContextOneM checks if the incoming request
// contains the context-1m beta header and updates the matched rule copy
// accordingly. This handles Claude Code/Desktop automatically triggering 1M
// context mode.
//
// The rule passed in is the per-request copy returned by rule matching, so
// the changes below scope to this request only (flag → upstream beta-header
// injection, [1m] name suffix → response model display); nothing persists to
// the stored config.
//
// For Claude Code/Desktop scenarios (profiles included):
// - If incoming request has context-1m beta header → set flag + [1m] name suffix
//
// For Codex scenario:
// - Only sets the flag (no name suffix); Codex manages model names independently
func applyContextOneM(c *gin.Context, rule *typ.Rule) {
	if rule == nil {
		return
	}

	// Match on the base scenario so profile rules (e.g. "claude_code:p1")
	// get the same treatment as the main scenario.
	var nameSuffix string
	switch rule.Scenario.Base() {
	case typ.ScenarioClaudeCode, typ.ScenarioClaudeDesktop:
		nameSuffix = config.Context1MSuffix
	case typ.ScenarioCodex:
		// flag only, no name suffix
	default:
		// Other scenarios: no auto-detection
		return
	}

	// Check for incoming context-1m beta header
	betaHeader := c.GetHeader("anthropic-beta")
	if betaHeader == "" || !strings.Contains(betaHeader, client.AnthropicContext1m) {
		return
	}

	if !rule.Flags.Context1M {
		// Downstream effects ride this flag: resolveRuleFlagsWithScenario turns
		// it into the context hint the outbound Anthropic transport reads to
		// inject the context-1m beta flag upstream.
		rule.Flags.Context1M = true

		// Update rule name with suffix for Claude Code/Desktop
		if nameSuffix != "" && rule.RequestModel != "" && !strings.HasSuffix(rule.RequestModel, nameSuffix) {
			rule.RequestModel = rule.RequestModel + nameSuffix
			if rule.ResponseModel != "" {
				rule.ResponseModel = rule.ResponseModel + nameSuffix
			}
		}
	}
}
