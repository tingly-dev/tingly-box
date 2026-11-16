package tool

import "encoding/json"

// ContentType identifies the kind of content a tool produced.
type ContentType string

const (
	ContentTypeText         ContentType = "text"
	ContentTypeImage        ContentType = "image"
	ContentTypeBlob         ContentType = "blob"
	ContentTypeAudio        ContentType = "audio"
	ContentTypeResource     ContentType = "resource"
	ContentTypeResourceLink ContentType = "resource_link"
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
	// Raw preserves MCP content fields (resources, annotations, links) across bridges.
	Raw json.RawMessage `json:"-"`
}

// ToolResult is the structured return value from a servertool execution.
type ToolResult struct {
	Contents          []ToolContent  `json:"content"`
	IsError           bool           `json:"isError,omitempty"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

func (c *ToolContent) UnmarshalJSON(data []byte) error {
	type plain ToolContent
	if err := json.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}
	c.Raw = append(c.Raw[:0], data...)
	return nil
}

func (c ToolContent) MarshalJSON() ([]byte, error) {
	if len(c.Raw) > 0 {
		return c.Raw, nil
	}
	type plain ToolContent
	return json.Marshal(plain(c))
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

// ModelText renders content for model APIs that cannot accept the original MCP block.
// Preserve resource/audio/link fields as JSON rather than silently emitting empty text.
func (c ToolContent) ModelText() string {
	if c.Type == ContentTypeText {
		return c.Text
	}
	raw, _ := json.Marshal(c)
	return string(raw)
}
