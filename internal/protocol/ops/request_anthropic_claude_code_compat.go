package ops

import "github.com/anthropics/anthropic-sdk-go"

// ApplyClaudeCodeCompatRoleRewrite normalizes any "system" role inside the
// Anthropic messages array so the request becomes valid for third-party
// Anthropic-compatible providers, which only accept "user"/"assistant" roles in
// messages and reject the non-standard mid-conversation "system" role.
//
// The normalization is position-aware on purpose. A system-role entry in
// messages is, by the Anthropic mid-conversation-system contract, always a
// mid-conversation operator message (it cannot be messages[0]); the global
// system prompt already lives in the top-level system field. So we never hoist
// it — hoisting would reorder a "as of turn N" instruction to a global one and
// invalidate the prompt cache. Instead the content is folded into an adjacent
// user turn so it stays in place and never produces two consecutive user
// messages (which strict third-party providers reject — "roles must
// alternate", trading one 400 for another):
//
//   - system after a user turn  → merged into that preceding user turn.
//   - system after an assistant → re-roled to "user" (assistant→user already
//     alternates).
//   - leading system(s) (defensive; the beta forbids messages[0]=="system") →
//     merged forward into the following user turn, or flushed as a lone user
//     turn if an assistant comes first.
//
// Non-system messages pass through untouched; a request with no system role is
// a pure pass-through (originally-consecutive user turns are left as-is).
func ApplyClaudeCodeCompatRoleRewrite(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	out := make([]anthropic.MessageParam, 0, len(req.Messages))
	var lead []anthropic.ContentBlockParamUnion // buffered leading system content

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if n := len(out); n > 0 && out[n-1].Role == anthropic.MessageParamRoleUser {
				out[n-1].Content = concatBlocks(out[n-1].Content, msg.Content)
				continue
			}
			if len(out) == 0 {
				lead = append(lead, msg.Content...)
				continue
			}
			msg.Role = anthropic.MessageParamRoleUser
			out = append(out, msg)
			continue
		}

		if len(lead) > 0 {
			if msg.Role == anthropic.MessageParamRoleUser {
				msg.Content = concatBlocks(lead, msg.Content)
			} else {
				out = append(out, anthropic.MessageParam{Role: anthropic.MessageParamRoleUser, Content: lead})
			}
			lead = nil
		}
		out = append(out, msg)
	}
	if len(lead) > 0 {
		out = append(out, anthropic.MessageParam{Role: anthropic.MessageParamRoleUser, Content: lead})
	}
	req.Messages = out
}

func concatBlocks(a, b []anthropic.ContentBlockParamUnion) []anthropic.ContentBlockParamUnion {
	merged := make([]anthropic.ContentBlockParamUnion, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return merged
}

// ApplyClaudeCodeBetaCompatRoleRewrite is the beta-namespace counterpart of
// ApplyClaudeCodeCompatRoleRewrite. The Beta SDK types are structurally
// identical but distinct, so the logic is duplicated rather than shared.
func ApplyClaudeCodeBetaCompatRoleRewrite(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}
	out := make([]anthropic.BetaMessageParam, 0, len(req.Messages))
	var lead []anthropic.BetaContentBlockParamUnion

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if n := len(out); n > 0 && out[n-1].Role == anthropic.BetaMessageParamRoleUser {
				out[n-1].Content = concatBetaBlocks(out[n-1].Content, msg.Content)
				continue
			}
			if len(out) == 0 {
				lead = append(lead, msg.Content...)
				continue
			}
			msg.Role = anthropic.BetaMessageParamRoleUser
			out = append(out, msg)
			continue
		}

		if len(lead) > 0 {
			if msg.Role == anthropic.BetaMessageParamRoleUser {
				msg.Content = concatBetaBlocks(lead, msg.Content)
			} else {
				out = append(out, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleUser, Content: lead})
			}
			lead = nil
		}
		out = append(out, msg)
	}
	if len(lead) > 0 {
		out = append(out, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleUser, Content: lead})
	}
	req.Messages = out
}

func concatBetaBlocks(a, b []anthropic.BetaContentBlockParamUnion) []anthropic.BetaContentBlockParamUnion {
	merged := make([]anthropic.BetaContentBlockParamUnion, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return merged
}
