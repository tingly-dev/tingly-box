package ops

import (
	"fmt"
	"regexp"

	"github.com/anthropics/anthropic-sdk-go"
)

// serverToolUseIDPattern is the ID format api.anthropic.com enforces on
// server_tool_use blocks (messages.N.content.M.server_tool_use.id).
var serverToolUseIDPattern = regexp.MustCompile(`^srvtoolu_[a-zA-Z0-9_]+$`)

// serverToolUseIDInvalidChars matches every character the pattern above rejects.
var serverToolUseIDInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// SanitizeAnthropicV1ServerToolUseIDs rewrites server_tool_use block IDs in the
// replayed message history so they satisfy Anthropic's ^srvtoolu_[a-zA-Z0-9_]+$
// requirement. History that passed through another provider (an OpenAI-converted
// response or a third-party Anthropic-compatible endpoint) can carry IDs minted
// in that provider's format; forwarding them verbatim makes api.anthropic.com
// reject the whole request with a 400. Result blocks referencing a rewritten ID
// via tool_use_id are remapped to keep the pairing intact.
func SanitizeAnthropicV1ServerToolUseIDs(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}

	remap := make(map[string]string)
	unnamed := 0
	for mi := range req.Messages {
		for bi := range req.Messages[mi].Content {
			stu := req.Messages[mi].Content[bi].OfServerToolUse
			if stu == nil || serverToolUseIDPattern.MatchString(stu.ID) {
				continue
			}
			newID, ok := remap[stu.ID]
			if !ok {
				newID = rewriteServerToolUseID(stu.ID, &unnamed)
				if stu.ID != "" {
					remap[stu.ID] = newID
				}
			}
			stu.ID = newID
		}
	}

	if len(remap) == 0 {
		return
	}
	for mi := range req.Messages {
		for bi := range req.Messages[mi].Content {
			if idRef := req.Messages[mi].Content[bi].GetToolUseID(); idRef != nil {
				if newID, ok := remap[*idRef]; ok {
					*idRef = newID
				}
			}
		}
	}
}

// SanitizeAnthropicBetaServerToolUseIDs is the Beta-variant of
// SanitizeAnthropicV1ServerToolUseIDs.
func SanitizeAnthropicBetaServerToolUseIDs(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}

	remap := make(map[string]string)
	unnamed := 0
	for mi := range req.Messages {
		for bi := range req.Messages[mi].Content {
			stu := req.Messages[mi].Content[bi].OfServerToolUse
			if stu == nil || serverToolUseIDPattern.MatchString(stu.ID) {
				continue
			}
			newID, ok := remap[stu.ID]
			if !ok {
				newID = rewriteServerToolUseID(stu.ID, &unnamed)
				if stu.ID != "" {
					remap[stu.ID] = newID
				}
			}
			stu.ID = newID
		}
	}

	if len(remap) == 0 {
		return
	}
	for mi := range req.Messages {
		for bi := range req.Messages[mi].Content {
			if idRef := req.Messages[mi].Content[bi].GetToolUseID(); idRef != nil {
				if newID, ok := remap[*idRef]; ok {
					*idRef = newID
				}
			}
		}
	}
}

// rewriteServerToolUseID derives a conforming ID from a foreign one. The
// original ID is kept (with rejected characters replaced by "_") so the model
// can still correlate the block across turns, and the mapping stays
// deterministic for identical input. Empty IDs get a per-request counter since
// they carry nothing to correlate on.
func rewriteServerToolUseID(id string, unnamed *int) string {
	if id == "" {
		*unnamed++
		return fmt.Sprintf("srvtoolu_missing_%d", *unnamed)
	}
	sanitized := serverToolUseIDInvalidChars.ReplaceAllString(id, "_")
	if serverToolUseIDPattern.MatchString(sanitized) {
		return sanitized
	}
	return "srvtoolu_" + sanitized
}
