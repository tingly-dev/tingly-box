package runtime

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

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
// It replaces the former plain-string return from Runtime.CallTool.
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

// ContentFromMCP converts an mcp.Content value to a ToolContent.
// Unknown content types are JSON-serialized as text.
func ContentFromMCP(c mcp.Content) ToolContent {
	switch v := c.(type) {
	case mcp.TextContent:
		return ToolContent{Type: ContentTypeText, Text: v.Text}
	case mcp.ImageContent:
		return ToolContent{Type: ContentTypeImage, Data: v.Data, MIMEType: v.MIMEType}
	}
	b, _ := json.Marshal(c)
	return ToolContent{Type: ContentTypeText, Text: string(b)}
}

// ToolResultFromMCPResult converts an *mcp.CallToolResult to a ToolResult.
func ToolResultFromMCPResult(r *mcp.CallToolResult) ToolResult {
	if r == nil {
		return ToolResult{}
	}
	out := ToolResult{IsError: r.IsError}
	for _, c := range r.Content {
		out.Contents = append(out.Contents, ContentFromMCP(c))
	}
	return out
}

// ToMCPContents converts ToolResult contents back to []mcp.Content for the
// local MCP server boundary (tingly-box acting as MCP server).
func (r ToolResult) ToMCPContents() []mcp.Content {
	out := make([]mcp.Content, 0, len(r.Contents))
	for _, c := range r.Contents {
		switch c.Type {
		case ContentTypeImage:
			out = append(out, mcp.ImageContent{Type: "image", Data: c.Data, MIMEType: c.MIMEType})
		default:
			out = append(out, mcp.TextContent{Type: "text", Text: c.Text})
		}
	}
	return out
}
