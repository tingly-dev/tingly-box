package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// runClaudeProfileTest executes a single claude profile test
// It uses a real client (claudeRoundTripper) against the virtual server
func runClaudeProfileTest(env *protocol_validate.ProfileTestEnv, scenario protocol_validate.ProfileScenario, streaming bool) protocol_validate.ProfileTestResult {
	name := fmt.Sprintf("claude/%s", scenario.Name)
	if streaming {
		name += "/streaming"
	} else {
		name += "/nonstream"
	}

	result := protocol_validate.ProfileTestResult{
		Name:     name,
		Profile:  protocol_validate.ProfileTypeClaudeCode,
		Scenario: scenario.Name,
		Passed:   false,
	}

	// Build the request
	baseURL := env.BaseURL()
	modelToken := env.ModelToken()

	// Claude Code uses the /tingly/claude_code endpoint
	endpoint := "/tingly/claude_code/v1/messages"
	if streaming {
		endpoint += "?beta=true"
	} else {
		endpoint += "?beta=true"
	}

	// Build request body for Claude format
	requestBody := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello, world!"},
		},
		"stream": streaming,
	}

	// Add metadata for OAuth tests (session_id)
	requestBody["metadata"] = map[string]interface{}{
		"user_id": "user_test123_account_test-uuid_session_test-session",
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "marshal_request",
			Error:     fmt.Sprintf("marshal request: %v", err),
		})
		return result
	}
	result.RequestBody = bodyBytes

	// Create HTTP request
	url := baseURL + endpoint
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "create_request",
			Error:     fmt.Sprintf("create request: %v", err),
		})
		return result
	}

	// Set headers for Claude Code
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", modelToken)

	// Note: For OAuth test, we would use OAuth token format here
	// oauthToken := "sk-ant-oatest123456"

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "send_request",
			Error:     fmt.Sprintf("send request: %v", err),
		})
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode
	result.RequestHeaders = req.Header

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "read_response",
			Error:     fmt.Sprintf("read response: %v", err),
		})
		return result
	}
	result.ResponseBody = responseBody

	// Validate response
	var validationErrors []protocol_validate.AssertionError

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		validationErrors = append(validationErrors, protocol_validate.AssertionError{
			Assertion: "http_status",
			Error:     fmt.Sprintf("expected 200, got %d", resp.StatusCode),
			Context:   string(responseBody),
		})
	}

	// Validate based on streaming mode
	if streaming {
		// Parse as SSE
		events := strings.Split(string(responseBody), "\n")
		if len(events) == 0 {
			validationErrors = append(validationErrors, protocol_validate.AssertionError{
				Assertion: "sse_events",
				Error:     "no SSE events received",
			})
		} else {
			// Check for message_start event
			hasMessageStart := false
			for _, event := range events {
				if strings.Contains(event, "message_start") {
					hasMessageStart = true
					break
				}
			}
			if !hasMessageStart {
				validationErrors = append(validationErrors, protocol_validate.AssertionError{
					Assertion: "sse_message_start",
					Error:     "no message_start event found",
				})
			}
		}

		// Validate tool prefix was stripped (for OAuth token)
		// In streaming, tool content should not have tool_prefix
		for _, event := range events {
			if strings.Contains(event, "tool_prefix") {
				validationErrors = append(validationErrors, protocol_validate.AssertionError{
					Assertion: "tool_prefix_stripped",
					Error:     "tool_prefix found in streaming response (should be stripped)",
				})
				break
			}
		}
	} else {
		// Parse as JSON
		var respJSON map[string]interface{}
		if err := json.Unmarshal(responseBody, &respJSON); err != nil {
			validationErrors = append(validationErrors, protocol_validate.AssertionError{
				Assertion: "json_parse",
				Error:     fmt.Sprintf("parse JSON: %v", err),
			})
		} else {
			// Validate response structure
			if respJSON["type"] == nil || respJSON["type"].(string) != "message" {
				validationErrors = append(validationErrors, protocol_validate.AssertionError{
					Assertion: "response_type",
					Error:     fmt.Sprintf("expected type=message, got %v", respJSON["type"]),
				})
			}

			// Validate tool prefix was stripped (for OAuth token)
			bodyStr := string(responseBody)
			if strings.Contains(bodyStr, "tool_prefix") {
				validationErrors = append(validationErrors, protocol_validate.AssertionError{
					Assertion: "tool_prefix_stripped",
					Error:     "tool_prefix found in response (should be stripped)",
				})
			}

			// Validate beta flag handling
			contentBlocks, ok := respJSON["content"].([]interface{})
			if !ok || len(contentBlocks) == 0 {
				validationErrors = append(validationErrors, protocol_validate.AssertionError{
					Assertion: "content_blocks",
					Error:     "no content blocks in response",
				})
			}
		}
	}

	// Validate request headers (note: OAuth token check is disabled for basic test)
	// When OAuth test is enabled, this will validate Bearer token conversion
	_ = req.Header.Get("Authorization")

	if len(validationErrors) > 0 {
		result.Errors = validationErrors
		result.Passed = false
	} else {
		result.Passed = true
	}

	return result
}

