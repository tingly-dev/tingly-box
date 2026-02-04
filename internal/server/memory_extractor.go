package server

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// MemoryExtractor extracts round data from protocol-specific requests
type MemoryExtractor interface {
	// ExtractRounds extracts conversation rounds from a request body
	ExtractRounds(requestBody []byte) ([]db.RoundData, error)

	// ExtractMetadata extracts protocol-specific metadata from response headers
	ExtractMetadata(headers http.Header) (map[string]interface{}, error)

	// NormalizeMessage converts protocol-specific message to normalized format
	NormalizeMessage(msg interface{}) (map[string]interface{}, error)

	// GetProtocol returns the protocol type for this extractor
	GetProtocol() db.ProtocolType
}

// AnthropicPromptExtractor extracts rounds from Anthropic API requests
type AnthropicPromptExtractor struct{}

// GetProtocol returns the protocol type
func (e *AnthropicPromptExtractor) GetProtocol() db.ProtocolType {
	return db.ProtocolAnthropic
}

// ExtractRounds extracts conversation rounds from Anthropic v1/v1beta request body
func (e *AnthropicPromptExtractor) ExtractRounds(requestBody []byte) ([]db.RoundData, error) {
	// Try to parse as beta request first
	var betaReq struct {
		Messages []anthropic.BetaMessageParam `json:"messages"`
	}
	if err := json.Unmarshal(requestBody, &betaReq); err == nil && len(betaReq.Messages) > 0 {
		return e.extractFromBetaMessages(betaReq.Messages), nil
	}

	// Try to parse as v1 request
	var v1Req struct {
		Messages []anthropic.MessageParam `json:"messages"`
	}
	if err := json.Unmarshal(requestBody, &v1Req); err != nil {
		return nil, err
	}

	return e.extractFromV1Messages(v1Req.Messages), nil
}

// extractFromV1Messages extracts rounds from v1 messages using the existing Grouper
func (e *AnthropicPromptExtractor) extractFromV1Messages(messages []anthropic.MessageParam) []db.RoundData {
	grouper := protocol.NewGrouper()
	rounds := grouper.GroupV1(messages)

	result := make([]db.RoundData, len(rounds))
	for i, round := range rounds {
		result[i] = db.RoundData{
			RoundIndex:   i,
			UserInput:    e.extractUserInputFromV1(round.Messages),
			RoundResult:  e.extractRoundResultFromV1(round.Messages),
			FullMessages: e.normalizeV1Messages(round.Messages),
		}
	}
	return result
}

// extractFromBetaMessages extracts rounds from beta messages using the existing Grouper
func (e *AnthropicPromptExtractor) extractFromBetaMessages(messages []anthropic.BetaMessageParam) []db.RoundData {
	grouper := protocol.NewGrouper()
	rounds := grouper.GroupBeta(messages)

	result := make([]db.RoundData, len(rounds))
	for i, round := range rounds {
		result[i] = db.RoundData{
			RoundIndex:   i,
			UserInput:    e.extractUserInputFromBeta(round.Messages),
			RoundResult:  e.extractRoundResultFromBeta(round.Messages),
			FullMessages: e.normalizeBetaMessages(round.Messages),
		}
	}
	return result
}

// ExtractMetadata extracts Anthropic-specific metadata from response headers
func (e *AnthropicPromptExtractor) ExtractMetadata(headers http.Header) (map[string]interface{}, error) {
	metadata := make(map[string]interface{})

	// Extract Anthropic-specific headers
	if userID := headers.Get("anthropic-user-id"); userID != "" {
		metadata["anthropic_user_id"] = userID
	}

	// Note: project_id and session_id are extracted to top-level fields separately
	// These headers are handled by the caller

	return metadata, nil
}

// ExtractProjectID extracts project ID from Anthropic response headers
func (e *AnthropicPromptExtractor) ExtractProjectID(headers http.Header) string {
	return headers.Get("anthropic-project-id")
}

// ExtractSessionID extracts session ID from Anthropic response headers
func (e *AnthropicPromptExtractor) ExtractSessionID(headers http.Header) string {
	return headers.Get("anthropic-session-id")
}

// ExtractRequestID extracts request ID from Anthropic response headers
func (e *AnthropicPromptExtractor) ExtractRequestID(headers http.Header) string {
	return headers.Get("anthropic-request-id")
}

