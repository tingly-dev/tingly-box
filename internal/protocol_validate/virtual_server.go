package protocol_validate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// VirtualServer is a mock provider server that speaks OpenAI, Anthropic, and Google
// response formats. It acts as a deterministic "virtual model" — scenario responses
// are pre-configured and returned without any real model calls.
//
// Conceptually equivalent to the project's existing "virtual model" concept:
// a virtual server that impersonates a real provider for testing purposes.
// Future integration with existing virtual model infrastructure is planned.
type VirtualServer struct {
	server    *httptest.Server
	scenarios map[string]Scenario // keyed by scenario name

	mu        sync.RWMutex
	callCount int
}

// NewVirtualServer creates a new VirtualServer and registers cleanup with t.
func NewVirtualServer(t *testing.T) *VirtualServer {
	t.Helper()
	vs := &VirtualServer{
		scenarios: make(map[string]Scenario),
	}

	mux := http.NewServeMux()
	// OpenAI endpoints
	mux.HandleFunc("/v1/chat/completions", vs.handleOpenAIChat)
	mux.HandleFunc("/v1/responses", vs.handleOpenAIResponses)
	// Anthropic endpoints
	mux.HandleFunc("/v1/messages", vs.handleAnthropicMessages)
	// Google endpoints
	mux.HandleFunc("/", vs.handleGoogle) // catches /v1beta/models/*/generateContent

	vs.server = httptest.NewServer(mux)
	t.Cleanup(vs.Close)
	return vs
}

// RegisterScenario registers a scenario so the virtual server can serve its mock responses.
func (vs *VirtualServer) RegisterScenario(s Scenario) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.scenarios[s.Name] = s
}

// URL returns the base URL of the virtual server.
func (vs *VirtualServer) URL() string {
	return vs.server.URL
}

// Close shuts down the virtual server.
func (vs *VirtualServer) Close() {
	vs.server.Close()
}

// CallCount returns the total number of requests received.
func (vs *VirtualServer) CallCount() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.callCount
}

// ─── Direct send helpers (for virtual_server_test.go) ─────────────────────────

// SendOpenAIChat sends a request directly to the virtual server's OpenAI endpoint
// and returns a RoundTripResult. Used in tests that exercise the virtual server in isolation.
func (vs *VirtualServer) SendOpenAIChat(t *testing.T, s Scenario, streaming bool) *RoundTripResult {
	t.Helper()
	vs.RegisterScenario(s)

	body := map[string]interface{}{
		"model":    "gpt-4o",
		"messages": []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":   streaming,
	}
	return vs.doRequest(t, "POST", vs.server.URL+"/v1/chat/completions", body, streaming, StyleOpenAI)
}

// SendAnthropicV1 sends a request directly to the virtual server's Anthropic endpoint.
func (vs *VirtualServer) SendAnthropicV1(t *testing.T, s Scenario, streaming bool) *RoundTripResult {
	t.Helper()
	vs.RegisterScenario(s)

	body := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":     streaming,
	}
	return vs.doRequest(t, "POST", vs.server.URL+"/v1/messages", body, streaming, StyleAnthropic)
}

// SendGoogle sends a request directly to the virtual server's Google endpoint.
func (vs *VirtualServer) SendGoogle(t *testing.T, s Scenario, streaming bool) *RoundTripResult {
	t.Helper()
	vs.RegisterScenario(s)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"role": "user", "parts": []map[string]string{{"text": "What is the capital of France?"}}},
		},
	}
	suffix := "generateContent"
	if streaming {
		suffix = "streamGenerateContent"
	}
	return vs.doRequest(t, "POST", vs.server.URL+"/v1beta/models/gemini-2.0-flash/"+suffix, body, streaming, StyleGoogle)
}

// ─── HTTP handlers ─────────────────────────────────────────────────────────────

func (vs *VirtualServer) handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(r)

	resp, ok := vs.scenarios[scenario]
	if !ok {
		// Fall back to first registered scenario
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[StyleOpenAI]
	if !ok {
		http.Error(w, "no openai mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	if streaming {
		vs.writeSSE(w, builder.Stream())
	} else {
		status, body := builder.NonStream()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}
}

func (vs *VirtualServer) handleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	// OpenAI Responses API — serve same OpenAI-format response
	vs.handleOpenAIChat(w, r)
}

