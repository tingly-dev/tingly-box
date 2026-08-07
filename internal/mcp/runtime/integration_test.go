package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestStdioToolSource_Configuration tests stdio tool source configuration
func TestStdioToolSource_Configuration(t *testing.T) {
	// Create a simple test configuration
	cfg := typ.MCPSourceConfig{
		ID:        "test-stdio",
		Name:      "Test Stdio Source",
		Transport: "stdio",
		Command:   "echo",
		Args:      []string{"hello", "world"},
		Enabled:   typ.BoolPtr(true),
		Tools:     []string{"*"},
	}

	sc := newSessionCache()
	source, err := NewStdioToolSource(cfg, sc)
	if err != nil {
		t.Fatalf("Failed to create stdio source: %v", err)
	}

	// Verify source properties
	if source.GetSourceID() != "test-stdio" {
		t.Errorf("Expected source ID 'test-stdio', got '%s'", source.GetSourceID())
	}

	if source.GetType() != TransportStdio {
		t.Errorf("Expected transport type 'stdio', got '%s'", source.GetType())
	}

	// Test initial state
	status := source.GetConnectionStatus()
	if status.State != StateDisconnected {
		t.Errorf("Expected initial state %s, got %s", StateDisconnected, status.State)
	}

	// Test disconnect before connect (should be safe)
	ctx := context.Background()
	if err := source.Disconnect(ctx); err != nil {
		t.Errorf("Disconnect before connect should be safe, got error: %v", err)
	}
}

// TestHTTPToolSource_Configuration tests HTTP source configuration
func TestHTTPToolSource_Configuration(t *testing.T) {
	ctx := context.Background()

	// Test HTTP source creation
	cfg := typ.MCPSourceConfig{
		ID:        "test-http",
		Name:      "Test HTTP Source",
		Transport: "http",
		Endpoint:  "http://localhost:8080/mcp",
		Enabled:   typ.BoolPtr(true),
		Tools:     []string{"*"},
	}

	sc := newSessionCache()
	source, err := NewHTTPToolSource(cfg, sc)
	if err != nil {
		t.Fatalf("Failed to create HTTP source: %v", err)
	}

	// Verify source properties
	if source.GetSourceID() != "test-http" {
		t.Errorf("Expected source ID 'test-http', got '%s'", source.GetSourceID())
	}

	if source.GetType() != TransportHTTP {
		t.Errorf("Expected transport type 'http', got '%s'", source.GetType())
	}

	// Test connection state
	status := source.GetConnectionStatus()
	if status.State != StateDisconnected {
		t.Errorf("Expected initial state %s, got %s", StateDisconnected, status.State)
	}

	// Test disconnect before connect (should be safe)
	if err := source.Disconnect(ctx); err != nil {
		t.Errorf("Disconnect before connect should be safe, got error: %v", err)
	}
}

// Compile-time checks: reconnect-capable sources implement ReconnectableSource.
var (
	_ ReconnectableSource = (*HTTPToolSource)(nil)
	_ ReconnectableSource = (*SSEToolSource)(nil)
)

// TestSSEToolSource_Configuration tests SSE source configuration
func TestSSEToolSource_Configuration(t *testing.T) {
	// Test SSE source creation
	cfg := typ.MCPSourceConfig{
		ID:        "test-sse",
		Name:      "Test SSE Source",
		Transport: "sse",
		Endpoint:  "http://localhost:8080/mcp",
		Enabled:   typ.BoolPtr(true),
		Tools:     []string{"*"},
	}

	sc := newSessionCache()
	source, err := NewSSEToolSource(cfg, sc)
	if err != nil {
		t.Fatalf("Failed to create SSE source: %v", err)
	}

	// Verify source properties
	if source.GetSourceID() != "test-sse" {
		t.Errorf("Expected source ID 'test-sse', got '%s'", source.GetSourceID())
	}

	if source.GetType() != TransportSSE {
		t.Errorf("Expected transport type 'sse', got '%s'", source.GetType())
	}
}