// NormalizeMessage converts an Anthropic v1 message to normalized format
func (e *AnthropicPromptExtractor) NormalizeMessage(msg interface{}) (map[string]interface{}, error) {
	switch m := msg.(type) {
	case anthropic.MessageParam:
		return e.normalizeV1Message(m), nil
	case anthropic.BetaMessageParam:
		return e.normalizeBetaMessage(m), nil
	default:
		return nil, nil
	}
}

// Helper methods for v1 messages

func (e *AnthropicPromptExtractor) extractUserInputFromV1(messages []anthropic.MessageParam) string {
	for _, msg := range messages {
		if string(msg.Role) == "user" {
			// Build user input from content blocks
			var input string
			for _, block := range msg.Content {
				if block.OfText != nil {
					input += block.OfText.Text + "\n"
				}
			}
			return input
		}
	}
	return ""
}

func (e *AnthropicPromptExtractor) extractRoundResultFromV1(messages []anthropic.MessageParam) string {
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			// Build assistant response from content blocks
			var result string
			for _, block := range msg.Content {
				if block.OfText != nil {
					result += block.OfText.Text + "\n"
				}
			}
			return result
		}
	}
	return ""
}

func (e *AnthropicPromptExtractor) normalizeV1Messages(messages []anthropic.MessageParam) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = e.normalizeV1Message(msg)
	}
	return result
}

func (e *AnthropicPromptExtractor) normalizeV1Message(msg anthropic.MessageParam) map[string]interface{} {
	normalized := map[string]interface{}{
		"role": string(msg.Role),
	}

	// Normalize content blocks
	content := make([]map[string]interface{}, len(msg.Content))
	for i, block := range msg.Content {
		content[i] = e.normalizeV1ContentBlock(block)
	}
	normalized["content"] = content

	return normalized
}

func (e *AnthropicPromptExtractor) normalizeV1ContentBlock(block anthropic.ContentBlockParamUnion) map[string]interface{} {
	result := make(map[string]interface{})

	switch {
	case block.OfText != nil:
		result["type"] = "text"
		result["text"] = block.OfText.Text
	case block.OfImage != nil:
		result["type"] = "image"
		// Store raw image source data
		img := block.OfImage
		result["source"] = map[string]interface{}{
			"type":       "image_source",
			"media_type": "image",
			"data":       img.Source,
		}
	case block.OfToolUse != nil:
		result["type"] = "tool_use"
		result["id"] = block.OfToolUse.ID
		result["name"] = block.OfToolUse.Name
		result["input"] = block.OfToolUse.Input
	case block.OfToolResult != nil:
		result["type"] = "tool_result"
		result["tool_use_id"] = block.OfToolResult.ToolUseID
		result["content"] = block.OfToolResult.Content
	}
	return result
}

// Helper methods for beta messages

func (e *AnthropicPromptExtractor) extractUserInputFromBeta(messages []anthropic.BetaMessageParam) string {
	for _, msg := range messages {
		if string(msg.Role) == "user" {
			var input string
			for _, block := range msg.Content {
				if block.OfText != nil {
					input += block.OfText.Text + "\n"
				}
			}
			return input
		}
	}
	return ""
}

func (e *AnthropicPromptExtractor) extractRoundResultFromBeta(messages []anthropic.BetaMessageParam) string {
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			var result string
			for _, block := range msg.Content {
				if block.OfText != nil {
					result += block.OfText.Text + "\n"
				}
			}
			return result
		}
	}
	return ""
}

func (e *AnthropicPromptExtractor) normalizeBetaMessages(messages []anthropic.BetaMessageParam) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = e.normalizeBetaMessage(msg)
	}
	return result
}

func (e *AnthropicPromptExtractor) normalizeBetaMessage(msg anthropic.BetaMessageParam) map[string]interface{} {
	normalized := map[string]interface{}{
		"role": string(msg.Role),
	}

	// Normalize content blocks
	content := make([]map[string]interface{}, len(msg.Content))
	for i, block := range msg.Content {
		content[i] = e.normalizeBetaContentBlock(block)
	}
	normalized["content"] = content

	return normalized
}

func (e *AnthropicPromptExtractor) normalizeBetaContentBlock(block anthropic.BetaContentBlockParamUnion) map[string]interface{} {
	result := make(map[string]interface{})

	switch {
	case block.OfText != nil:
		result["type"] = "text"
		result["text"] = block.OfText.Text
	case block.OfImage != nil:
		result["type"] = "image"
		// Store raw image source data
		img := block.OfImage
		result["source"] = map[string]interface{}{
			"type":       "image_source",
			"media_type": "image",
			"data":       img.Source,
		}
	case block.OfToolUse != nil:
		result["type"] = "tool_use"
		result["id"] = block.OfToolUse.ID
		result["name"] = block.OfToolUse.Name
		result["input"] = block.OfToolUse.Input
	case block.OfToolResult != nil:
		result["type"] = "tool_result"
		result["tool_use_id"] = block.OfToolResult.ToolUseID
		result["content"] = block.OfToolResult.Content
	}
	return result
}

