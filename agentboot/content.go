package agentboot

// Content block types a user message can carry.
const (
	ContentText  = "text"
	ContentImage = "image"
)

// ContentBlock is one piece of a user message: a run of text, or an image as
// base64 bytes. A host builds them when the message is more than plain text —
// a chat with picture attachments, say — and sets them on
// [ExecutionOptions.Content]. Text-only messages keep using Prompt.
type ContentBlock struct {
	// Type is ContentText or ContentImage.
	Type string
	// Text is the text of a ContentText block.
	Text string
	// MediaType and Data describe a ContentImage block: a MIME type such as
	// image/png and the raw bytes, base64-encoded.
	MediaType string
	Data      string
}

// TextContent is a text content block.
func TextContent(text string) ContentBlock {
	return ContentBlock{Type: ContentText, Text: text}
}

// ImageContent is an image content block from base64-encoded bytes.
func ImageContent(mediaType, base64Data string) ContentBlock {
	return ContentBlock{Type: ContentImage, MediaType: mediaType, Data: base64Data}
}

// Wire renders the blocks in the shape the agent's input protocol expects: a
// text block is {"type":"text","text":…}; an image block carries its base64
// bytes under {"type":"image","source":{"type":"base64",…}}.
func (c ContentBlock) Wire() map[string]any {
	switch c.Type {
	case ContentImage:
		return map[string]any{
			"type":   ContentImage,
			"source": map[string]any{"type": "base64", "media_type": c.MediaType, "data": c.Data},
		}
	default:
		return map[string]any{"type": ContentText, "text": c.Text}
	}
}

// WireContent renders a user message's content: the plain prompt when there are
// no blocks, else an array of blocks. The prompt stays the leading text block
// unless the blocks already carry text.
func WireContent(prompt string, content []ContentBlock) any {
	if len(content) == 0 {
		return prompt
	}
	hasText := false
	for _, c := range content {
		if c.Type == ContentText || c.Type == "" {
			hasText = true
			break
		}
	}
	out := make([]any, 0, len(content)+1)
	if prompt != "" && !hasText {
		out = append(out, map[string]any{"type": ContentText, "text": prompt})
	}
	for _, c := range content {
		out = append(out, c.Wire())
	}
	return out
}