// TestRuntime_EndToEndToolCall tests complete tool call flow
func TestRuntime_EndToEndToolCall(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	ctx := context.Background()

	// Get the project root directory
	projectRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	// Navigate to project root (from internal/mcp/runtime to project root)
	for i := 0; i < 3; i++ {
		projectRoot = filepath.Dir(projectRoot)
	}

	// Build path to tingly-box binary
	binaryPath := filepath.Join(projectRoot, "build", "tingly-box")

	// Check if binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skipf("Skipping end-to-end test - tingly-box binary not found at %s", binaryPath)
	}

	// Create a test configuration with builtin MCP server
	cfg := &typ.MCPRuntimeConfig{
		RequestTimeout: 30,
		Sources: []typ.MCPSourceConfig{
			{
				ID:        "webtools",
				Name:      "Built-in Web Tools",
				Transport: "stdio",
				Command:   binaryPath,
				Args:      []string{"mcp-builtin"},
				Enabled:   typ.BoolPtr(true),
				Tools:     []string{"*"},
			},
		},
	}

	// Create runtime
	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()

	t.Run("ListServerToolsForInjection", func(t *testing.T) {
		tools := r.ListServerToolsForInjection(ctx)
		if tools == nil {
			t.Fatal("Expected tools list, got nil")
		}

		t.Logf("Found %d tools", len(tools))

		// Injection is reserved for server-side virtual tools.
		// webtools is a client-facing non-virtual source and must not be injected.
		if len(tools) != 0 {
			t.Errorf("Expected 0 injected tools for non-virtual webtools-only config, got %d", len(tools))
		}
	})

	t.Run("CallTool_WebSearch", func(t *testing.T) {
		// Check if SERPER_API_KEY is set
		apiKey := os.Getenv("SERPER_API_KEY")
		if apiKey == "" {
			t.Skip("Skipping websearch test - SERPER_API_KEY environment variable not set")
		}

		normalizedName := NormalizeToolName("webtools", "mcp_web_search")
		arguments := `{"query": "golang programming"}`

		result, err := r.CallTool(ctx, normalizedName, arguments)
		if err != nil {
			t.Fatalf("Tool call failed: %v", err)
		}

		if result.FirstText() == "" {
			t.Fatal("Expected non-empty result from websearch")
		}

		// Verify result contains expected content
		// Result should be JSON with search results
		if !strings.Contains(result.FirstText(), "content") {
			t.Errorf("Expected result to contain 'content', got: %s", result.FirstText())
		}

		// Parse the JSON response to verify structure
		var response map[string]interface{}
		if err := json.Unmarshal([]byte(result.FirstText()), &response); err != nil {
			t.Fatalf("Failed to parse result JSON: %v", err)
		}

		content, ok := response["content"]
		if !ok {
			t.Fatal("Expected result to have 'content' field")
		}

		// Content should be an array
		contentArray, ok := content.([]interface{})
		if !ok {
			t.Fatalf("Expected content to be an array, got %T", content)
		}

		if len(contentArray) == 0 {
			t.Fatal("Expected non-empty content array")
		}

		// Check that we got some text content
		foundTextContent := false
		for _, item := range contentArray {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
				foundTextContent = true
				if text, ok := itemMap["text"].(string); ok {
					t.Logf("✅ Got search results with text content (length: %d)", len(text))
					// Verify the search actually returned something relevant
					if len(text) > 50 {
						t.Logf("✅ Search returned substantial content: %s...", text[:50])
					}
				}
				break
			}
		}

		if !foundTextContent {
			t.Error("Expected to find text content in search results")
		}

		t.Logf("✅ Websearch test passed - got valid search results")
	})

	t.Run("CallTool_WebFetch", func(t *testing.T) {
		// WebFetch uses Jina Reader which doesn't require API key
		// Test with a simple URL
		normalizedName := NormalizeToolName("webtools", "mcp_web_fetch")

		// Use a reliable test URL with a prompt
		testURL := "https://example.com"
		arguments := fmt.Sprintf(`{"url": "%s", "prompt": "Summarize this page"}`, testURL)

		result, err := r.CallTool(ctx, normalizedName, arguments)
		if err != nil {
			t.Fatalf("Tool call failed: %v", err)
		}

		if result.FirstText() == "" {
			t.Fatal("Expected non-empty result from webfetch")
		}

		// Verify result contains expected content
		// Result should be JSON with fetched content
		if !strings.Contains(result.FirstText(), "content") {
			t.Errorf("Expected result to contain 'content', got: %s", result.FirstText())
		}

		// Parse the JSON response to verify structure
		var response map[string]interface{}
		if err := json.Unmarshal([]byte(result.FirstText()), &response); err != nil {
			t.Fatalf("Failed to parse result JSON: %v", err)
		}

		content, ok := response["content"]
		if !ok {
			t.Fatal("Expected result to have 'content' field")
		}

		// Content should be an array
		contentArray, ok := content.([]interface{})
		if !ok {
			t.Fatalf("Expected content to be an array, got %T", content)
		}

		if len(contentArray) == 0 {
			t.Fatal("Expected non-empty content array")
		}

		// Check that we got some text content
		foundTextContent := false
		for _, item := range contentArray {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
				foundTextContent = true
				if text, ok := itemMap["text"].(string); ok {
					t.Logf("✅ Got webfetch content (length: %d)", len(text))
					// Verify we actually got some content
					if len(text) > 20 {
						previewLen := 50
						if len(text) < previewLen {
							previewLen = len(text)
						}
						t.Logf("✅ Webfetch returned content: %s...", text[:previewLen])
					}
				}
				break
			}
		}

		if !foundTextContent {
			t.Error("Expected to find text content in webfetch result")
		}

		t.Logf("✅ Webfetch test passed - successfully fetched URL content")
	})

	t.Run("ListSourceTools", func(t *testing.T) {
		sourceTools, err := r.ListSourceTools(ctx)
		if err != nil {
			t.Fatalf("Failed to list source tools: %v", err)
		}

		// Check that webtools source exists
		webtoolsTools, ok := sourceTools["webtools"]
		if !ok {
			t.Fatal("Expected to find webtools source")
		}

		t.Logf("Found %d tools from webtools source", len(webtoolsTools))

		if len(webtoolsTools) < 2 {
			t.Errorf("Expected at least 2 tools, got %d", len(webtoolsTools))
		}

		// Verify tool names
		foundWebSearch := false
		foundWebFetch := false
		for _, tool := range webtoolsTools {
			if tool.Name == "mcp_web_search" {
				foundWebSearch = true
			}
			if tool.Name == "mcp_web_fetch" {
				foundWebFetch = true
			}
		}

		if !foundWebSearch {
			t.Error("Expected to find mcp_web_search in source tools")
		}
		if !foundWebFetch {
			t.Error("Expected to find mcp_web_fetch in source tools")
		}
	})
}

