package tool

// ContentType identifies the kind of content a tool produced.
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
	ContentTypeBlob  ContentType = "blob"
)

// ToolContent is a single piece of content from a tool result.
// It is protocol-agnostic — adapters convert it to the wire format required by
// each upstream model API.
type ToolContent struct {
	Type ContentType `json:"type"`
	// Text is populated when Type == ContentTypeText.
	Text string `json:"text,omitempty"`
	// Data is base64-encoded binary, populated when Type == ContentTypeImage or ContentTypeBlob.
	Data string `json:"data,omitempty"`
	// MIMEType is populated when Type == ContentTypeImage or ContentTypeBlob.
	MIMEType string `json:"mimeType,omitempty"`
}

// ToolResult is the structured return value from a servertool execution.
type ToolResult struct {
	Contents []ToolContent
	IsError  bool
}

// TextToolResult constructs a ToolResult with a single text content item.
func TextToolResult(text string) ToolResult {
	return ToolResult{Contents: []ToolContent{{Type: ContentTypeText, Text: text}}}
}

// ErrorToolResult constructs an error ToolResult with a single text content item.
func ErrorToolResult(text string) ToolResult {
	return ToolResult{
		Contents: []ToolContent{{Type: ContentTypeText, Text: text}},
		IsError:  true,
	}
}

// FirstText returns the text of the first text content item, or empty string if none.
func (r ToolResult) FirstText() string {
	for _, c := range r.Contents {
		if c.Type == ContentTypeText {
			return c.Text
		}
	}
	return ""
}
