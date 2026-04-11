package sse

import (
	"encoding/json"
	"strings"
)

// ParsedTokenUsage holds token counts extracted from a parsed response.
type ParsedTokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// ParsedToolCall holds a single tool/function call extracted from a response.
type ParsedToolCall struct {
	ID        string
	Name      string
	Arguments string // raw JSON string
}

// ParsedResult is a protocol-agnostic view of an AI response, extracted from raw
// JSON (non-streaming) or SSE event lines (streaming). Used by test infrastructure
// and virtual model servers to inspect gateway responses without importing SDK types.
type ParsedResult struct {
	Role            string
	Content         string
	Model           string
	FinishReason    string
	ThinkingContent string
	ToolCalls       []ParsedToolCall
	Usage           *ParsedTokenUsage
}

// ─── Non-streaming parsers ────────────────────────────────────────────────────

// ParseOpenAIChatResult extracts fields from an OpenAI Chat Completion response
// (map[string]interface{} already JSON-decoded).
func ParseOpenAIChatResult(m map[string]interface{}) *ParsedResult {
	r := &ParsedResult{}
	if model, ok := m["model"].(string); ok {
		r.Model = model
	}
	choices, _ := m["choices"].([]interface{})
	if len(choices) == 0 {
		return r
	}
	choice, _ := choices[0].(map[string]interface{})
	if fr, ok := choice["finish_reason"].(string); ok {
		r.FinishReason = fr
	}
	msg, _ := choice["message"].(map[string]interface{})
	if msg != nil {
		r.Role, _ = msg["role"].(string)
		r.Content, _ = msg["content"].(string)
		if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
			for _, tc := range toolCalls {
				tcMap, _ := tc.(map[string]interface{})
				if tcMap == nil {
					continue
				}
				id, _ := tcMap["id"].(string)
				fn, _ := tcMap["function"].(map[string]interface{})
				name, _ := fn["name"].(string)
				args, _ := fn["arguments"].(string)
				r.ToolCalls = append(r.ToolCalls, ParsedToolCall{ID: id, Name: name, Arguments: args})
			}
		}
	}
	if usage, ok := m["usage"].(map[string]interface{}); ok {
		r.Usage = &ParsedTokenUsage{
			InputTokens:  parsedToInt(usage["prompt_tokens"]),
			OutputTokens: parsedToInt(usage["completion_tokens"]),
		}
	}
	return r
}

// ParseOpenAIResponsesResult extracts fields from an OpenAI Responses API response.
func ParseOpenAIResponsesResult(m map[string]interface{}) *ParsedResult {
	r := &ParsedResult{}
	if model, ok := m["model"].(string); ok {
		r.Model = model
	}
	if status, ok := m["status"].(string); ok {
		r.FinishReason = status
	}
	output, _ := m["output"].([]interface{})
	for _, item := range output {
		iMap, _ := item.(map[string]interface{})
		if iMap == nil {
			continue
		}
		switch iMap["type"] {
		case "message":
			r.Role, _ = iMap["role"].(string)
			content, _ := iMap["content"].([]interface{})
			for _, c := range content {
				cMap, _ := c.(map[string]interface{})
				if cMap != nil && cMap["type"] == "output_text" {
					r.Content, _ = cMap["text"].(string)
				}
			}
		case "output_text":
			r.Role = "assistant"
			r.Content, _ = iMap["text"].(string)
		case "function_call", "custom_tool_call", "mcp_call":
			id, _ := iMap["id"].(string)
			callID, _ := iMap["call_id"].(string)
			if id == "" {
				id = callID
			}
			name, _ := iMap["name"].(string)
			args, _ := iMap["arguments"].(string)
			r.ToolCalls = append(r.ToolCalls, ParsedToolCall{ID: id, Name: name, Arguments: args})
		}
	}
	if usage, ok := m["usage"].(map[string]interface{}); ok {
		r.Usage = &ParsedTokenUsage{
			InputTokens:  parsedToInt(usage["input_tokens"]),
			OutputTokens: parsedToInt(usage["output_tokens"]),
		}
	}
	return r
}