// TestToolSourceFactory_AllTransports tests factory creates all transport types
func TestToolSourceFactory_AllTransports(t *testing.T) {
	sc := newSessionCache()
	factory := NewToolSourceFactory(sc, nil)

	testCases := []struct {
		name      string
		transport string
		config    typ.MCPSourceConfig
	}{
		{
			name:      "Stdio transport",
			transport: "stdio",
			config: typ.MCPSourceConfig{
				ID:        "test-stdio",
				Transport: "stdio",
				Command:   "echo",
				Args:      []string{"test"},
			},
		},
		{
			name:      "HTTP transport",
			transport: "http",
			config: typ.MCPSourceConfig{
				ID:        "test-http",
				Transport: "http",
				Endpoint:  "http://localhost:8080/mcp",
			},
		},
		{
			name:      "SSE transport",
			transport: "sse",
			config: typ.MCPSourceConfig{
				ID:        "test-sse",
				Transport: "sse",
				Endpoint:  "http://localhost:8080/mcp",
			},
		},
		{
			name:      "Default transport (should become stdio)",
			transport: "",
			config: typ.MCPSourceConfig{
				ID:      "test-default",
				Command: "echo",
				Args:    []string{"test"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := factory.CreateToolSource(tc.config)
			if err != nil {
				t.Fatalf("Failed to create %s source: %v", tc.name, err)
			}

			if source == nil {
				t.Fatal("Source should not be nil")
			}

			// Verify transport type matches expected
			expectedTransport := tc.transport
			if expectedTransport == "" {
				expectedTransport = "stdio"
			}

			if source.GetType() != TransportType(expectedTransport) {
				t.Errorf("Expected transport type %s, got %s", expectedTransport, source.GetType())
			}
		})
	}

	// Test unsupported transport
	t.Run("Unsupported transport", func(t *testing.T) {
		config := typ.MCPSourceConfig{
			ID:        "test-unsupported",
			Transport: "websocket", // Unsupported
		}

		_, err := factory.CreateToolSource(config)
		if err == nil {
			t.Error("Expected error for unsupported transport")
		}

		if _, ok := err.(*UnsupportedTransportError); !ok {
			t.Errorf("Expected UnsupportedTransportError, got %T", err)
		}
	})
}

// TestHealthMonitor_Strategy tests exponential backoff strategy
func TestHealthMonitor_Strategy(t *testing.T) {
	strategy := NewExponentialBackoffStrategy()

	// Test cases account for ±25% jitter in implementation
	testCases := []struct {
		retryCount int
		minDelay   time.Duration // Base delay - 25% jitter
		maxDelay   time.Duration // Base delay + 25% jitter (or max)
	}{
		{0, 5 * time.Second, 5 * time.Second},                  // No jitter for first attempt
		{1, 7500 * time.Millisecond, 12500 * time.Millisecond}, // 10s ± 25%
		{2, 15 * time.Second, 25 * time.Second},                // 20s ± 25%
		{3, 30 * time.Second, 50 * time.Second},                // 40s ± 25%
		{4, 45 * time.Second, 60 * time.Second},                // 60s ± 25% (capped at max)
		{5, 45 * time.Second, 60 * time.Second},                // Still max
		{10, 45 * time.Second, 60 * time.Second},               // Still max
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("retry_%d", tc.retryCount), func(t *testing.T) {
			// Jitter is random, so sample repeatedly to exercise the bounds.
			for i := 0; i < 100; i++ {
				delay := strategy.NextRetry(tc.retryCount)

				if delay < tc.minDelay {
					t.Fatalf("Delay %v is less than minimum %v", delay, tc.minDelay)
				}
				if delay > tc.maxDelay {
					t.Fatalf("Delay %v exceeds maximum %v", delay, tc.maxDelay)
				}
			}
		})
	}
}

