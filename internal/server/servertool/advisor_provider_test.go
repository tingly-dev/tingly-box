package servertool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAdvisorProvider_Descriptor(t *testing.T) {
	p := servertool.NewAdvisorProvider(typ.AdvisorConfig{MaxUsesPerRequest: 3}, nil, nil)
	d := p.Descriptor()
	assert.Equal(t, "advisor", d.Name)
	assert.Equal(t, typ.ToolVisibilityServer, d.Visibility)
}

func TestAdvisorProvider_Hook(t *testing.T) {
	p := servertool.NewAdvisorProvider(typ.AdvisorConfig{}, nil, nil)
	assert.NotNil(t, p.Hook())
	_, ok := p.Hook().(servertool.AdvisorHook)
	assert.True(t, ok)
}
