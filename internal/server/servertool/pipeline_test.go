package servertool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type stubProvider struct {
	tool coretool.VirtualTool
	hook servertool.Hook
}

func (s stubProvider) Descriptor() coretool.VirtualTool { return s.tool }
func (s stubProvider) Hook() servertool.Hook            { return s.hook }

type stubRegistry struct {
	tools map[string]coretool.VirtualTool
}

func (r *stubRegistry) Register(tool coretool.VirtualTool) {
	if r.tools == nil {
		r.tools = make(map[string]coretool.VirtualTool)
	}
	r.tools[tool.Name] = tool
}

func TestPipeline_RegisterInto(t *testing.T) {
	registry := &stubRegistry{}
	p := servertool.NewPipeline()

	p.Register(stubProvider{
		tool: coretool.VirtualTool{
			Name:       "test_tool",
			Visibility: typ.ToolVisibilityServer,
		},
	})
	p.RegisterInto(registry)

	_, ok := registry.tools["test_tool"]
	assert.True(t, ok)
}

func TestPipeline_HooksCollected(t *testing.T) {
	p := servertool.NewPipeline()
	p.Register(stubProvider{
		tool: coretool.VirtualTool{Name: "tool_a"},
		hook: servertool.AdvisorHook{},
	})
	p.Register(stubProvider{
		tool: coretool.VirtualTool{Name: "tool_b"},
		// hook is nil — should not panic
	})

	exec := p.NewExecutor(nil, nil)
	assert.NotNil(t, exec)
}
