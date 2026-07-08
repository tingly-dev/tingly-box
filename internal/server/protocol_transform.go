package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/recording"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	servertransform "github.com/tingly-dev/tingly-box/internal/server/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (ph *ProtocolHandler) TransformAnthropicBeta(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *recording.ProtocolRecorder, scenarioType typ.RuleScenario, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {

	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := ph.buildTransformChain(c, target, scenarioType, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		flags := scenarioConfig.GetDefaultFlags()
		scenarioFlags = &flags
		if flags.SmartCompact {
			baseTransforms := chain.GetTransforms()
			chain.SetTransforms(append(
				[]transform.Transform{smart_compact.NewCompactTransform(2)},
				baseTransforms...,
			))
		}
	}

	opts := []transform.TransformOption{
		transform.WithContext(c.Request.Context()),
		transform.WithProvider(provider),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(ph.deps.Config.ClaudeCodeDeviceID),
	}

	// Advisor loopback requests carry X-Tingly-Advisor-Depth >= 1; skip MCP tool injection for them
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		opts = append(opts, transform.WithIsAdvisorRequest(true))
	}

	transformCtx := transform.NewTransformContext(
		req.BetaMessageNewParams,
		opts...,
	)
	transformCtx.HasNativeAdvisor = HasNativeAdvisorBeta(req)
	transformCtx.SourceAPI = protocol.TypeAnthropicBeta
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}

	return finalCtx, nil
}

func (ph *ProtocolHandler) TransformAnthropicV1(c *gin.Context, req *protocol.AnthropicMessagesRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *recording.ProtocolRecorder, scenarioType typ.RuleScenario, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {
	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := ph.buildTransformChain(c, target, scenarioType, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		flags := scenarioConfig.GetDefaultFlags()
		scenarioFlags = &flags
		if flags.SmartCompact {
			baseTransforms := chain.GetTransforms()
			chain.SetTransforms(append(
				[]transform.Transform{smart_compact.NewCompactTransform(2)},
				baseTransforms...,
			))
		}
	}

	opts := []transform.TransformOption{
		transform.WithProvider(provider),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(ph.deps.Config.ClaudeCodeDeviceID),
	}

	// Check if this is an advisor request
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		opts = append(opts, transform.WithIsAdvisorRequest(true))
	}

	transformCtx := transform.NewTransformContext(
		req.MessageNewParams,
		opts...,
	)
	transformCtx.SourceAPI = protocol.TypeAnthropicV1
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		if protocolRecorder != nil {
			protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
			protocolRecorder.RecordError(err)
		}
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}
	return finalCtx, nil
}

func (ph *ProtocolHandler) TransformOpenAIChat(c *gin.Context, req *protocol.OpenAIChatCompletionRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *recording.ProtocolRecorder, scenarioType typ.RuleScenario, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {
	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := ph.buildTransformChain(c, target, scenarioType, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		scenarioFlags = &scenarioConfig.Flags
	}

	opts := []transform.TransformOption{
		transform.WithProvider(provider),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(ph.deps.Config.ClaudeCodeDeviceID),
	}

	// Check if this is an advisor request
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		opts = append(opts, transform.WithIsAdvisorRequest(true))
	}

	transformCtx := transform.NewTransformContext(
		req.ChatCompletionNewParams,
		opts...,
	)
	transformCtx.SourceAPI = protocol.TypeOpenAIChat
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		if protocolRecorder != nil {
			protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
			protocolRecorder.RecordError(err)
		}
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}
	return finalCtx, nil
}

func (ph *ProtocolHandler) TransformOpenAIResponses(c *gin.Context, req *protocol.ResponseCreateRequest, target protocol.APIType, provider *typ.Provider, isStreaming bool, protocolRecorder *recording.ProtocolRecorder, scenarioType typ.RuleScenario, maxAllowed int, preBaseTransforms []transform.Transform, preVendorTransforms []transform.Transform) (*transform.TransformContext, error) {
	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain, err := ph.buildTransformChain(c, target, scenarioType, protocolRecorder, preBaseTransforms, preVendorTransforms)
	if err != nil {
		return nil, err
	}

	// Create transform context
	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		scenarioFlags = &scenarioConfig.Flags
	}

	opts := []transform.TransformOption{
		transform.WithProvider(provider),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(ph.deps.Config.ClaudeCodeDeviceID),
		transform.WithMaxTokens(int64(maxAllowed)),
	}

	transformCtx := transform.NewTransformContext(
		req.ResponseNewParams,
		opts...,
	)
	transformCtx.SourceAPI = protocol.TypeOpenAIResponses
	transformCtx.TargetAPI = target

	// Execute transform chain
	finalCtx, err := chain.Execute(transformCtx)
	if err != nil {
		if protocolRecorder != nil {
			protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
			protocolRecorder.RecordError(err)
		}
		return nil, err
	}

	// Store transform steps in V2 recorder
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(finalCtx.TransformSteps)
	}
	return finalCtx, nil
}