// TestErrorClassifier tests error classification
func TestErrorClassifier(t *testing.T) {
	classifier := &DefaultErrorClassifier{}

	cases := []struct {
		name      string
		err       error
		transient bool
		permanent bool
	}{
		{"nil error", nil, false, false},
		{"HTTP 500", &HTTPError{StatusCode: 500, Message: "internal"}, true, false},
		{"HTTP 503", &HTTPError{StatusCode: 503, Message: "unavailable"}, true, false},
		{"HTTP 401", &HTTPError{StatusCode: 401, Message: "unauthorized"}, false, true},
		{"HTTP 404", &HTTPError{StatusCode: 404, Message: "not found"}, false, true},
		{"HTTP 429 is neither", &HTTPError{StatusCode: 429, Message: "rate limited"}, false, false},
		{"wrapped HTTP 502", fmt.Errorf("call failed: %w", &HTTPError{StatusCode: 502, Message: "bad gateway"}), true, false},
		{"context deadline", context.DeadlineExceeded, true, false},
		{"context canceled", context.Canceled, true, false},
		{"net timeout", &net.DNSError{IsTimeout: true}, true, false},
		{"net non-timeout", &net.DNSError{IsNotFound: true}, false, false},
		{"plain error", fmt.Errorf("something odd"), false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifier.IsTransient(tc.err); got != tc.transient {
				t.Errorf("IsTransient(%v) = %v, want %v", tc.err, got, tc.transient)
			}
			if got := classifier.IsPermanent(tc.err); got != tc.permanent {
				t.Errorf("IsPermanent(%v) = %v, want %v", tc.err, got, tc.permanent)
			}
		})
	}
}

// TestRuntime_ConcurrentAccess exercises Runtime methods from concurrent
// goroutines so the race detector can catch unsynchronized state access.
func TestRuntime_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	ctx := context.Background()

	cfg := &typ.MCPRuntimeConfig{
		RequestTimeout: 30,
		Sources: []typ.MCPSourceConfig{
			{
				ID:        "test-concurrent",
				Name:      "Test Concurrent",
				Transport: "stdio",
				Command:   "echo",
				Args:      []string{"concurrent"},
				Enabled:   typ.BoolPtr(true),
				Tools:     []string{"*"},
			},
		},
	}

	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()

	const numGoroutines = 10
	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Mix of read paths that share runtime state.
			_ = r.GetConfig()
			_ = r.HasServerTools()
			_ = r.ListServerToolsForInjection(ctx)

			// CallTool with an unknown tool must fail gracefully, not race.
			normalizedTool := NormalizeToolName("no-such-source", "test_tool")
			if _, err := r.CallTool(ctx, normalizedTool, `{}`); err == nil {
				t.Errorf("Goroutine %d: expected error calling tool on unknown source", id)
			}
		}(i)
	}

	waited := make(chan struct{})
	go func() { wg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for goroutines to complete")
	}
}

// BenchmarkToolSourceFactory_Creation benchmarks tool source creation
func BenchmarkToolSourceFactory_Creation(b *testing.B) {
	sc := newSessionCache()
	factory := NewToolSourceFactory(sc, nil)

	config := typ.MCPSourceConfig{
		ID:        "bench-stdio",
		Transport: "stdio",
		Command:   "echo",
		Args:      []string{"bench"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := factory.CreateToolSource(config)
		if err != nil {
			b.Fatalf("Failed to create source: %v", err)
		}
	}
}

// BenchmarkRuntime_ParseToolName benchmarks tool name parsing
func BenchmarkRuntime_ParseToolName(b *testing.B) {
	names := []string{
		"tingly_box_mcp__webtools__mcp_web_search",
		"tingly_box_mcp__weather__get_current_weather",
		"tingly_box_mcp__filesystem__read_file",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range names {
			_, _, ok := ParseNormalizedToolName(name)
			if !ok {
				b.Errorf("Failed to parse tool name: %s", name)
			}
		}
	}
}
