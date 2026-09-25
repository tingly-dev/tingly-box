package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// TestNewServerMCPRuntimeOnFirstRun pins that a server booted on a brand-new
// config has a working MCP runtime. The runtime config only exists after
// builtin tool registration seeds it; constructing the runtime earlier left
// it nil (MCP silently off) until the next restart.
func TestNewServerMCPRuntimeOnFirstRun(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	s := NewServer(cfg)
	defer s.CloseVirtualModelServer()

	if s.mcpRuntime == nil {
		t.Fatal("MCP runtime is nil on a first-run config")
	}
	if s.mcpRuntime.VirtualRegistry() == nil {
		t.Fatal("MCP runtime has no virtual tool registry")
	}
}
