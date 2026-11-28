package agentboot

// Content block types a user message can carry.
const (
	ContentText  = "text"
	ContentImage = "image"
)

// ContentBlock is one piece of a user message: a run of text, or an image.
// A host builds them when the message is more than plain text — a message with
// picture attachments, say — and sets them on [ExecutionOptions.Content].
// Text-only messages keep using Prompt. The wire encoding belongs to the
// agent's transport (agentboot/claude for Claude Code).
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
