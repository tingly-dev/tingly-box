package client

import (
	"bytes"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ClaudeToolPrefix is empty to match real Claude Code behavior (no tool name prefix).
const ClaudeToolPrefix = ""

// IsClaudeOAuthToken checks if the given API key is a Claude OAuth token
// by checking for the "sk-ant-oat" prefix.
func IsClaudeOAuthToken(apiKey string) bool {
	if apiKey == "" {
		return false
	}
	return containsString(apiKey, "sk-ant-oat")
}

// containsString is a simple string contains check for portability
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || indexOfSubstring(s, substr) >= 0))
}

// indexOfSubstring finds the index of substr in s
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ApplyClaudeToolPrefix applies a prefix to tool names in the request body.
// This is required for Claude Code OAuth tokens to avoid conflicts with
// built-in tools. The prefix is applied to user-defined tools only.
func ApplyClaudeToolPrefix(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}

	// Collect built-in tool names (those with a non-empty "type" field) so we can
	// skip them consistently in both tools and message history.
	builtinTools := map[string]bool{}
	for _, name := range []string{"web_search", "code_execution", "text_editor", "computer"} {
		builtinTools[name] = true
	}

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() {
		tools.ForEach(func(index, tool gjson.Result) bool {
			// Skip built-in tools (web_search, code_execution, etc.) which have
			// a "type" field and require their name to remain unchanged.
			if tool.Get("type").Exists() && tool.Get("type").String() != "" {
				if n := tool.Get("name").String(); n != "" {
					builtinTools[n] = true
				}
				return true
			}

			// Prefix user-defined tools (those without a "type" field)
			name := tool.Get("name").String()
			if name != "" && !startsWith(name, prefix) && !builtinTools[name] {
				path := "tools." + index.String() + ".name"
				body, _ = sjson.SetBytes(body, path, prefix+name)
			}
			return true
		})
	}

	// Prefix tool names in message history (tool_result blocks that reference tool_use)
	if messages := gjson.GetBytes(body, "messages"); messages.Exists() && messages.IsArray() {
		messages.ForEach(func(_, msg gjson.Result) bool {
			if content := msg.Get("content"); content.Exists() && content.IsArray() {
				content.ForEach(func(_, part gjson.Result) bool {
					partType := part.Get("type").String()
					if partType == "tool_result" {
						toolUseID := part.Get("tool_use_id").String()
						// Find the matching tool_use block and check if its name has our prefix
						// If so, we need to update tool_reference blocks to match
						messages.ForEach(func(_, searchMsg gjson.Result) bool {
							searchContent := searchMsg.Get("content")
							if searchContent.Exists() && searchContent.IsArray() {
								searchContent.ForEach(func(_, searchPart gjson.Result) bool {
									if searchPart.Get("type").String() == "tool_use" &&
										searchPart.Get("id").String() == toolUseID {
										toolName := searchPart.Get("name").String()
										if startsWith(toolName, prefix) {
											// This tool_result references a prefixed tool
											// Need to ensure tool_reference blocks also get prefix
											nestedContent := part.Get("content")
											if nestedContent.Exists() && nestedContent.IsArray() {
												nestedContent.ForEach(func(_, nestedPart gjson.Result) bool {
													if nestedPart.Get("type").String() == "tool_reference" {
														nestedToolName := nestedPart.Get("tool_name").String()
														if nestedToolName != "" && !startsWith(nestedToolName, prefix) && !builtinTools[nestedToolName] {
															msgPath := fmt.Sprintf("messages.%d.content.%d.content.%d.tool_name", msg.Index, part.Index, nestedPart.Index)
															body, _ = sjson.SetBytes(body, msgPath, prefix+nestedToolName)
														}
													}
													return true
												})
											}
										}
										return false
									}
									return true
								})
							}
							return true
						})
					}
					return true
				})
			}
			return true
		})
	}

	return body
}

// StripClaudeToolPrefixFromResponse removes the tool prefix from tool names
// in the API response. This reverses the transformation done by ApplyClaudeToolPrefix.
func StripClaudeToolPrefixFromResponse(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}
	content := gjson.GetBytes(body, "content")
	if !content.Exists() || !content.IsArray() {
		return body
	}
	content.ForEach(func(_, part gjson.Result) bool {
		partType := part.Get("type").String()
		switch partType {
		case "tool_use":
			name := part.Get("name").String()
			if !startsWith(name, prefix) {
				return true
			}
			path := fmt.Sprintf("content.%d.name", part.Index)
			body, _ = sjson.SetBytes(body, path, trimPrefix(name, prefix))
		case "tool_reference":
			toolName := part.Get("tool_name").String()
			if !startsWith(toolName, prefix) {
				return true
			}
			path := fmt.Sprintf("content.%d.tool_name", part.Index)
			body, _ = sjson.SetBytes(body, path, trimPrefix(toolName, prefix))

			// Handle nested tool_reference blocks inside tool_result.content[]
			nestedContent := part.Get("content")
			if nestedContent.Exists() && nestedContent.IsArray() {
				nestedContent.ForEach(func(_, nestedPart gjson.Result) bool {
					if nestedPart.Get("type").String() == "tool_reference" {
						nestedToolName := nestedPart.Get("tool_name").String()
						if startsWith(nestedToolName, prefix) {
							nestedPath := fmt.Sprintf("content.%d.content.%d.tool_name", part.Index, nestedPart.Index)
							body, _ = sjson.SetBytes(body, nestedPath, trimPrefix(nestedToolName, prefix))
						}
					}
					return true
				})
			}
		}
		return true
	})
	return body
}

// StripClaudeToolPrefixFromStreamLine removes the tool prefix from tool names
// in a single SSE stream line. This is used for streaming responses.
func StripClaudeToolPrefixFromStreamLine(line []byte, prefix string) []byte {
	if prefix == "" {
		return line
	}
	payload := jsonPayloadFromSSE(line)
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return line
	}
	contentBlock := gjson.GetBytes(payload, "content_block")
	if !contentBlock.Exists() {
		return line
	}

	blockType := contentBlock.Get("type").String()
	var updated []byte
	var err error

	switch blockType {
	case "tool_use":
		name := contentBlock.Get("name").String()
		if !startsWith(name, prefix) {
			return line
		}
		updated, err = sjson.SetBytes(payload, "content_block.name", trimPrefix(name, prefix))
		if err != nil {
			return line
		}
	case "tool_reference":
		toolName := contentBlock.Get("tool_name").String()
		if !startsWith(toolName, prefix) {
			return line
		}
		updated, err = sjson.SetBytes(payload, "content_block.tool_name", trimPrefix(toolName, prefix))
		if err != nil {
			return line
		}
	default:
		return line
	}

	trimmed := bytes.TrimSpace(line)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		return append([]byte("data: "), updated...)
	}
	return updated
}

// jsonPayloadFromSSE extracts JSON payload from an SSE line
// by removing the "data: " prefix if present
func jsonPayloadFromSSE(line []byte) []byte {
	trimmed := bytes.TrimSpace(line)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		payload := trimmed[5:]
		return bytes.TrimSpace(payload)
	}
	return line
}

// startsWith checks if a string starts with a prefix
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// trimPrefix removes a prefix from a string
func trimPrefix(s, prefix string) string {
	if startsWith(s, prefix) {
		return s[len(prefix):]
	}
	return s
}
