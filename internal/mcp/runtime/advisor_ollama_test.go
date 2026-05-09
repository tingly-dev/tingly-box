package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

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

	result, err := vt.Handler(ctx, ToolCall{Name: "advisor", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("advisor returned error result: %v", result.Contents)
	}
	if len(result.Contents) == 0 {
		t.Fatal("empty content in result")
	}

	text := result.FirstText()
	t.Logf("Advisor response:\n%s", text)

	// The advisor must return valid JSON with at least "assessment" and "recommendation".
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("advisor response is not valid JSON: %v\nraw: %s", err, text)
	}
	if _, ok := resp["assessment"]; !ok {
		t.Errorf("response missing 'assessment' field: %s", text)
	}
	if _, ok := resp["recommendation"]; !ok {
		t.Errorf("response missing 'recommendation' field: %s", text)
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

	makeCall := func() ToolCall {
		return ToolCall{Name: "advisor", Arguments: map[string]any{}}
	}

	// First call: should succeed.
	result, err := vt.Handler(ctx, makeCall())
	if err != nil {
		t.Fatalf("first call unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("first call should not error, got: %v", result.Contents)
	}
	if *actx.UsesRemaining != 0 {
		t.Errorf("expected UsesRemaining=0 after first call, got %d", *actx.UsesRemaining)
	}

	text := result.FirstText()
	t.Logf("First advisor response: %s", text)

	// Second call: should be rejected with exhaustion.
	result, err = vt.Handler(ctx, makeCall())
	if err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("second call should return error (exhausted)")
	}
	text2 := result.FirstText()
	if text2 != "Advisor consultations exhausted for this request." {
		t.Errorf("unexpected exhaustion message: %s", text2)
	}
}
