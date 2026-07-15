package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	stagetoolloop "github.com/tingly-dev/tingly-box/internal/protocol/stage/toolloop"
	mcpmodule "github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	servertransform "github.com/tingly-dev/tingly-box/internal/server/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (ph *ProtocolHandler) newProtocolStageBetaToolLoop(
	c *gin.Context,
	provider *typ.Provider,
	hasNativeAdvisor bool,
) (protocolstage.Stage, error) {
	if ph == nil || ph.deps.MCPRuntime == nil {
		return nil, fmt.Errorf("construct Beta Tool Loop: MCP runtime is nil")
	}
	if provider == nil || provider.UUID == "" {
		return nil, fmt.Errorf("construct Beta Tool Loop: provider identity is empty")
	}
	return mcpmodule.NewAnthropicBetaStage(mcpmodule.AnthropicBetaStageConfig{
		Tools: servertransform.NewProtocolStageBetaToolProvider(
			ph.deps.MCPRuntime,
			c.GetHeader("X-Tingly-Advisor-Depth") != "",
			hasNativeAdvisor,
		),
		Executor:      mcpmodule.NewServerToolExecutor(ph),
		Continuations: mcpmodule.NewProviderBetaContinuationStore(provider.UUID),
	})
}

// preserveProtocolStageSideEffectBoundary prevents provider failover from
// replaying a successful server tool when a later round or outward conversion
// fails before any client-visible output has committed.
func preserveProtocolStageSideEffectBoundary(c *gin.Context, err error, committed bool) {
	if c == nil || (!committed && !stagetoolloop.HasCommittedSideEffects(err)) {
		return
	}
	MarkSideEffectsCommittedIfGate(c.Writer)
	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"protocol_pipeline":      "stage",
		"side_effects_committed": true,
	}).Debug("Protocol Stage disabled failover after tool side effects")
}
