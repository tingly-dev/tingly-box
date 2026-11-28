package claude

import "github.com/tingly-dev/tingly-box/agentboot"

// wireContent renders a user message's content in the shape Claude Code's
// stream-json input expects: a plain string when there are no blocks, else an
// array of content blocks. An image block carries base64 bytes under
// {"type":"image","source":{"type":"base64",…}}. The prompt stays the leading
// text block unless the blocks already carry text.
//
// The neutral agentboot.ContentBlock is just data; this file is where it maps
// onto Claude's protocol.
func wireContent(prompt string, content []agentboot.ContentBlock) any {
	if len(content) == 0 {
		return prompt
	}
	hasText := false
	for _, c := range content {
		if c.Type == agentboot.ContentText || c.Type == "" {
			hasText = true
			break
		}
	}
	out := make([]any, 0, len(content)+1)
	if prompt != "" && !hasText {
		out = append(out, map[string]any{"type": agentboot.ContentText, "text": prompt})
	}
	for _, c := range content {
		out = append(out, wireBlock(c))
	}
	return out
}

// wireBlock is one content block in Claude's wire shape.
func wireBlock(c agentboot.ContentBlock) map[string]any {
	switch c.Type {
	case agentboot.ContentImage:
		return map[string]any{
			"type":   agentboot.ContentImage,
			"source": map[string]any{"type": "base64", "media_type": c.MediaType, "data": c.Data},
		}
	default:
		return map[string]any{"type": agentboot.ContentText, "text": c.Text}
	}
}
