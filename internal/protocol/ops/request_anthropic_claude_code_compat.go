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
// it — hoisting would reorder an "as of turn N" instruction to a global one and
// invalidate the prompt cache. Instead each system block is folded, in place,
// into a user turn under one hard constraint: the result must never contain two
// consecutive user messages (strict providers reject that — "roles must
// alternate" — which would just trade one 400 for another).
//
// The merge *direction* is not a free choice; it is forced by the system
// block's neighbours. Enumerating (prev role, next role):
//
//	prev=user,      next=user      → collapse prev+system+next into one user turn
//	prev=user,      next=assistant → merge backward into the preceding user
//	prev=user,      next=∅ (end)   → merge backward into the preceding user
//	prev=assistant, next=user      → merge forward into the following user
//	prev=assistant, next=assistant → stand alone as its own user turn
//	prev=assistant, next=∅         → stand alone as its own user turn
//	prev=∅,         next=user      → merge forward into the following user
//	prev=∅,         next=assistant → stand alone as its own user turn
//	prev=∅,         next=∅         → stand alone as its own user turn (sole msg)
//
// In one rule: fold backward when the preceding turn is a user, otherwise fold
// forward into the next user, and only stand alone when neither neighbour is a
// user. The direction can only be decided once the *next* role is known, so
// system content is held in a pending buffer and placed when the following
// non-system message (or the end of the array) is reached.
//
// Non-system messages with no pending system pass through untouched; a request
// with no system role is a pure pass-through (genuinely pre-existing
// consecutive user turns are left as-is — we only avoid *creating* new ones).
func ApplyClaudeCodeCompatRoleRewrite(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	if !hasSystemRole(req.Messages) {
		return // already valid — leave the messages (and their backing array) untouched
	}
	out := make([]anthropic.MessageParam, 0, len(req.Messages))
	var pending []anthropic.ContentBlockParamUnion // system content not yet placed

	// placePendingBackward folds buffered system content into the preceding user
	// turn, or stands it up as its own user turn when there is none to merge into
	// (preceding turn is an assistant, or out is empty). Used when the next turn
	// is an assistant or we have reached the end — i.e. forward merge is unavailable.
	placePendingBackward := func() {
		if len(pending) == 0 {
			return
		}
		if n := len(out); n > 0 && out[n-1].Role == anthropic.MessageParamRoleUser {
			out[n-1].Content = concatBlocks(out[n-1].Content, pending)
		} else {
			out = append(out, anthropic.MessageParam{Role: anthropic.MessageParamRoleUser, Content: pending})
		}
		pending = nil
	}

	for _, msg := range req.Messages {
		switch {
		case msg.Role == "system":
			pending = append(pending, msg.Content...)

		case msg.Role == anthropic.MessageParamRoleUser:
			if len(pending) > 0 {
				if n := len(out); n > 0 && out[n-1].Role == anthropic.MessageParamRoleUser {
					// prev=user, next=user → collapse all three into the preceding turn.
					out[n-1].Content = concatBlocks(concatBlocks(out[n-1].Content, pending), msg.Content)
					pending = nil
					continue
				}
				// prev=assistant/∅, next=user → merge forward into this user.
				msg.Content = concatBlocks(pending, msg.Content)
				pending = nil
			}
			out = append(out, msg)

		default: // assistant (or any other non-user, non-system role)
			placePendingBackward()
			out = append(out, msg)
		}
	}
	placePendingBackward()
	req.Messages = out
}

// hasSystemRole reports whether any message carries the non-standard "system"
// role. When none do, the messages are already valid and the rewrite is a no-op,
// so callers can skip the allocate-and-rebuild pass entirely.
func hasSystemRole(msgs []anthropic.MessageParam) bool {
	for i := range msgs {
		if msgs[i].Role == "system" {
			return true
		}
	}
	return false
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
	if !hasBetaSystemRole(req.Messages) {
		return // already valid — leave the messages (and their backing array) untouched
	}
	out := make([]anthropic.BetaMessageParam, 0, len(req.Messages))
	var pending []anthropic.BetaContentBlockParamUnion

	placePendingBackward := func() {
		if len(pending) == 0 {
			return
		}
		if n := len(out); n > 0 && out[n-1].Role == anthropic.BetaMessageParamRoleUser {
			out[n-1].Content = concatBetaBlocks(out[n-1].Content, pending)
		} else {
			out = append(out, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleUser, Content: pending})
		}
		pending = nil
	}

	for _, msg := range req.Messages {
		switch {
		case msg.Role == "system":
			pending = append(pending, msg.Content...)

		case msg.Role == anthropic.BetaMessageParamRoleUser:
			if len(pending) > 0 {
				if n := len(out); n > 0 && out[n-1].Role == anthropic.BetaMessageParamRoleUser {
					out[n-1].Content = concatBetaBlocks(concatBetaBlocks(out[n-1].Content, pending), msg.Content)
					pending = nil
					continue
				}
				msg.Content = concatBetaBlocks(pending, msg.Content)
				pending = nil
			}
			out = append(out, msg)

		default: // assistant (or any other non-user, non-system role)
			placePendingBackward()
			out = append(out, msg)
		}
	}
	placePendingBackward()
	req.Messages = out
}

func hasBetaSystemRole(msgs []anthropic.BetaMessageParam) bool {
	for i := range msgs {
		if msgs[i].Role == "system" {
			return true
		}
	}
	return false
}

func concatBetaBlocks(a, b []anthropic.BetaContentBlockParamUnion) []anthropic.BetaContentBlockParamUnion {
	merged := make([]anthropic.BetaContentBlockParamUnion, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return merged
}
