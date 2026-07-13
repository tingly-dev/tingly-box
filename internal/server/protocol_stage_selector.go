package server

import (
	"fmt"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/openaibridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/responsesbridge"
)

// ProtocolStageSelector is the immutable process-level selector enabled by
// --stage. It resolves only exact, capability-complete protocol pairs; callers
// keep all other paths on the legacy pipeline.
type ProtocolStageSelector struct {
	enabled  bool
	registry *stage.BridgeRegistry
}

// NewProtocolStageSelector constructs the production selector. The registry is
// code-defined rather than mutable configuration so active requests cannot see
// a partially updated protocol graph.
func NewProtocolStageSelector(enabled bool) *ProtocolStageSelector {
	registry, err := stage.NewBridgeRegistry(
		stage.NewIdentityBridge(protocol.TypeAnthropicV1),
		anthropicbridge.NewV1ToOpenAIChat(anthropicbridge.ChatOptions{}),
		anthropicbridge.NewV1ToOpenAIResponses(anthropicbridge.ResponsesOptions{}),
		stage.NewIdentityBridge(protocol.TypeAnthropicBeta),
		anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{}),
		anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{}),
		openaibridge.NewChatToAnthropicBeta(openaibridge.AnthropicOptions{}),
		openaibridge.NewChatToOpenAIResponses(openaibridge.ResponsesOptions{}),
		stage.NewIdentityBridge(protocol.TypeOpenAIResponses),
		responsesbridge.NewToAnthropicBeta(responsesbridge.AnthropicOptions{}),
		responsesbridge.NewToOpenAIChat(responsesbridge.ChatOptions{}),
	)
	if err != nil {
		panic(fmt.Sprintf("construct Protocol Stage selector: %v", err))
	}
	return &ProtocolStageSelector{enabled: enabled, registry: registry}
}

// Enabled reports the immutable server-start choice.
func (s *ProtocolStageSelector) Enabled() bool {
	return s != nil && s.enabled
}

// ShouldUseStage returns true only when --stage is enabled and an explicitly
// registered exact Bridge satisfies every required capability. A missing route
// is returned as an error so diagnostics can explain why that request stayed on
// legacy. Implicit identity Bridges are intentionally excluded: production
// routes must be opted in one pair at a time.
func (s *ProtocolStageSelector) ShouldUseStage(
	source, target protocol.APIType,
	required stage.Capabilities,
) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	if s.registry == nil {
		return false, fmt.Errorf("Protocol Stage registry is nil")
	}
	if _, err := s.registry.ResolveRegistered(source, target, required); err != nil {
		return false, err
	}
	return true, nil
}
