package ops

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
)

// Rolling cache breakpoint for Anthropic-compatible third-party providers.
//
// Claude Code marks the system prompt and only the *last* message block, and
// moves that one message breakpoint forward every turn. Anthropic finds the
// previous turn's cache entry anyway by looking back up to 20 blocks from the
// new breakpoint. Third-party Anthropic-compatible providers commonly match
// only at the breakpoints present in the current request, so the position the
// previous turn wrote at — the end of what was then the last message — is no
// longer marked, and the whole conversation is re-read uncached every turn.
//
// AddAnthropic*PreviousTurnBreakpoint re-marks that position: the last
// cacheable block of the user message before the final one, which is where
// the previous request ended. Adding a marker never changes the prompt
// content (the cache-shape invariant in internal/protocol/request), and it is
// skipped when the request already uses the four breakpoints Anthropic's
// format allows.

// anthropicMaxBreakpoints is the per-request breakpoint limit of the
// Anthropic Messages API.
const anthropicMaxBreakpoints = 4

// AddAnthropicBetaPreviousTurnBreakpoint adds the previous-turn breakpoint to
// a beta request; see the package comment above.
func AddAnthropicBetaPreviousTurnBreakpoint(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}
	count := 0
	var latest *anthropic.BetaCacheControlEphemeralParam // the last message breakpoint
	mark := func(cc *anthropic.BetaCacheControlEphemeralParam) bool {
		if cc != nil && !param.IsOmitted(*cc) {
			count++
			return true
		}
		return false
	}
	if !param.IsOmitted(req.CacheControl) {
		count++
	}
	for i := range req.System {
		mark(&req.System[i].CacheControl)
	}
	for i := range req.Tools {
		mark(req.Tools[i].GetCacheControl())
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			if cc := req.Messages[i].Content[j].GetCacheControl(); mark(cc) {
				latest = cc
			}
		}
	}
	// Only a client that already rolls a message breakpoint is extended; a
	// request without one is left as sent.
	if latest == nil || count >= anthropicMaxBreakpoints {
		return
	}

	// The user message before the final user message ended the previous request.
	users := 0
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != anthropic.BetaMessageParamRoleUser {
			continue
		}
		users++
		if users < 2 {
			continue
		}
		content := req.Messages[i].Content
		for j := len(content) - 1; j >= 0; j-- {
			cc := content[j].GetCacheControl()
			if cc == nil {
				continue // block type without cache_control; try the one before
			}
			if param.IsOmitted(*cc) {
				*cc = *latest // same type and ttl as the client's own breakpoint
			}
			return
		}
		return
	}
}

// AddAnthropicV1PreviousTurnBreakpoint is AddAnthropicBetaPreviousTurnBreakpoint
// for v1 requests.
func AddAnthropicV1PreviousTurnBreakpoint(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	count := 0
	var latest *anthropic.CacheControlEphemeralParam // the last message breakpoint
	mark := func(cc *anthropic.CacheControlEphemeralParam) bool {
		if cc != nil && !param.IsOmitted(*cc) {
			count++
			return true
		}
		return false
	}
	if !param.IsOmitted(req.CacheControl) {
		count++
	}
	for i := range req.System {
		mark(&req.System[i].CacheControl)
	}
	for i := range req.Tools {
		mark(req.Tools[i].GetCacheControl())
	}
	for i := range req.Messages {
		for j := range req.Messages[i].Content {
			if cc := req.Messages[i].Content[j].GetCacheControl(); mark(cc) {
				latest = cc
			}
		}
	}
	if latest == nil || count >= anthropicMaxBreakpoints {
		return
	}

	users := 0
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != anthropic.MessageParamRoleUser {
			continue
		}
		users++
		if users < 2 {
			continue
		}
		content := req.Messages[i].Content
		for j := len(content) - 1; j >= 0; j-- {
			cc := content[j].GetCacheControl()
			if cc == nil {
				continue
			}
			if param.IsOmitted(*cc) {
				*cc = *latest
			}
			return
		}
		return
	}
}
