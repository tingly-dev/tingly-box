package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const ollamaBaseURL = "http://localhost:11434"
const ollamaModel = "qwen2.5:latest"

// checkOllamaAvailable returns true if ollama is running and the model is available.
func checkOllamaAvailable(t *testing.T) bool {
	t.Helper()
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(ollamaBaseURL + "/api/tags")
	if err != nil {
		t.Logf("ollama not available: %v", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Logf("ollama returned non-200: %d", resp.StatusCode)
		return false
	}
	return true
}

// TestAdvisorVirtualTool_OllamaReal exercises the full advisor call against a real ollama instance.
// It is skipped automatically when ollama is not running.
func TestAdvisorVirtualTool_OllamaReal(t *testing.T) {
	if !checkOllamaAvailable(t) {
		t.Skip("ollama not running — skipping real advisor integration test")
	}

	cp := client.NewClientPool()
	cfg := typ.AdvisorConfig{
		BaseURL:           ollamaBaseURL + "/v1",
		Model:             ollamaModel,
		APIKey:            "ollama", // any non-empty value
		MaxUsesPerRequest: 2,
		MaxTokens:         512,
	}
	store := NewSessionStore(10 * time.Minute)
	defer store.Sweep()

	vt := NewAdvisorVirtualTool(cfg, cp, store)

	uses2 := 2
	actx := &AdvisorContext{
		Messages: []map[string]any{
			{"role": "system", "content": "You are a helpful coding assistant."},
			{"role": "user", "content": "I need to add a new endpoint /health to a Go HTTP server."},
			{"role": "assistant", "content": "I'll add a /health endpoint that returns 200 OK with a JSON body."},
		},
		UsesRemaining: &uses2,
	}
	ctx := WithAdvisorContext(context.Background(), actx)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "advisor",
			Arguments: map[string]any{},
		},
	}

	result, err := vt.Handler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.IsError {
		t.Fatalf("advisor returned error result: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content in result")
	}

	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	t.Logf("Advisor response:\n%s", text.Text)

	// The advisor must return valid JSON with at least "assessment" and "recommendation".
	var resp map[string]any
	if err := json.Unmarshal([]byte(text.Text), &resp); err != nil {
		t.Fatalf("advisor response is not valid JSON: %v\nraw: %s", err, text.Text)
	}
	if _, ok := resp["assessment"]; !ok {
		t.Errorf("response missing 'assessment' field: %s", text.Text)
	}
	if _, ok := resp["recommendation"]; !ok {
		t.Errorf("response missing 'recommendation' field: %s", text.Text)
	}

	// UsesRemaining should have decremented.
	if *actx.UsesRemaining != 1 {
		t.Errorf("expected UsesRemaining=1, got %d", *actx.UsesRemaining)
	}
}

// TestAdvisorVirtualTool_OllamaExhaustion verifies exhaustion behaviour against real ollama.
func TestAdvisorVirtualTool_OllamaExhaustion(t *testing.T) {
	if !checkOllamaAvailable(t) {
		t.Skip("ollama not running — skipping real advisor integration test")
	}

	cp := client.NewClientPool()
	cfg := typ.AdvisorConfig{
		BaseURL:           ollamaBaseURL + "/v1",
		Model:             ollamaModel,
		APIKey:            "ollama",
		MaxUsesPerRequest: 1,
		MaxTokens:         256,
	}
	store := NewSessionStore(10 * time.Minute)
	defer store.Sweep()

	vt := NewAdvisorVirtualTool(cfg, cp, store)

	uses1 := 1
	actx := &AdvisorContext{
		Messages:      []map[string]any{{"role": "user", "content": "hello"}},
		UsesRemaining: &uses1,
	}
	ctx := WithAdvisorContext(context.Background(), actx)

	makeReq := func() mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "advisor",
				Arguments: map[string]any{},
			},
		}
	}

	// First call: should succeed.
	result, err := vt.Handler(ctx, makeReq())
	if err != nil {
		t.Fatalf("first call unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("first call should not error, got: %v", result.Content)
	}
	if *actx.UsesRemaining != 0 {
		t.Errorf("expected UsesRemaining=0 after first call, got %d", *actx.UsesRemaining)
	}

	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	t.Logf("First advisor response: %s", text.Text)

	// Second call: should be rejected with exhaustion.
	result, err = vt.Handler(ctx, makeReq())
	if err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("second call should return error (exhausted)")
	}
	text2, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent on exhaustion, got %T", result.Content[0])
	}
	if text2.Text != "Advisor consultations exhausted for this request." {
		t.Errorf("unexpected exhaustion message: %s", text2.Text)
	}
}
