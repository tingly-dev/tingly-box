package servertool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type stubProvider struct {
	tool mcpruntime.VirtualTool
	hook servertool.Hook
}

func (s stubProvider) Descriptor() mcpruntime.VirtualTool { return s.tool }
func (s stubProvider) Hook() servertool.Hook              { return s.hook }

func TestPipeline_RegisterInto(t *testing.T) {
	registry := mcpruntime.NewVirtualToolRegistry()
	p := servertool.NewPipeline()

	p.Register(stubProvider{
		tool: mcpruntime.VirtualTool{
			Name:       "test_tool",
			Visibility: typ.ToolVisibilityServer,
		},
	})
	p.RegisterInto(registry)

	_, ok := registry.Get("test_tool")
	assert.True(t, ok)
}

func TestPipeline_HooksCollected(t *testing.T) {
	p := servertool.NewPipeline()
	p.Register(stubProvider{
		tool: mcpruntime.VirtualTool{Name: "tool_a"},
		hook: servertool.AdvisorHook{},
	})
	p.Register(stubProvider{
		tool: mcpruntime.VirtualTool{Name: "tool_b"},
		// hook is nil — should not panic
	})

	exec := p.NewExecutor(nil, nil)
	assert.NotNil(t, exec)
}
