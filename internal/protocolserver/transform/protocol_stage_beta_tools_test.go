package transform

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestProtocolStageBetaToolProviderPreservesAdvisorPreparation(t *testing.T) {
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{
		ID: "advisor", Name: "advisor", Transport: "advisor", Enabled: typ.BoolPtr(true),
		Visibility: typ.ToolVisibilityServer, Tools: []string{"advisor"},
	}}}
	runtime := runtime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	t.Cleanup(runtime.Close)
	runtime.VirtualRegistry().Register(coretool.VirtualTool{
		Name: "advisor", Description: "server-side advisor",
		InputSchema: mcp.ToolInputSchema{Type: "object"}, Visibility: typ.ToolVisibilityServer,
	})

	provider := NewProtocolStageBetaToolProvider(runtime, false, false)
	request := &anthropic.BetaMessageNewParams{}
	owned, err := provider.PrepareRequest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[0] != advisorInjectedToolName || !hasAnthropicBetaToolName(request.Tools, advisorInjectedToolName) {
		t.Fatalf("owned=%#v tools=%#v", owned, request.Tools)
	}
	if len(request.System) == 0 {
		t.Fatal("Advisor behavior was not added to the Beta system prompt")
	}

	native := NewProtocolStageBetaToolProvider(runtime, false, true)
	nativeRequest := &anthropic.BetaMessageNewParams{}
	nativeOwned, err := native.PrepareRequest(context.Background(), nativeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(nativeOwned) != 0 || len(nativeRequest.Tools) != 0 || len(nativeRequest.System) != 0 {
		t.Fatalf("native advisor was duplicated: owned=%#v request=%#v", nativeOwned, nativeRequest)
	}
}