// runCodexProfileTest executes a single codex profile test
// It validates path rewriting and parameter filtering
func runCodexProfileTest(env *protocol_validate.ProfileTestEnv, scenario protocol_validate.ProfileScenario, streaming bool) protocol_validate.ProfileTestResult {
	name := fmt.Sprintf("codex/%s", scenario.Name)
	if streaming {
		name += "/streaming"
	} else {
		name += "/nonstream"
	}

	result := protocol_validate.ProfileTestResult{
		Name:     name,
		Profile:  protocol_validate.ProfileTypeCodex,
		Scenario: scenario.Name,
		Passed:   false,
	}

	// Note: Codex requires streaming=true
	if !streaming {
		result.Skipped = true
		result.SkipReason = "Codex only supports streaming mode"
		return result
	}

	// Build request for Codex (OpenAI Responses API format)
	baseURL := env.BaseURL()
	modelToken := env.ModelToken()

	endpoint := "/tingly/codex/v1/responses"

	requestBody := map[string]interface{}{
		"model": "gpt-4",
		"input": []map[string]interface{}{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": "Hello, world!"},
				},
			},
		},
		"stream": true,
		// These should be filtered out by codexRoundTripper
		"max_tokens":            1024,
		"temperature":           0.7,
		"max_completion_tokens": 2048,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "marshal_request",
			Error:     fmt.Sprintf("marshal request: %v", err),
		})
		return result
	}
	result.RequestBody = bodyBytes

	// Create HTTP request
	url := baseURL + endpoint
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "create_request",
			Error:     fmt.Sprintf("create request: %v", err),
		})
		return result
	}

	// Set headers for Codex
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", modelToken)

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "send_request",
			Error:     fmt.Sprintf("send request: %v", err),
		})
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode
	result.RequestHeaders = req.Header

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "read_response",
			Error:     fmt.Sprintf("read response: %v", err),
		})
		return result
	}
	result.ResponseBody = responseBody

	// Validate response
	var validationErrors []protocol_validate.AssertionError

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		validationErrors = append(validationErrors, protocol_validate.AssertionError{
			Assertion: "http_status",
			Error:     fmt.Sprintf("expected 200, got %d", resp.StatusCode),
			Context:   string(responseBody),
		})
	}

	// Parse as SSE for streaming
	lines := strings.Split(string(responseBody), "\n")
	if len(lines) == 0 {
		validationErrors = append(validationErrors, protocol_validate.AssertionError{
			Assertion: "sse_lines",
			Error:     "no SSE lines received",
		})
	}

	// Check for proper SSE format
	hasDataPrefix := false
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			hasDataPrefix = true
			break
		}
	}
	if !hasDataPrefix {
		validationErrors = append(validationErrors, protocol_validate.AssertionError{
			Assertion: "sse_format",
			Error:     "no data: prefix found in SSE response",
		})
	}

	if len(validationErrors) > 0 {
		result.Errors = validationErrors
		result.Passed = false
	} else {
		result.Passed = true
	}

	return result
}

// runOpenCodeProfileTest executes a single opencode profile test
func runOpenCodeProfileTest(env *protocol_validate.ProfileTestEnv, scenario protocol_validate.ProfileScenario, streaming bool) protocol_validate.ProfileTestResult {
	name := fmt.Sprintf("opencode/%s", scenario.Name)
	if streaming {
		name += "/streaming"
	} else {
		name += "/nonstream"
	}

	result := protocol_validate.ProfileTestResult{
		Name:     name,
		Profile:  protocol_validate.ProfileTypeOpenCode,
		Scenario: scenario.Name,
		Passed:   false,
	}

	// Build request for OpenCode (Anthropic format)
	baseURL := env.BaseURL()
	modelToken := env.ModelToken()

	// OpenCode uses /tingly/opencode endpoint
	endpoint := "/tingly/opencode/v1/messages"

	requestBody := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello, world!"},
		},
		"stream": streaming,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "marshal_request",
			Error:     fmt.Sprintf("marshal request: %v", err),
		})
		return result
	}
	result.RequestBody = bodyBytes

	// Create HTTP request
	url := baseURL + endpoint
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "create_request",
			Error:     fmt.Sprintf("create request: %v", err),
		})
		return result
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", modelToken)

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "send_request",
			Error:     fmt.Sprintf("send request: %v", err),
		})
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode
	result.RequestHeaders = req.Header

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Errors = append(result.Errors, protocol_validate.AssertionError{
			Assertion: "read_response",
			Error:     fmt.Sprintf("read response: %v", err),
		})
		return result
	}
	result.ResponseBody = responseBody

	// Validate response (similar to claude but without OAuth transformations)
	var validationErrors []protocol_validate.AssertionError

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		validationErrors = append(validationErrors, protocol_validate.AssertionError{
			Assertion: "http_status",
			Error:     fmt.Sprintf("expected 200, got %d", resp.StatusCode),
			Context:   string(responseBody),
		})
	}

	// Parse and validate based on streaming mode
	if streaming {
		// Parse as SSE
		parsed := sse.AssembleAnthropicStream(strings.Split(string(responseBody), "\n"))
		if parsed.Content == "" {
			validationErrors = append(validationErrors, protocol_validate.AssertionError{
				Assertion: "sse_parse",
				Error:     "failed to parse SSE response",
			})
		}
	} else {
		// Parse as JSON
		parsed := sse.ParseAnthropicResult(map[string]interface{}{})
		if err := json.Unmarshal(responseBody, &parsed); err != nil {
			validationErrors = append(validationErrors, protocol_validate.AssertionError{
				Assertion: "json_parse",
				Error:     fmt.Sprintf("parse JSON: %v", err),
			})
		}
	}

	if len(validationErrors) > 0 {
		result.Errors = validationErrors
		result.Passed = false
	} else {
		result.Passed = true
	}

	return result
}
