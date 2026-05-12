package mcp

import (
	"testing"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestNewHandler_WithoutRuntime_CreatesOwnRuntime(t *testing.T) {
	h := NewHandler(nil)
	if h == nil {
		t.Fatal("NewHandler(nil) returned nil")
	}
	if h.transportHandler == nil {
		t.Fatal("transportHandler must not be nil")
	}
}

func TestNewHandler_WithSharedRuntime_UsesIt(t *testing.T) {
	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
		return &typ.MCPRuntimeConfig{}
	})

	h := NewHandler(nil, rt)
	if h == nil {
		t.Fatal("NewHandler(nil, rt) returned nil")
	}
	if h.transportHandler == nil {
		t.Fatal("transportHandler must not be nil")
	}

	// The transport handler must reference the same runtime pointer (Fix 1).
	if h.transportHandler.RuntimePtr() != rt {
		t.Error("transportHandler should use the provided shared runtime, not a new one")
	}
}
