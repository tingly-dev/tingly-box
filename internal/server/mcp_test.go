package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAnthropicBetaGenericPathUsesProviderLimits(t *testing.T) {
	s := &Server{config: &config.Config{}}

	provider := &typ.Provider{Name: "deepseek"}
	assert.True(t, ShouldUseGenericMCPForProvider(s.config, provider))

	s.config.GenericMCP.ProviderLimits = "other-provider"
	assert.False(t, ShouldUseGenericMCPForProvider(s.config, provider))
}
