package smartrouting

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// RequestContext holds extracted request data for evaluation
type RequestContext struct {
	Model             string
	ThinkingEnabled   bool
	SystemMessages    []string
	UserMessages      []string
	ToolUses          []string
	LatestRole        string // Latest message role (user, assistant, tool, function, etc.)
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
func ExtractContextFromOpenAIRequest(req *openai.ChatCompletionNewParams) *RequestContext {
	ctx := &RequestContext{
		Model: string(req.Model),
	}

	if req.Messages != nil {
		for _, msgUnion := range req.Messages {
			switch {
			case msgUnion.OfSystem != nil:
				contentStr := extractOpenAISystemContent(msgUnion.OfSystem.Content)
				ctx.SystemMessages = append(ctx.SystemMessages, contentStr)
				ctx.LatestRole = "system"
			case msgUnion.OfUser != nil:
				contentStr, hasImage := extractOpenAIUserContent(msgUnion.OfUser.Content)
				ctx.UserMessages = append(ctx.UserMessages, contentStr)
				ctx.LatestRole = "user"
				if hasImage {
					ctx.LatestContentType = "image"
				}
			case msgUnion.OfAssistant != nil:
				ctx.LatestRole = "assistant"
				// Extract tool calls from assistant messages
				if len(msgUnion.OfAssistant.ToolCalls) > 0 {
					for _, toolCallUnion := range msgUnion.OfAssistant.ToolCalls {
						if toolCallUnion.OfFunction != nil {
							ctx.ToolUses = append(ctx.ToolUses, toolCallUnion.OfFunction.Function.Name)
						}
					}
				}
			case msgUnion.OfTool != nil:
				ctx.LatestRole = "tool"
			case msgUnion.OfFunction != nil:
				ctx.LatestRole = "function"
			}
		}
	}

	// Estimate tokens from all content
	allContent := strings.Join(append(ctx.SystemMessages, ctx.UserMessages...), "\n")
	ctx.EstimatedTokens = EstimateTokens(allContent)

	return ctx
}

// extractOpenAISystemContent extracts string content from system message
func extractOpenAISystemContent(content openai.ChatCompletionSystemMessageParamContentUnion) string {
	if content.OfString.Valid() {
		return content.OfString.Value
	}
	// Array of text parts
	var parts []string
	for _, textPart := range content.OfArrayOfContentParts {
		parts = append(parts, textPart.Text)
	}
	return strings.Join(parts, "\n")
}

// extractOpenAIUserContent extracts string content and checks for image from user message content
func extractOpenAIUserContent(content openai.ChatCompletionUserMessageParamContentUnion) (string, bool) {
	var parts []string
	hasImage := false

	// Check if it's a string
	if content.OfString.Valid() {
		return content.OfString.Value, false
	}

	// Check if it's an array of content parts
	for _, partUnion := range content.OfArrayOfContentParts {
		switch {
		case partUnion.OfText != nil:
			parts = append(parts, partUnion.OfText.Text)
		case partUnion.OfImageURL != nil:
			hasImage = true
			parts = append(parts, "[image]")
		}
	}

	return strings.Join(parts, "\n"), hasImage
}

// ExtractContextFromAnthropicRequest extracts RequestContext from an Anthropic messages request
func ExtractContextFromAnthropicRequest(req *anthropic.MessageNewParams) *RequestContext {
	ctx := &RequestContext{
		Model:           string(req.Model),
		ThinkingEnabled: req.Thinking.OfEnabled != nil,
	}

	if req.System != nil {
		for _, s := range req.System {
			if s.Text != "" {
				ctx.SystemMessages = append(ctx.SystemMessages, s.Text)
			}
		}
	}

	if req.Messages != nil {
		for _, msg := range req.Messages {
			ctx.LatestRole = string(msg.Role)

			// Only process user messages
			if string(msg.Role) != "user" {
				continue
			}

			contentStr, toolUses := extractAnthropicContent(msg.Content)
			hasImage := hasImageInAnthropicContent(msg.Content)

			if contentStr != "" {
				ctx.UserMessages = append(ctx.UserMessages, contentStr)
			}
			if hasImage {
				ctx.LatestContentType = "image"
			}

			ctx.ToolUses = append(ctx.ToolUses, toolUses...)
		}
	}

	// Estimate tokens from all content
	allContent := strings.Join(append(ctx.SystemMessages, ctx.UserMessages...), "\n")
	ctx.EstimatedTokens = EstimateTokens(allContent)

	return ctx
}

// extractAnthropicContent extracts string content and tool uses from Anthropic content blocks
func extractAnthropicContent(content []anthropic.ContentBlockParamUnion) (string, []string) {
	var parts []string
	var tools []string

	for _, blockUnion := range content {
		switch {
		case blockUnion.OfText != nil:
			parts = append(parts, blockUnion.OfText.Text)
		case blockUnion.OfImage != nil:
			parts = append(parts, "[image]")
		case blockUnion.OfToolUse != nil:
			tools = append(tools, blockUnion.OfToolUse.Name)
		}
	}

	return strings.Join(parts, "\n"), tools
}

// hasImageInAnthropicContent checks if content contains image
func hasImageInAnthropicContent(content []anthropic.ContentBlockParamUnion) bool {
	for _, blockUnion := range content {
		if blockUnion.OfImage != nil {
			return true
		}
	}
	return false
}