// ParseAnthropicResult extracts fields from an Anthropic Messages API response.
func ParseAnthropicResult(m map[string]interface{}) *ParsedResult {
	r := &ParsedResult{}
	if model, ok := m["model"].(string); ok {
		r.Model = model
	}
	r.Role, _ = m["role"].(string)
	r.FinishReason, _ = m["stop_reason"].(string)
	content, _ := m["content"].([]interface{})
	for _, block := range content {
		bMap, _ := block.(map[string]interface{})
		if bMap == nil {
			continue
		}
		switch bMap["type"] {
		case "text":
			r.Content, _ = bMap["text"].(string)
		case "thinking":
			r.ThinkingContent, _ = bMap["thinking"].(string)
		case "tool_use":
			id, _ := bMap["id"].(string)
			name, _ := bMap["name"].(string)
			var args string
			switch v := bMap["input"].(type) {
			case map[string]interface{}:
				b, _ := json.Marshal(v)
				args = string(b)
			case string:
				args = v
			}
			r.ToolCalls = append(r.ToolCalls, ParsedToolCall{ID: id, Name: name, Arguments: args})
		}
	}
	if usage, ok := m["usage"].(map[string]interface{}); ok {
		r.Usage = &ParsedTokenUsage{
			InputTokens:  parsedToInt(usage["input_tokens"]),
			OutputTokens: parsedToInt(usage["output_tokens"]),
		}
	}
	return r
}

// ParseGoogleResult extracts fields from a Google GenerateContent response.
func ParseGoogleResult(m map[string]interface{}) *ParsedResult {
	r := &ParsedResult{}
	candidates, _ := m["candidates"].([]interface{})
	if len(candidates) == 0 {
		return r
	}
	cand, _ := candidates[0].(map[string]interface{})
	r.FinishReason, _ = cand["finishReason"].(string)
	content, _ := cand["content"].(map[string]interface{})
	if content == nil {
		return r
	}
	r.Role = "assistant"
	parts, _ := content["parts"].([]interface{})
	for _, part := range parts {
		pMap, _ := part.(map[string]interface{})
		if pMap == nil {
			continue
		}
		if text, ok := pMap["text"].(string); ok {
			r.Content = text
		}
		if fc, ok := pMap["functionCall"].(map[string]interface{}); ok {
			name, _ := fc["name"].(string)
			args, _ := fc["args"].(map[string]interface{})
			argsJSON, _ := json.Marshal(args)
			r.ToolCalls = append(r.ToolCalls, ParsedToolCall{Name: name, Arguments: string(argsJSON)})
		}
	}
	if usage, ok := m["usageMetadata"].(map[string]interface{}); ok {
		r.Usage = &ParsedTokenUsage{
			InputTokens:  parsedToInt(usage["promptTokenCount"]),
			OutputTokens: parsedToInt(usage["candidatesTokenCount"]),
		}
	}
	return r
}

// ─── Streaming assemblers ─────────────────────────────────────────────────────

// AssembleOpenAIChatStream assembles a ParsedResult from OpenAI Chat SSE event lines.
func AssembleOpenAIChatStream(events []string) *ParsedResult {
	r := &ParsedResult{Role: "assistant"}
	var content strings.Builder
	for _, event := range events {
		data, ok := ParseSSEDataPayload(event)
		if !ok || data == "[DONE]" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		if model, ok := m["model"].(string); ok && r.Model == "" {
			r.Model = model
		}
		choices, _ := m["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]interface{})
		if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
			r.FinishReason = fr
		}
		delta, _ := choice["delta"].(map[string]interface{})
		if delta == nil {
			continue
		}
		if c, ok := delta["content"].(string); ok {
			content.WriteString(c)
		}
		if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range toolCalls {
				tcMap, _ := tc.(map[string]interface{})
				fn, _ := tcMap["function"].(map[string]interface{})
				id, _ := tcMap["id"].(string)
				name, _ := fn["name"].(string)
				args, _ := fn["arguments"].(string)
				idx := parsedToInt(tcMap["index"])
				for len(r.ToolCalls) <= idx {
					r.ToolCalls = append(r.ToolCalls, ParsedToolCall{})
				}
				if id != "" {
					r.ToolCalls[idx].ID = id
				}
				if name != "" {
					r.ToolCalls[idx].Name = name
				}
				r.ToolCalls[idx].Arguments += args
			}
		}
	}
	r.Content = content.String()
	return r
}

