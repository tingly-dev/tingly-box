package ops

import "github.com/anthropics/anthropic-sdk-go"

// ApplyClaudeCodeCompatRoleRewrite rewrites any "system" role found in the
// messages array to "user". Claude Code sends system-role entries inside the
// messages list (a non-standard extension); this op normalises them so
// third-party Anthropic-compatible providers that reject the non-standard role
// do not error.
func ApplyClaudeCodeCompatRoleRewrite(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	for i := range req.Messages {
		if req.Messages[i].Role == "system" {
			req.Messages[i].Role = anthropic.MessageParamRoleUser
		}
	}
}

// ApplyClaudeCodeBetaCompatRoleRewrite is the beta-variant of
// ApplyClaudeCodeCompatRoleRewrite, acting on BetaMessageNewParams.
func ApplyClaudeCodeBetaCompatRoleRewrite(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}
	for i := range req.Messages {
		if req.Messages[i].Role == "system" {
			req.Messages[i].Role = anthropic.BetaMessageParamRoleUser
		}
	}
}