// buildTransformChain assembles the canonical transform chain in a single place,
// slotting the rule-driven transforms into the two named positions — preBase and
// preVendor — that bracket the protocol conversion and the vendor finalize:
//
//	preBase slot   : preBase rule transforms (act on the client's original shape)
//	StagePre-record (if enabled)
//	Base           (protocol conversion)
//	MCP            (inject / native-websearch-strip / strip-guard) [if mcpEnabled]
//	Consistency    (cross-provider normalization, param clamping)
//	preVendor slot : preVendor rule transforms (act on the converted, upstream-bound shape)
//	Vendor         (provider-specific finalize)
//	StagePost-record (if enabled)
//
// Invariant: nothing runs after Vendor except recording. Vendor directly faces
// the provider and must be the last mutation, so the preVendor transforms are
// inserted after Consistency but BEFORE Vendor — this also means the StagePost
// recording captures the truly-final, dispatched request.
func (ph *ProtocolHandler) buildTransformChain(c *gin.Context, targetType protocol.APIType, scenarioType typ.RuleScenario, recorder *recording.ProtocolRecorder, preBase []transform.Transform, preVendor []transform.Transform) (*transform.TransformChain, error) {

	recordMode := ph.getScenarioRecordMode(scenarioType)
	shouldRecord := recorder != nil

	var transforms []transform.Transform

	requestRecordingEnabled := recordMode == obs.RecordModeRequestOnly ||
		recordMode == obs.RecordModeRequestResponse ||
		recordMode == obs.RecordModeStagedRequestResponse

	// preBase slot: rule transforms that act on the inbound request shape, before
	// any protocol conversion (and before recording, so the type-switch in each
	// transform sees what the client actually sent).
	transforms = append(transforms, preBase...)

	// 1. Pre-transform recording (if request recording is enabled)
	if shouldRecord && requestRecordingEnabled {
		transforms = append(transforms, NewTransformRecorder(c, recorder, StagePre))
	}

	// 2. Base transform (protocol conversion)
	transforms = append(transforms, transform.NewBaseTransform(targetType))
	if ph.mcpEnabled() {
		transforms = append(transforms, servertransform.NewMCPToolInjectionTransform(ph.deps.MCPRuntime))
		transforms = append(transforms, servertransform.NewNativeWebSearchStripTransform(ph.deps.MCPRuntime))
		transforms = append(transforms, servertransform.NewMCPToolStripGuardTransform(ph.deps.MCPRuntime, ph.mcpStripDisabledToolsEnabled()))
	}
	// 3. Consistency transform (cross-provider normalization including message alignment)
	transforms = append(transforms, transform.NewConsistencyTransform(targetType))

	// preVendor slot: rule transforms that act on the converted, upstream-bound
	// shape. Placed after Consistency (so its param clamping still applies) and
	// before Vendor (so Vendor remains the final, immutable step).
	transforms = append(transforms, preVendor...)

	transforms = append(transforms, transform.NewVendorTransform())

	// 4. Post-transform recording (if request recording is enabled). Runs last so
	// it snapshots the truly-final request dispatched to the provider.
	if shouldRecord && requestRecordingEnabled {
		transforms = append(transforms, NewTransformRecorder(c, recorder, StagePost))
	}

	return transform.NewTransformChain(transforms), nil
}

// buildAnthropicPreChain constructs the pre-request transform chain for Anthropic V1 and Beta handlers.
// Currently only applies MaxTokens validation.
// All other scenario-level transforms (ThinkingEffort, CleanHeader) are handled via
// rule flags injection in resolveRuleFlagsWithScenario.
func buildAnthropicPreChain(
	scenarioConfig *typ.ScenarioConfig,
	defaultMaxTokens, maxAllowed int,
) []transform.Transform {
	var chain []transform.Transform
	// Only MaxTokens validation remains at scenario level
	chain = append(chain, servertransform.NewMaxTokensTransform(defaultMaxTokens, maxAllowed))
	return chain
}

// scenarioFlagsOrNil returns the scenario flags or nil.
func scenarioFlagsOrNil(scenarioConfig *typ.ScenarioConfig) *typ.ScenarioFlags {
	if scenarioConfig != nil {
		return &scenarioConfig.Flags
	}
	return nil
}

// ExecuteAnthropicV1PreChain builds and runs the pre-transform chain for Anthropic V1 requests.
// Returns an error that should be mapped to HTTP 400.
func ExecuteAnthropicV1PreChain(
	req *anthropic.MessageNewParams,
	scenarioConfig *typ.ScenarioConfig,
	defaultMaxTokens, maxAllowed int,
	isStreaming bool,
) error {
	transforms := buildAnthropicPreChain(scenarioConfig, defaultMaxTokens, maxAllowed)
	ctx := transform.NewTransformContext(
		req,
		transform.WithScenarioFlags(scenarioFlagsOrNil(scenarioConfig)),
		transform.WithStreaming(isStreaming),
	)
	if len(transforms) == 0 {
		return nil
	}
	_, err := transform.NewTransformChain(transforms).Execute(ctx)
	return err
}

// ExecuteAnthropicBetaPreChain builds and runs the pre-transform chain for Anthropic Beta requests.
// Returns an error that should be mapped to HTTP 400.
func ExecuteAnthropicBetaPreChain(
	req *anthropic.BetaMessageNewParams,
	scenarioConfig *typ.ScenarioConfig,
	defaultMaxTokens, maxAllowed int,
	isStreaming bool,
) error {
	transforms := buildAnthropicPreChain(scenarioConfig, defaultMaxTokens, maxAllowed)
	ctx := transform.NewTransformContext(
		req,
		transform.WithScenarioFlags(scenarioFlagsOrNil(scenarioConfig)),
		transform.WithStreaming(isStreaming),
	)
	if len(transforms) == 0 {
		return nil
	}
	_, err := transform.NewTransformChain(transforms).Execute(ctx)
	return err
}
