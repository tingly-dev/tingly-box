package smartrouting

import (
	"fmt"
	"strings"

	"github.com/valyala/fastjson"
	"tingly-box/internal/loadbalance"
)

// RequestContext holds extracted request data for evaluation
type RequestContext struct {
	Model             string
	ThinkingEnabled   bool
	SystemMessages    []string
	UserMessages      []string
	ToolUses          []string
	LatestContentType string
	EstimatedTokens   int
}

// GetLatestUserMessage returns the latest user message
func (rc *RequestContext) GetLatestUserMessage() string {
	if len(rc.UserMessages) == 0 {
		return ""
	}
	return rc.UserMessages[len(rc.UserMessages)-1]
}

// CombineMessages combines all messages of a type into a single string
func (rc *RequestContext) CombineMessages(messages []string) string {
	return strings.Join(messages, "\n")
}

// ExtractContextFromOpenAIRequest extracts RequestContext from an OpenAI chat completion request
func ExtractContextFromOpenAIRequest(body []byte) (*RequestContext, error) {
	ctx := &RequestContext{}

	// Parse JSON
	var parser fastjson.Parser
	v, err := parser.ParseBytes(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract model
	if modelBytes := v.GetStringBytes("model"); len(modelBytes) > 0 {
		ctx.Model = string(modelBytes)
	}

	// Extract messages
	messagesArray := v.GetArray("messages")
	for _, msgVal := range messagesArray {
		msgObj, err := msgVal.Object()
		if err != nil {
			continue
		}

		// Get role using Object.Get() then Value.StringBytes()
		if roleVal := msgObj.Get("role"); roleVal != nil {
			if roleBytes, err := roleVal.StringBytes(); err == nil {
				role := string(roleBytes)

				// Extract content (can be string or array)
				contentVal := msgObj.Get("content")
				if contentVal == nil {
					continue
				}

				contentStr := extractContentString(contentVal)

				switch role {
				case "system":
					if contentStr != "" {
						ctx.SystemMessages = append(ctx.SystemMessages, contentStr)
					}
				case "user":
					if contentStr != "" {
						ctx.UserMessages = append(ctx.UserMessages, contentStr)
					}
					// Check for image content type
					if hasImageContent(contentVal) {
						ctx.LatestContentType = "image"
					}
				}
			}
		}
	}

	// Extract tool uses if present (in tools array or tool_use in content)
	ctx.ToolUses = extractToolUsesOpenAI(v)

	// Estimate tokens from all content
	allContent := strings.Join(append(ctx.SystemMessages, ctx.UserMessages...), "\n")
	ctx.EstimatedTokens = EstimateTokens(allContent)

	return ctx, nil
}

// ExtractContextFromAnthropicRequest extracts RequestContext from an Anthropic messages request
func ExtractContextFromAnthropicRequest(body []byte) (*RequestContext, error) {
	ctx := &RequestContext{}

	// Parse JSON
	var parser fastjson.Parser
	v, err := parser.ParseBytes(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract model
	if modelBytes := v.GetStringBytes("model"); len(modelBytes) > 0 {
		ctx.Model = string(modelBytes)
	}

	// Extract system prompt
	if systemBytes := v.GetStringBytes("system"); len(systemBytes) > 0 {
		ctx.SystemMessages = append(ctx.SystemMessages, string(systemBytes))
	}

	// Check thinking enabled
	if thinkingVal := v.Get("thinking"); thinkingVal != nil {
		if thinkingVal.Type() == fastjson.TypeObject {
			ctx.ThinkingEnabled = true
		}
	}

	// Extract messages
	messagesArray := v.GetArray("messages")
	for _, msgVal := range messagesArray {
		msgObj, err := msgVal.Object()
		if err != nil {
			continue
		}

		// Get role
		if roleVal := msgObj.Get("role"); roleVal != nil {
			if roleBytes, err := roleVal.StringBytes(); err == nil {
				role := string(roleBytes)

				// Extract content
				contentVal := msgObj.Get("content")
				if contentVal == nil {
					continue
				}

				contentStr := extractContentString(contentVal)
				toolUses := extractToolUsesAnthropicContent(contentVal)

				switch role {
				case "user":
					if contentStr != "" {
						ctx.UserMessages = append(ctx.UserMessages, contentStr)
					}
					// Check for image content type
					if hasImageContent(contentVal) {
						ctx.LatestContentType = "image"
					}
				}

				// Collect tool uses
				for _, toolUse := range toolUses {
					ctx.ToolUses = append(ctx.ToolUses, toolUse)
				}
			}
		}
	}

	// Also check for tools definition
	toolsArray := v.GetArray("tools")
	for _, toolVal := range toolsArray {
		toolObj, err := toolVal.Object()
		if err != nil {
			continue
		}
		if nameVal := toolObj.Get("name"); nameVal != nil {
			if nameBytes, err := nameVal.StringBytes(); err == nil {
				ctx.ToolUses = append(ctx.ToolUses, string(nameBytes))
			}
		}
	}

	// Estimate tokens from all content
	allContent := strings.Join(append(ctx.SystemMessages, ctx.UserMessages...), "\n")
	ctx.EstimatedTokens = EstimateTokens(allContent)

	return ctx, nil
}

// extractContentString extracts a string representation from content value
func extractContentString(contentVal *fastjson.Value) string {
	if contentVal == nil {
		return ""
	}

	switch contentVal.Type() {
	case fastjson.TypeString:
		if bytes, err := contentVal.StringBytes(); err == nil {
			return string(bytes)
		}
		return ""
	case fastjson.TypeArray:
		// Content is an array of blocks
		array, _ := contentVal.Array()
		var parts []string
		for _, item := range array {
			obj, err := item.Object()
			if err != nil {
				continue
			}

			if typeVal := obj.Get("type"); typeVal != nil {
				if typeBytes, err := typeVal.StringBytes(); err == nil {
					typeStr := string(typeBytes)

					switch typeStr {
					case "text":
						if textVal := obj.Get("text"); textVal != nil {
							if textBytes, err := textVal.StringBytes(); err == nil {
								parts = append(parts, string(textBytes))
							}
						}
					case "image":
						// Skip image data, just note the type
						parts = append(parts, "[image]")
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return string(contentVal.MarshalTo(nil))
	}
}

// hasImageContent checks if content contains image
func hasImageContent(contentVal *fastjson.Value) bool {
	if contentVal == nil {
		return false
	}

	if contentVal.Type() == fastjson.TypeArray {
		array, _ := contentVal.Array()
		for _, item := range array {
			obj, err := item.Object()
			if err != nil {
				continue
			}
			if typeVal := obj.Get("type"); typeVal != nil {
				if typeBytes, err := typeVal.StringBytes(); err == nil {
					if string(typeBytes) == "image" {
						return true
					}
				}
			}
		}
	}

	return false
}

// extractToolUsesOpenAI extracts tool names from OpenAI request
func extractToolUsesOpenAI(v *fastjson.Value) []string {
	var tools []string

	// Check for tool_calls in messages
	messagesArray := v.GetArray("messages")
	for _, msgVal := range messagesArray {
		msgObj, err := msgVal.Object()
		if err != nil {
			continue
		}

		// Check for tool_calls array
		if toolCallsVal := msgObj.Get("tool_calls"); toolCallsVal != nil {
			if toolCallsArray, err := toolCallsVal.Array(); err == nil {
				for _, toolCallVal := range toolCallsArray {
					toolCallObj, err := toolCallVal.Object()
					if err != nil {
						continue
					}
					funcObj := toolCallObj.Get("function")
					if funcObj != nil {
						if nameVal := funcObj.Get("name"); nameVal != nil {
							if nameBytes, err := nameVal.StringBytes(); err == nil {
								tools = append(tools, string(nameBytes))
							}
						}
					}
				}
			}
		}
	}

	// Check for tools definition array
	toolsArray := v.GetArray("tools")
	for _, toolVal := range toolsArray {
		toolObj, err := toolVal.Object()
		if err != nil {
			continue
		}
		if functionVal := toolObj.Get("function"); functionVal != nil {
			if nameVal := functionVal.Get("name"); nameVal != nil {
				if nameBytes, err := nameVal.StringBytes(); err == nil {
					tools = append(tools, string(nameBytes))
				}
			}
		}
	}

	return tools
}

// extractToolUsesAnthropicContent extracts tool names from Anthropic content blocks
func extractToolUsesAnthropicContent(contentVal *fastjson.Value) []string {
	var tools []string

	if contentVal.Type() != fastjson.TypeArray {
		return tools
	}

	array, _ := contentVal.Array()
	for _, item := range array {
		obj, err := item.Object()
		if err != nil {
			continue
		}

		if typeVal := obj.Get("type"); typeVal != nil {
			if typeBytes, err := typeVal.StringBytes(); err == nil {
				if string(typeBytes) == "tool_use" {
					if nameVal := obj.Get("name"); nameVal != nil {
						if nameBytes, err := nameVal.StringBytes(); err == nil {
							tools = append(tools, string(nameBytes))
						}
					}
				}
			}
		}
	}

	return tools
}

// EvaluateAndSelectServices evaluates a request and returns the appropriate services
// This is the main entry point for smart routing
func EvaluateAndSelectServices(router *Router, scenario string, body []byte) ([]loadbalance.Service, bool) {
	if router == nil {
		return nil, false
	}

	var ctx *RequestContext
	var err error

	// Extract context based on scenario
	switch scenario {
	case "openai":
		ctx, err = ExtractContextFromOpenAIRequest(body)
	case "anthropic", "claude_code":
		ctx, err = ExtractContextFromAnthropicRequest(body)
	default:
		// Default to OpenAI format
		ctx, err = ExtractContextFromOpenAIRequest(body)
	}

	if err != nil {
		return nil, false
	}

	// Evaluate against smart routing rules
	return router.EvaluateRequest(ctx)
}