// OpenAIPromptExtractor extracts rounds from OpenAI API requests
type OpenAIPromptExtractor struct{}

// GetProtocol returns the protocol type
func (e *OpenAIPromptExtractor) GetProtocol() db.ProtocolType {
	return db.ProtocolOpenAI
}

// ExtractRounds extracts conversation rounds from OpenAI request body
func (e *OpenAIPromptExtractor) ExtractRounds(requestBody []byte) ([]db.RoundData, error) {
	// OpenAI chat completion format
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(requestBody, &req); err != nil {
		return nil, err
	}

	// For OpenAI, we'll group by user messages (similar to Anthropic rounds)
	// This is a simplified implementation
	rounds := make([]db.RoundData, 0)
	currentRound := []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}{}

	for _, msg := range req.Messages {
		if msg.Role == "user" && len(currentRound) > 0 {
			// Save previous round
			rounds = append(rounds, e.createOpenAIRound(currentRound))
			currentRound = []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			}{}
		}
		currentRound = append(currentRound, msg)
	}

	// Add the last round
	if len(currentRound) > 0 {
		rounds = append(rounds, e.createOpenAIRound(currentRound))
	}

	return rounds, nil
}

func (e *OpenAIPromptExtractor) createOpenAIRound(messages []struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}) db.RoundData {
	round := db.RoundData{
		RoundIndex:   0,
		FullMessages: make([]map[string]interface{}, len(messages)),
	}

	for i, msg := range messages {
		round.FullMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}

		if msg.Role == "user" && round.UserInput == "" {
			round.UserInput = e.extractOpenAIContentText(msg.Content)
		}
		if msg.Role == "assistant" && round.RoundResult == "" {
			round.RoundResult = e.extractOpenAIContentText(msg.Content)
		}
	}

	return round
}

func (e *OpenAIPromptExtractor) extractOpenAIContentText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []interface{}:
		var result string
		for _, item := range c {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemType, ok := itemMap["type"].(string); ok {
					if itemType == "text" {
						if text, ok := itemMap["text"].(string); ok {
							result += text + "\n"
						}
					}
				}
			}
		}
		return result
	}
	return ""
}

// ExtractMetadata extracts OpenAI-specific metadata from response headers
func (e *OpenAIPromptExtractor) ExtractMetadata(headers http.Header) (map[string]interface{}, error) {
	metadata := make(map[string]interface{})

	// OpenAI-specific headers
	if orgID := headers.Get("Openai-Organization"); orgID != "" {
		metadata["openai_organization_id"] = orgID
	}

	// Note: OpenAI doesn't natively provide project/session concepts
	// These may be derived from custom headers or request metadata

	return metadata, nil
}

// ExtractProjectID extracts project ID (may be empty for OpenAI)
func (e *OpenAIPromptExtractor) ExtractProjectID(headers http.Header) string {
	// OpenAI doesn't natively provide project ID in response headers
	// Could be derived from custom headers
	return headers.Get("X-Project-ID")
}

// ExtractSessionID extracts session ID (may be empty for OpenAI)
func (e *OpenAIPromptExtractor) ExtractSessionID(headers http.Header) string {
	// OpenAI doesn't natively provide session ID
	// Could be derived from custom headers
	return headers.Get("X-Session-ID")
}

// ExtractRequestID extracts request ID from OpenAI response headers
func (e *OpenAIPromptExtractor) ExtractRequestID(headers http.Header) string {
	return headers.Get("X-Request-Id")
}

// NormalizeMessage converts an OpenAI message to normalized format
func (e *OpenAIPromptExtractor) NormalizeMessage(msg interface{}) (map[string]interface{}, error) {
	if msgMap, ok := msg.(map[string]interface{}); ok {
		return msgMap, nil
	}
	return nil, nil
}

// PromptExtractors registry for protocol handlers
var PromptExtractors = map[db.ProtocolType]MemoryExtractor{
	db.ProtocolAnthropic:    &AnthropicPromptExtractor{},
	db.ProtocolOpenAI:       &OpenAIPromptExtractor{},
	db.ProtocolOpenAICompat: &OpenAIPromptExtractor{}, // Reuse OpenAI extractor for compatible APIs
	// Google extractor to be added in the future
}