// AssembleOpenAIResponsesStream assembles a ParsedResult from OpenAI Responses API SSE event lines.
func AssembleOpenAIResponsesStream(events []string) *ParsedResult {
	r := &ParsedResult{Role: "assistant"}
	var content strings.Builder
	for _, event := range events {
		data, ok := ParseSSEDataPayload(event)
		if !ok || data == "[DONE]" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		evtType, _ := m["type"].(string)
		switch evtType {
		case "response.output_text.delta":
			if delta, ok := m["delta"].(string); ok {
				content.WriteString(delta)
			}
		case "response.function_call_arguments.delta":
			if delta, ok := m["delta"].(string); ok && len(r.ToolCalls) > 0 {
				r.ToolCalls[len(r.ToolCalls)-1].Arguments += delta
			}
		case "response.output_item.added":
			item, _ := m["item"].(map[string]interface{})
			if item != nil && item["type"] == "function_call" {
				id, _ := item["id"].(string)
				name, _ := item["name"].(string)
				r.ToolCalls = append(r.ToolCalls, ParsedToolCall{ID: id, Name: name})
			}
		case "response.completed":
			resp, _ := m["response"].(map[string]interface{})
			if resp != nil {
				if usage, ok := resp["usage"].(map[string]interface{}); ok {
					r.Usage = &ParsedTokenUsage{
						InputTokens:  parsedToInt(usage["input_tokens"]),
						OutputTokens: parsedToInt(usage["output_tokens"]),
					}
				}
			}
		}
	}
	r.Content = content.String()
	return r
}

// AssembleAnthropicStream assembles a ParsedResult from Anthropic SSE event lines.
func AssembleAnthropicStream(events []string) *ParsedResult {
	r := &ParsedResult{Role: "assistant"}
	var content strings.Builder
	var thinking strings.Builder
	for _, event := range events {
		data, ok := ParseSSEDataPayload(event)
		if !ok {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		switch m["type"] {
		case "message_start":
			msg, _ := m["message"].(map[string]interface{})
			if msg != nil {
				r.Model, _ = msg["model"].(string)
			}
		case "content_block_start":
			cb, _ := m["content_block"].(map[string]interface{})
			if cb != nil && cb["type"] == "tool_use" {
				id, _ := cb["id"].(string)
				name, _ := cb["name"].(string)
				r.ToolCalls = append(r.ToolCalls, ParsedToolCall{ID: id, Name: name})
			}
		case "content_block_delta":
			delta, _ := m["delta"].(map[string]interface{})
			if delta == nil {
				continue
			}
			switch delta["type"] {
			case "text_delta":
				if text, ok := delta["text"].(string); ok {
					content.WriteString(text)
				}
			case "thinking_delta":
				if text, ok := delta["thinking"].(string); ok {
					thinking.WriteString(text)
				}
			case "input_json_delta":
				if partial, ok := delta["partial_json"].(string); ok && len(r.ToolCalls) > 0 {
					r.ToolCalls[len(r.ToolCalls)-1].Arguments += partial
				}
			}
		case "message_delta":
			delta, _ := m["delta"].(map[string]interface{})
			if delta != nil {
				r.FinishReason, _ = delta["stop_reason"].(string)
			}
		}
	}
	r.Content = content.String()
	r.ThinkingContent = thinking.String()
	return r
}

// AssembleGoogleStream assembles a ParsedResult from Google GenerateContent SSE event lines.
func AssembleGoogleStream(events []string) *ParsedResult {
	r := &ParsedResult{Role: "assistant"}
	var content strings.Builder
	for _, event := range events {
		data, ok := ParseSSEDataPayload(event)
		if !ok {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		candidates, _ := m["candidates"].([]interface{})
		if len(candidates) == 0 {
			continue
		}
		cand, _ := candidates[0].(map[string]interface{})
		c, _ := cand["content"].(map[string]interface{})
		if c == nil {
			continue
		}
		parts, _ := c["parts"].([]interface{})
		for _, part := range parts {
			pMap, _ := part.(map[string]interface{})
			if text, ok := pMap["text"].(string); ok {
				content.WriteString(text)
			}
		}
	}
	r.Content = content.String()
	return r
}

// AssembleOpenAIStream auto-detects whether events are Responses API or Chat format
// and delegates accordingly.
func AssembleOpenAIStream(events []string) *ParsedResult {
	for _, event := range events {
		if data, ok := ParseSSEDataPayload(event); ok {
			if strings.Contains(data, "response.output_text.delta") ||
				strings.Contains(data, `"object":"response"`) {
				return AssembleOpenAIResponsesStream(events)
			}
			if data != "[DONE]" {
				break
			}
		}
	}
	return AssembleOpenAIChatStream(events)
}

// ─── internal helper ──────────────────────────────────────────────────────────

// parsedToInt converts a JSON-decoded number (float64) or int to int.
func parsedToInt(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	}
	return 0
}
