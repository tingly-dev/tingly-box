package claude

import "encoding/json"

// ContentBlock types.
type ContentBlock interface {
	GetContentType() string
}

// TextBlock represents text content.
type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// GetContentType implements ContentBlock.
func (b *TextBlock) GetContentType() string {
	return b.Type
}

// ToolUseBlock represents a tool use invocation.
type ToolUseBlock struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// GetContentType implements ContentBlock.
func (b *ToolUseBlock) GetContentType() string {
	return b.Type
}

// ThinkingBlock represents reasoning/thinking content.
type ThinkingBlock struct {
	Type     string `json:"type"`
	Thinking string `json:"thinking"`
}

// GetContentType implements ContentBlock.
func (b *ThinkingBlock) GetContentType() string {
	return b.Type
}

// RedactedThinkingBlock represents reasoning withheld by the API.
type RedactedThinkingBlock struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// GetContentType implements ContentBlock.
func (b *RedactedThinkingBlock) GetContentType() string {
	return b.Type
}

// ServerToolResultContentBlock preserves the common envelope shared by
// Claude's server-side tool result variants.
type ServerToolResultContentBlock struct {
	Type      string          `json:"type"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// GetContentType implements ContentBlock.
func (b *ServerToolResultContentBlock) GetContentType() string {
	return b.Type
}

// ContainerUploadContentBlock represents a file uploaded to Claude's
// server-side execution container.
type ContainerUploadContentBlock struct {
	Type   string `json:"type"`
	FileID string `json:"file_id"`
}

// GetContentType implements ContentBlock.
func (b *ContainerUploadContentBlock) GetContentType() string {
	return b.Type
}

// ToolResultContentBlock represents tool result content (within message content array).
type ToolResultContentBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// GetContentType implements ContentBlock.
func (b *ToolResultContentBlock) GetContentType() string {
	return b.Type
}

// UnmarshalContentBlock unmarshals a content block from JSON.
func UnmarshalContentBlock(data []byte) (ContentBlock, error) {
	var typeDetect struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeDetect); err != nil {
		return nil, err
	}

	switch typeDetect.Type {
	case ContentBlockTypeText:
		var block TextBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case ContentBlockTypeToolUse, ContentBlockTypeServerToolUse:
		var block ToolUseBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case ContentBlockTypeThinking:
		var block ThinkingBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case ContentBlockTypeRedactedThinking:
		var block RedactedThinkingBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case ContentBlockTypeToolResult:
		var block ToolResultContentBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case ContentBlockTypeWebSearchToolResult,
		ContentBlockTypeWebFetchToolResult,
		ContentBlockTypeCodeExecutionToolResult,
		ContentBlockTypeBashCodeExecutionToolResult,
		ContentBlockTypeTextEditorExecutionToolResult,
		ContentBlockTypeToolSearchToolResult:
		var block ServerToolResultContentBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	case ContentBlockTypeContainerUpload:
		var block ContainerUploadContentBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &block, nil
	default:
		// Return unknown block type
		var block map[string]interface{}
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return &UnknownBlock{Data: block}, nil
	}
}

// UnknownBlock represents an unrecognized content block.
type UnknownBlock struct {
	Data map[string]interface{}
}

// GetContentType implements ContentBlock.
func (b *UnknownBlock) GetContentType() string {
	if t, ok := b.Data["type"].(string); ok {
		return t
	}
	return "unknown"
}
