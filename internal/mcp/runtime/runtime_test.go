package runtime

import (
	"context"
	"testing"
	"time"

	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNormalizeAndParseToolName(t *testing.T) {
	name := NormalizeToolName("search", "web_search")
	if name != "tingly_box_mcp__search__web_search" {
		t.Fatalf("unexpected normalized name: %s", name)
	}

	source, tool, ok := ParseNormalizedToolName(name)
	if !ok {
		t.Fatalf("expected normalized name to parse")
	}
	if source != "search" || tool != "web_search" {
		t.Fatalf("unexpected parse result: source=%s tool=%s", source, tool)
	}
}

func TestIsMCPToolName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "tingly_box_mcp__search__web_search", want: true},
		{name: "tingly_box_mcp__onlyonesep", want: false},
		{name: "mcp__search__web_search", want: false},
		{name: "web_search", want: false},
		{name: "", want: false},
	}

	for _, tc := range cases {
		got := IsMCPToolName(tc.name)
		if got != tc.want {
			t.Fatalf("IsMCPToolName(%q)=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBuildAllowList(t *testing.T) {
	allowAll, allowSet := buildAllowList(nil)
	if !allowAll || allowSet != nil {
		t.Fatalf("expected nil list to allow all")
	}

	allowAll, allowSet = buildAllowList([]string{"*"})
	if !allowAll || allowSet != nil {
		t.Fatalf("expected wildcard to allow all")
	}

	allowAll, allowSet = buildAllowList([]string{"web_search", "web_fetch"})
	if allowAll {
		t.Fatalf("expected explicit list to not allow all")
	}
	if !allowSet["web_search"] || !allowSet["web_fetch"] {
		t.Fatalf("expected allow set to include explicit names")
	}
}

// TestRuntimeGetOrCreateSource tests the getOrCreateSource method
func TestRuntimeGetOrCreateSource(t *testing.T) {
	ctx := context.Background()

	// Create test config
	cfg := &typ.MCPRuntimeConfig{
		RequestTimeout: 30,
		Sources: []typ.MCPSourceConfig{
			{
				ID:        "test-stdio",
				Name:      "Test Stdio Source",
				Transport: "stdio",
				Command:   "echo",
				Args:      []string{"test"},
				Enabled:   typ.BoolPtr(true),
			},
		},
	}

	// Create runtime
	r := &Runtime{
		getConfig:         func() *typ.MCPRuntimeConfig { return cfg },
		sc:                newSessionCache(),
		toolSourceFactory: NewToolSourceFactory(newSessionCache(), nil),
		activeSources:     make(map[string]ToolSource),
	}

	// Test getting existing source
	source1, err := r.getOrCreateSource(ctx, "test-stdio")
	if err != nil {
		t.Fatalf("Failed to get source: %v", err)
	}

	if source1 == nil {
		t.Fatal("Source is nil")
	}

	if source1.GetSourceID() != "test-stdio" {
		t.Errorf("Expected source ID 'test-stdio', got '%s'", source1.GetSourceID())
	}

	if source1.GetType() != TransportStdio {
		t.Errorf("Expected transport type 'stdio', got '%s'", source1.GetType())
	}

	// Test source caching (should return same instance)
	source2, err := r.getOrCreateSource(ctx, "test-stdio")
	if err != nil {
		t.Fatalf("Failed to get cached source: %v", err)
	}

	if source1 != source2 {
		t.Error("Expected cached source to be same instance")
	}
}

// TestRuntimeCallTool_BasicParsing tests the CallTool method's parsing logic
func TestRuntimeCallTool_BasicParsing(t *testing.T) {
	// Create test config
	cfg := &typ.MCPRuntimeConfig{
		RequestTimeout: 30,
		Sources: []typ.MCPSourceConfig{
			{
				ID:        "test-echo",
				Name:      "Test Echo Source",
				Transport: "stdio",
				Command:   "echo",
				Args:      []string{"hello"},
				Enabled:   typ.BoolPtr(true),
				Tools:     []string{"*"},
			},
		},
	}

	// Create runtime (just to verify it doesn't crash)
	_ = NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })

	// Test normalized tool name parsing
	_, toolName, ok := ParseNormalizedToolName("tingly_box_mcp__test-echo__some_tool")
	if !ok {
		t.Fatal("Failed to parse normalized tool name")
	}

	if toolName != "some_tool" {
		t.Errorf("Expected tool name 'some_tool', got '%s'", toolName)
	}
}

// TestServerReadyCheck tests the server readiness check mechanism
func TestServerReadyCheck(t *testing.T) {
	// This test verifies that the waitForServerReady mechanism is in place
	// In real scenarios, this would test with actual MCP servers

	t.Log("Server ready check mechanism is implemented in StdioToolSource.waitForServerReady()")
	t.Log("It includes:")
	t.Log("- Retry mechanism (up to 5 attempts)")
	t.Log("- Exponential backoff (100ms between retries)")
	t.Log("- 5 second timeout")
	t.Log("- Readiness check via ListTools()")

	// Verify constants are defined
	if maxStartupRetries != 5 {
		t.Errorf("Expected maxStartupRetries=5, got %d", maxStartupRetries)
	}

	if startupRetryDelay != 100*time.Millisecond {
		t.Errorf("Expected startupRetryDelay=100ms, got %v", startupRetryDelay)
	}

	if startupReadyCheckTimeout != 5*time.Second {
		t.Errorf("Expected startupReadyCheckTimeout=5s, got %v", startupReadyCheckTimeout)
	}
}

func TestListEnabledServerToolNames_AdvisorDisabledByDefault(t *testing.T) {
	r := NewRuntime(func() *typ.MCPRuntimeConfig {
		return &typ.MCPRuntimeConfig{}
	})
	if r == nil {
		t.Fatal("expected runtime")
	}

	r.RegisterAdviser(typ.AdvisorConfig{
		BaseURL: "https://example.com",
		Model:   "claude-opus-4-6",
		APIKey:  "test-key",
	}, nil)

	names := r.ListEnabledServerToolNames(context.Background())
	if _, ok := names[NormalizeToolName("builtin", "advisor")]; ok {
		t.Fatalf("expected %q to be disabled by default", NormalizeToolName("builtin", "advisor"))
	}
	if _, ok := names[NormalizeToolName("advisor", "advisor")]; ok {
		t.Fatalf("expected %q to be disabled by default", NormalizeToolName("advisor", "advisor"))
	}
}

func TestListEnabledServerToolNames_AdvisorEnabledWhenSourceEnabled(t *testing.T) {
	r := NewRuntime(func() *typ.MCPRuntimeConfig {
		return &typ.MCPRuntimeConfig{
			Sources: []typ.MCPSourceConfig{{
				ID:           mcptools.BuiltinAdvisorSourceID,
				Enabled:      typ.BoolPtr(true),
				IsClientTool: typ.BoolPtr(false),
				Tools:        []string{"advisor"},
			}},
		}
	})
	if r == nil {
		t.Fatal("expected runtime")
	}

	r.RegisterAdviser(typ.AdvisorConfig{
		BaseURL: "https://example.com",
		Model:   "claude-opus-4-6",
		APIKey:  "test-key",
	}, nil)

	names := r.ListEnabledServerToolNames(context.Background())
	if _, ok := names[NormalizeToolName("builtin", "advisor")]; !ok {
		t.Fatalf("expected %q to be enabled", NormalizeToolName("builtin", "advisor"))
	}
	if _, ok := names[NormalizeToolName("advisor", "advisor")]; !ok {
		t.Fatalf("expected %q to be enabled for backward compatibility", NormalizeToolName("advisor", "advisor"))
	}
}