func (vs *VirtualServer) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(r)

	resp, ok := vs.scenarios[scenario]
	if !ok {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[StyleAnthropic]
	if !ok {
		http.Error(w, "no anthropic mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	if streaming {
		vs.writeSSE(w, builder.Stream())
	} else {
		status, body := builder.NonStream()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}
}

func (vs *VirtualServer) handleGoogle(w http.ResponseWriter, r *http.Request) {
	vs.mu.Lock()
	vs.callCount++
	vs.mu.Unlock()

	streaming := strings.Contains(r.URL.Path, "streamGenerateContent")
	scenario := vs.detectScenario(r)

	resp, ok := vs.scenarios[scenario]
	if !ok {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[StyleGoogle]
	if !ok {
		http.Error(w, "no google mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	if streaming {
		vs.writeSSE(w, builder.Stream())
	} else {
		status, body := builder.NonStream()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}
}

// ─── SSE writer ───────────────────────────────────────────────────────────────

func (vs *VirtualServer) writeSSE(w http.ResponseWriter, events []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Group lines into SSE messages: a message ends after a "data:" line.
	// Lines starting with "event:" are headers for the next data line.
	// This matches the SSE spec: each message is separated by a blank line.
	i := 0
	for i < len(events) {
		line := events[i]
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "event: ") {
			// Write event: line + next data: line as one SSE message
			fmt.Fprintf(w, "%s\n", line)
			i++
			if i < len(events) && (strings.HasPrefix(events[i], "data:") || strings.HasPrefix(events[i], "data: ")) {
				fmt.Fprintf(w, "%s\n\n", events[i])
				i++
			} else {
				fmt.Fprintf(w, "\n")
			}
		} else {
			// Standalone data: line or other (e.g. "data: [DONE]")
			fmt.Fprintf(w, "%s\n\n", line)
			i++
		}
		flusher.Flush()
		time.Sleep(5 * time.Millisecond)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (vs *VirtualServer) parseStreamFlagFromBytes(body []byte) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	stream, _ := m["stream"].(bool)
	return stream
}

// parseStreamFlag is kept for the Google handler which reads streaming from the URL.
func (vs *VirtualServer) parseStreamFlag(r *http.Request) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	stream, _ := m["stream"].(bool)
	return stream
}

// detectScenario tries to infer the scenario from request headers/body.
// Currently falls back to the first registered scenario.
func (vs *VirtualServer) detectScenario(_ *http.Request) string {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	for name := range vs.scenarios {
		return name
	}
	return ""
}

func (vs *VirtualServer) firstScenario() Scenario {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	for _, s := range vs.scenarios {
		return s
	}
	return Scenario{}
}

// ─── Result parsing ───────────────────────────────────────────────────────────

// doRequest sends an HTTP request and returns a parsed RoundTripResult.
func (vs *VirtualServer) doRequest(t *testing.T, method, url string, body interface{}, streaming bool, style APIStyle) *RoundTripResult {
	t.Helper()

	reqBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequest(method, url, strings.NewReader(string(reqBody)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	result := &RoundTripResult{
		HTTPStatus:  resp.StatusCode,
		IsStreaming: streaming,
	}

	if streaming {
		result.StreamEvents, result.RawBody = readSSEEvents(resp.Body)
		parseStreamedResult(result, style)
	} else {
		result.RawBody, _ = io.ReadAll(resp.Body)
		parseJSONResult(result, style)
	}

	return result
}

// readSSEEvents reads SSE events from a response body.
func readSSEEvents(r io.Reader) (events []string, raw []byte) {
	scanner := bufio.NewScanner(r)
	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			events = append(events, line)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	return events, []byte(sb.String())
}

// parseJSONResult extracts semantic fields from a non-streaming JSON response.
func parseJSONResult(result *RoundTripResult, style APIStyle) {
	var m map[string]interface{}
	if err := json.Unmarshal(result.RawBody, &m); err != nil {
		return
	}

	switch style {
	case StyleOpenAI:
		// Try Responses API format first (has "output" array), fall back to Chat format
		if _, hasOutput := m["output"]; hasOutput {
			parseOpenAIResponsesResult(result, m)
		} else {
			parseOpenAIResult(result, m)
		}
	case StyleAnthropic:
		parseAnthropicResult(result, m)
	case StyleGoogle:
		parseGoogleResult(result, m)
	}
}

func parseOpenAIResponsesResult(result *RoundTripResult, m map[string]interface{}) {
	if model, ok := m["model"].(string); ok {
		result.Model = model
	}
	if status, ok := m["status"].(string); ok {
		result.FinishReason = status
	}
	output, _ := m["output"].([]interface{})
	for _, item := range output {
		iMap, _ := item.(map[string]interface{})
		if iMap == nil {
			continue
		}
		switch iMap["type"] {
		case "message":
			// Nested message format: {type:"message", role:"assistant", content:[...]}
			result.Role, _ = iMap["role"].(string)
			content, _ := iMap["content"].([]interface{})
			for _, c := range content {
				cMap, _ := c.(map[string]interface{})
				if cMap == nil {
					continue
				}
				if cMap["type"] == "output_text" {
					result.Content, _ = cMap["text"].(string)
				}
			}
		case "output_text":
			// Flat format: {type:"output_text", text:"..."}
			result.Role = "assistant"
			result.Content, _ = iMap["text"].(string)
		case "function_call", "custom_tool_call", "mcp_call":
			id, _ := iMap["id"].(string)
			callID, _ := iMap["call_id"].(string)
			if id == "" {
				id = callID
			}
			name, _ := iMap["name"].(string)
			args, _ := iMap["arguments"].(string)
			result.ToolCalls = append(result.ToolCalls, ToolCallResult{ID: id, Name: name, Arguments: args})
		}
	}
	if usage, ok := m["usage"].(map[string]interface{}); ok {
		result.Usage = &TokenUsage{
			InputTokens:  toInt(usage["input_tokens"]),
			OutputTokens: toInt(usage["output_tokens"]),
		}
	}
}

func parseOpenAIResult(result *RoundTripResult, m map[string]interface{}) {
	if model, ok := m["model"].(string); ok {
		result.Model = model
	}
	choices, _ := m["choices"].([]interface{})
	if len(choices) == 0 {
		return
	}
	choice, _ := choices[0].(map[string]interface{})
	if fr, ok := choice["finish_reason"].(string); ok {
		result.FinishReason = fr
	}
	msg, _ := choice["message"].(map[string]interface{})
	if msg == nil {
		return
	}
	if role, ok := msg["role"].(string); ok {
		result.Role = role
	}
	if content, ok := msg["content"].(string); ok {
		result.Content = content
	}
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
			result.ToolCalls = append(result.ToolCalls, ToolCallResult{ID: id, Name: name, Arguments: args})
		}
	}
	if usage, ok := m["usage"].(map[string]interface{}); ok {
		result.Usage = &TokenUsage{
			InputTokens:  toInt(usage["prompt_tokens"]),
			OutputTokens: toInt(usage["completion_tokens"]),
		}
	}
}

func parseAnthropicResult(result *RoundTripResult, m map[string]interface{}) {
	if model, ok := m["model"].(string); ok {
		result.Model = model
	}
	result.Role, _ = m["role"].(string)
	if stopReason, ok := m["stop_reason"].(string); ok {
		// Normalize Anthropic stop_reason to match test expectations
		switch stopReason {
		case "end_turn":
			result.FinishReason = "end_turn"
		case "tool_use":
			result.FinishReason = "tool_use"
		default:
			result.FinishReason = stopReason
		}
	}
	content, _ := m["content"].([]interface{})
	for _, block := range content {
		bMap, _ := block.(map[string]interface{})
		if bMap == nil {
			continue
		}
		switch bMap["type"] {
		case "text":
			result.Content, _ = bMap["text"].(string)
		case "thinking":
			result.ThinkingContent, _ = bMap["thinking"].(string)
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
			result.ToolCalls = append(result.ToolCalls, ToolCallResult{ID: id, Name: name, Arguments: args})
		}
	}
	if usage, ok := m["usage"].(map[string]interface{}); ok {
		result.Usage = &TokenUsage{
			InputTokens:  toInt(usage["input_tokens"]),
			OutputTokens: toInt(usage["output_tokens"]),
		}
	}
}

func parseGoogleResult(result *RoundTripResult, m map[string]interface{}) {
	candidates, _ := m["candidates"].([]interface{})
	if len(candidates) == 0 {
		return
	}
	cand, _ := candidates[0].(map[string]interface{})
	if fr, ok := cand["finishReason"].(string); ok {
		result.FinishReason = fr
	}
	content, _ := cand["content"].(map[string]interface{})
	if content == nil {
		return
	}
	result.Role = "assistant" // Google uses "model" role internally
	parts, _ := content["parts"].([]interface{})
	for _, part := range parts {
		pMap, _ := part.(map[string]interface{})
		if pMap == nil {
			continue
		}
		if text, ok := pMap["text"].(string); ok {
			result.Content = text
		}
		if fc, ok := pMap["functionCall"].(map[string]interface{}); ok {
			name, _ := fc["name"].(string)
			args, _ := fc["args"].(map[string]interface{})
			argsJSON, _ := json.Marshal(args)
			result.ToolCalls = append(result.ToolCalls, ToolCallResult{Name: name, Arguments: string(argsJSON)})
		}
	}
	if usage, ok := m["usageMetadata"].(map[string]interface{}); ok {
		result.Usage = &TokenUsage{
			InputTokens:  toInt(usage["promptTokenCount"]),
			OutputTokens: toInt(usage["candidatesTokenCount"]),
		}
	}
}

// parseStreamedResult assembles content from SSE events into result fields.
func parseStreamedResult(result *RoundTripResult, style APIStyle) {
	switch style {
	case StyleOpenAI:
		assembleOpenAIStream(result)
	case StyleAnthropic:
		assembleAnthropicStream(result)
	case StyleGoogle:
		assembleGoogleStream(result)
	}
}

func assembleOpenAIStream(result *RoundTripResult) {
	// Detect Responses API streaming format by checking event types
	for _, event := range result.StreamEvents {
		if data, ok := sseDataPayload(event); ok {
			if strings.Contains(data, "response.output_text.delta") ||
				strings.Contains(data, `"object":"response"`) {
				assembleOpenAIResponsesStream(result)
				return
			}
			if data != "[DONE]" {
				break // use Chat format
			}
		}
	}

	result.Role = "assistant"
	var content strings.Builder
	for _, event := range result.StreamEvents {
		data, isData := sseDataPayload(event)
		if !isData {
			continue
		}
		// data already extracted by sseDataPayload
		if data == "[DONE]" {
			break
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		if model, ok := m["model"].(string); ok && result.Model == "" {
			result.Model = model
		}
		choices, _ := m["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]interface{})
		if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
			result.FinishReason = fr
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
				// New tool call or append to existing
				idx := toInt(tcMap["index"])
				for len(result.ToolCalls) <= idx {
					result.ToolCalls = append(result.ToolCalls, ToolCallResult{})
				}
				if id != "" {
					result.ToolCalls[idx].ID = id
				}
				if name != "" {
					result.ToolCalls[idx].Name = name
				}
				result.ToolCalls[idx].Arguments += args
			}
		}
	}
	result.Content = content.String()
}

func assembleAnthropicStream(result *RoundTripResult) {
	result.Role = "assistant"
	var content strings.Builder
	var thinking strings.Builder
	inThinking := false

	for _, event := range result.StreamEvents {
		data, isData := sseDataPayload(event)
		if !isData {
			continue
		}
		// data already extracted by sseDataPayload
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		switch m["type"] {
		case "message_start":
			msg, _ := m["message"].(map[string]interface{})
			if msg != nil {
				result.Model, _ = msg["model"].(string)
			}
		case "content_block_start":
			cb, _ := m["content_block"].(map[string]interface{})
			if cb != nil {
				inThinking = cb["type"] == "thinking"
				if cb["type"] == "tool_use" {
					id, _ := cb["id"].(string)
					name, _ := cb["name"].(string)
					result.ToolCalls = append(result.ToolCalls, ToolCallResult{ID: id, Name: name})
				}
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
				// Tool call argument delta — append to last tool call
				if partial, ok := delta["partial_json"].(string); ok && len(result.ToolCalls) > 0 {
					result.ToolCalls[len(result.ToolCalls)-1].Arguments += partial
				}
			}
		case "content_block_stop":
			inThinking = false
		case "message_delta":
			delta, _ := m["delta"].(map[string]interface{})
			if delta != nil {
				result.FinishReason, _ = delta["stop_reason"].(string)
			}
		}
		_ = inThinking
	}
	result.Content = content.String()
	result.ThinkingContent = thinking.String()
}

func assembleGoogleStream(result *RoundTripResult) {
	result.Role = "assistant"
	var content strings.Builder
	for _, event := range result.StreamEvents {
		data, isData := sseDataPayload(event)
		if !isData {
			continue
		}
		// data already extracted by sseDataPayload
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
	result.Content = content.String()
}

func assembleOpenAIResponsesStream(result *RoundTripResult) {
	result.Role = "assistant"
	var content strings.Builder
	for _, event := range result.StreamEvents {
		data, isData := sseDataPayload(event)
		if !isData || data == "[DONE]" {
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
			if delta, ok := m["delta"].(string); ok && len(result.ToolCalls) > 0 {
				result.ToolCalls[len(result.ToolCalls)-1].Arguments += delta
			}
		case "response.output_item.added":
			item, _ := m["item"].(map[string]interface{})
			if item != nil && item["type"] == "function_call" {
				id, _ := item["id"].(string)
				name, _ := item["name"].(string)
				result.ToolCalls = append(result.ToolCalls, ToolCallResult{ID: id, Name: name})
			}
		case "response.completed":
			resp, _ := m["response"].(map[string]interface{})
			if resp != nil {
				if usage, ok := resp["usage"].(map[string]interface{}); ok {
					result.Usage = &TokenUsage{
						InputTokens:  toInt(usage["input_tokens"]),
						OutputTokens: toInt(usage["output_tokens"]),
					}
				}
			}
		}
	}
	result.Content = content.String()
}

// sseDataPayload checks if a line is a SSE data line ("data:" or "data: " prefix)
// and returns the payload. Handles both Gin-style ("data:") and standard ("data: ").
func sseDataPayload(line string) (payload string, ok bool) {
	if strings.HasPrefix(line, "data: ") {
		return strings.TrimPrefix(line, "data: "), true
	}
	if strings.HasPrefix(line, "data:") {
		return strings.TrimPrefix(line, "data:"), true
	}
	return "", false
}

func toInt(v interface{}) int {
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
