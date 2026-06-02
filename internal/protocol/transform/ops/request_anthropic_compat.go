package ops

import "github.com/anthropics/anthropic-sdk-go"

// ApplyAnthropicCompatRoleRewrite rewrites any "system" role found in the
// messages array to "user". The Anthropic API reserves the "system" role for
// the top-level system parameter; some clients erroneously send system-role
// entries inside the messages list. This op normalises them so third-party
// Anthropic-compatible providers that reject the non-standard role do not error.
func ApplyAnthropicCompatRoleRewrite(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	for i := range req.Messages {
		if req.Messages[i].Role == "system" {
			req.Messages[i].Role = anthropic.MessageParamRoleUser
		}
	}
}

// ApplyAnthropicBetaCompatRoleRewrite is the beta-variant of
// ApplyAnthropicCompatRoleRewrite, acting on BetaMessageNewParams.
func ApplyAnthropicBetaCompatRoleRewrite(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}
	for i := range req.Messages {
		if req.Messages[i].Role == "system" {
			req.Messages[i].Role = anthropic.BetaMessageParamRoleUser
		}
	}
}
