package protocolserver

import (
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/recording"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	servertransform "github.com/tingly-dev/tingly-box/internal/protocolserver/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Stateless transform singletons shared across requests. BaseTransform and
// ConsistencyTransform are pure functions of the target type, and
// VendorTransform carries no state at all, so rebuilding them per request
// only produced allocation churn on the core forwarding path.
var (
	baseTransformCache        sync.Map // protocol.APIType -> *transform.BaseTransform
	consistencyTransformCache sync.Map // protocol.APIType -> *transform.ConsistencyTransform
	vendorTransformShared     = transform.NewVendorTransform()
)

func baseTransformFor(targetType protocol.APIType) *transform.BaseTransform {
	if v, ok := baseTransformCache.Load(targetType); ok {
		return v.(*transform.BaseTransform)
	}
	v, _ := baseTransformCache.LoadOrStore(targetType, transform.NewBaseTransform(targetType))
	return v.(*transform.BaseTransform)
}

func consistencyTransformFor(targetType protocol.APIType) *transform.ConsistencyTransform {
	if v, ok := consistencyTransformCache.Load(targetType); ok {
		return v.(*transform.ConsistencyTransform)
	}
	v, _ := consistencyTransformCache.LoadOrStore(targetType, transform.NewConsistencyTransform(targetType))
	return v.(*transform.ConsistencyTransform)
}

// mcpTransformCache lazily builds the MCP transforms once per handler. They
// only hold the (construction-time-fixed) MCP runtime pointer plus the strip
// flag, so both guard variants are prebuilt and selected per request.
type mcpTransformCache struct {
	once      sync.Once
	injection transform.Transform
	strip     transform.Transform
	guardOn   transform.Transform
	guardOff  transform.Transform
}

func (ph *ProtocolHandler) mcpChainTransforms(stripEnabled bool) []transform.Transform {
	ph.mcpTC.once.Do(func() {
		rt := ph.deps.MCPRuntime
		ph.mcpTC.injection = servertransform.NewMCPToolInjectionTransform(rt)
		ph.mcpTC.strip = servertransform.NewNativeWebSearchStripTransform(rt)
		ph.mcpTC.guardOn = servertransform.NewMCPToolStripGuardTransform(rt, true)
		ph.mcpTC.guardOff = servertransform.NewMCPToolStripGuardTransform(rt, false)
	})
	guard := ph.mcpTC.guardOff
	if stripEnabled {
		guard = ph.mcpTC.guardOn
	}
	return []transform.Transform{ph.mcpTC.injection, ph.mcpTC.strip, guard}
}

// transformSourceOptions carries the per-source knobs that differ between the
// four Transform* entry points; the rest of the pipeline (chain build, flag
// resolution, context setup, execution, recording) is shared by
// transformRequest.
type transformSourceOptions struct {
	source protocol.APIType

	// defaultScenarioFlags selects how scenario flags are resolved: the
	// Anthropic entry points go through ScenarioConfig.GetDefaultFlags()
	// while the OpenAI entry points use the raw ScenarioConfig.Flags pointer.
	defaultScenarioFlags bool

	hasNativeAdvisor bool

	// extraOpts are appended after the shared options: WithMaxTokens for the
	// Responses path, and WithContext for the Beta path. The latter is
	// preserved drift, not design — historically only the Beta entry point
	// passed the request context into the transform chain (the MCP transforms
	// fall back to context.Background() on the other paths).
	extraOpts []transform.TransformOption
}

// transformRequest is the shared core of the four Transform* entry points:
// build the canonical chain, resolve scenario flags, assemble the
// TransformContext, execute, and mirror steps/errors into the recorder.
// Generic (free function — Go methods cannot have type parameters) so the
// compile-time RequestUnionConstraint on NewTransformContext is preserved.
//
// When the plan is served by the Stage pipeline only the source half of the
// chain runs here: the request keeps the client's protocol, and the pipeline
// converts it and runs the target half per provider call
// (targetTransformStage).
func transformRequest[T transform.RequestUnionConstraint](ph *ProtocolHandler, c *gin.Context, req T, plan *attemptPlan, isStreaming bool, scenarioType typ.RuleScenario, src transformSourceOptions) (*transform.TransformContext, error) {
	protocolRecorder := recording.FromGin(c)
	target, provider := plan.Target, plan.Provider
	// Build transform chain with recording support. The rule-driven pre-Base and
	// preVendor transforms are slotted into their canonical positions by the builder.
	chain := ph.buildTransformChain(c, plan)
	if plan.servedByStage() {
		chain = transform.NewTransformChain(ph.sourceTransforms(c, plan))
	}

	var scenarioFlags *typ.ScenarioFlags
	if scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType); scenarioConfig != nil {
		if src.defaultScenarioFlags {
			flags := scenarioConfig.GetDefaultFlags()
			scenarioFlags = &flags
		} else {
			scenarioFlags = &scenarioConfig.Flags
		}
	}

	opts := []transform.TransformOption{
		transform.WithProvider(provider),
		transform.WithScenarioFlags(scenarioFlags),
		transform.WithStreaming(isStreaming),
		transform.WithDevice(ph.deps.Config.ClaudeCodeDeviceID),
	}
	opts = append(opts, src.extraOpts...)

	// Advisor loopback requests carry X-Tingly-Advisor-Depth >= 1; mark them
	// so MCP tool injection is skipped. Advisor loopbacks are Anthropic/Chat
	// shaped, so on the Responses path this is a no-op.
	if c.GetHeader("X-Tingly-Advisor-Depth") != "" {
		opts = append(opts, transform.WithIsAdvisorRequest(true))
	}

	transformCtx := transform.NewTransformContext(req, opts...)
	transformCtx.HasNativeAdvisor = src.hasNativeAdvisor
	transformCtx.SourceAPI = src.source
	transformCtx.TargetAPI = target

	finalCtx, execErr := chain.Execute(transformCtx)

	// Mirror transform steps (and any failure) into the V2 recorder. Steps are
	// read from transformCtx, which Execute mutates in place: on failure it
	// returns a nil finalCtx, but transformCtx still holds every step up to and
	// including the one that failed.
	if protocolRecorder != nil {
		protocolRecorder.SetTransformSteps(transformCtx.TransformSteps)
		if execErr != nil {
			protocolRecorder.RecordError(execErr)
		}
	}
	if execErr != nil {
		return nil, execErr
	}
	// Response-shaping hints for the dispatch layer (ShouldStripUsage). Only
	// the OpenAI-client paths read them today.
	finalCtx.Extra["cursor_compat"] = plan.Flags.CursorCompat
	finalCtx.Extra["skip_usage"] = plan.Flags.SkipUsage
	return finalCtx, nil
}

func (ph *ProtocolHandler) TransformAnthropicBeta(c *gin.Context, req *protocol.AnthropicBetaMessagesRequest, plan *attemptPlan, isStreaming bool, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {
	return transformRequest(ph, c, req.BetaMessageNewParams, plan, isStreaming, scenarioType, transformSourceOptions{
		source:               protocol.TypeAnthropicBeta,
		defaultScenarioFlags: true,
		hasNativeAdvisor:     HasNativeAdvisorBeta(req),
		extraOpts:            []transform.TransformOption{transform.WithContext(c.Request.Context())},
	})
}

func (ph *ProtocolHandler) TransformAnthropicV1(c *gin.Context, req *protocol.AnthropicMessagesRequest, plan *attemptPlan, isStreaming bool, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {
	return transformRequest(ph, c, req.MessageNewParams, plan, isStreaming, scenarioType, transformSourceOptions{
		source:               protocol.TypeAnthropicV1,
		defaultScenarioFlags: true,
	})
}

func (ph *ProtocolHandler) TransformOpenAIChat(c *gin.Context, req *protocol.OpenAIChatCompletionRequest, plan *attemptPlan, isStreaming bool, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {
	return transformRequest(ph, c, req.ChatCompletionNewParams, plan, isStreaming, scenarioType, transformSourceOptions{
		source: protocol.TypeOpenAIChat,
	})
}

func (ph *ProtocolHandler) TransformOpenAIResponses(c *gin.Context, req *protocol.ResponseCreateRequest, plan *attemptPlan, isStreaming bool, scenarioType typ.RuleScenario) (*transform.TransformContext, error) {
	return transformRequest(ph, c, req.ResponseNewParams, plan, isStreaming, scenarioType, transformSourceOptions{
		source:    protocol.TypeOpenAIResponses,
		extraOpts: []transform.TransformOption{transform.WithMaxTokens(int64(plan.MaxAllowed))},
	})
}

// buildTransformChain assembles the canonical transform chain in a single place,
// slotting the rule-driven transforms into the two named positions — preBase and
// preVendor — that bracket the protocol conversion and the vendor finalize:
//
//	max_tokens_default : fill Anthropic's required max_tokens (client shape)
//	preBase slot       : preBase rule transforms (act on the client's original shape)
//	StagePre-record    (if enabled)
//	Base               (protocol conversion)
//	output_limit       : model output limit on the upstream-bound shape
//	MCP                (inject / native-websearch-strip / strip-guard) [if mcpEnabled]
//	Consistency        (cross-provider normalization, param clamping)
//	preVendor slot     : preVendor rule transforms (act on the converted, upstream-bound shape)
//	Vendor             (provider-specific finalize)
//	StagePost-record   (if enabled)
//
// Invariant: nothing runs after Vendor except recording. Vendor directly faces
// the provider and must be the last mutation, so the preVendor transforms are
// inserted after Consistency but BEFORE Vendor — this also means the StagePost
// recording captures the truly-final, dispatched request.
//
// Which half a step belongs to is set by .design/protocol-stage-pipeline.md.
func (ph *ProtocolHandler) buildTransformChain(c *gin.Context, plan *attemptPlan) *transform.TransformChain {
	transforms := ph.sourceTransforms(c, plan)
	// Base transform (protocol conversion)
	transforms = append(transforms, baseTransformFor(plan.Target))
	transforms = append(transforms, ph.targetTransforms(c, plan)...)
	return transform.NewTransformChain(transforms)
}

// sourceTransforms is the part of the chain that acts on the client's own
// request shape, before any protocol conversion.
func (ph *ProtocolHandler) sourceTransforms(c *gin.Context, plan *attemptPlan) []transform.Transform {
	recorder := recording.FromGin(c)

	var transforms []transform.Transform

	// Anthropic requires max_tokens; fill it before anything reads the request.
	if plan.DefaultMaxTokens > 0 {
		transforms = append(transforms, servertransform.NewMaxTokensDefaultTransform(plan.DefaultMaxTokens))
	}

	// preBase slot: rule transforms that act on the inbound request shape, before
	// any protocol conversion (and before recording, so the type-switch in each
	// transform sees what the client actually sent).
	transforms = append(transforms, plan.PreBase...)

	// Pre-transform recording — snapshots the inbound (client) request.
	// Gated on the recorder's own capture-point selection (nil-safe).
	if recorder.Wants(typ.RecordClientRequest) {
		transforms = append(transforms, NewTransformRecorder(c, recorder, StagePre))
	}
	return transforms
}

// targetTransforms is the part of the chain that acts on the converted,
// upstream-bound request, after protocol conversion. The Stage pipeline runs it
// per provider call (targetTransformStage).
func (ph *ProtocolHandler) targetTransforms(c *gin.Context, plan *attemptPlan) []transform.Transform {
	recorder := recording.FromGin(c)

	var transforms []transform.Transform

	// Model output limit, first so Consistency validates the bounded request.
	if plan.MaxAllowed > 0 {
		transforms = append(transforms, servertransform.NewOutputLimitTransform(plan.MaxAllowed))
	}

	if ph.mcpEnabled() {
		if mcpServesPair(plan.Source, plan.Target) {
			transforms = append(transforms, ph.mcpChainTransforms(ph.mcpStripDisabledToolsEnabled())...)
		} else {
			noteMCPSkipped(c, plan.Source, plan.Target)
		}
	}
	// Consistency transform (cross-provider normalization including message alignment)
	transforms = append(transforms, consistencyTransformFor(plan.Target))

	// preVendor slot: rule transforms that act on the converted, upstream-bound
	// shape. Placed after Consistency (so its param clamping still applies) and
	// before Vendor (so Vendor remains the final, immutable step).
	transforms = append(transforms, plan.PreVendor...)

	transforms = append(transforms, vendorTransformShared)

	// Post-transform recording — snapshots the outbound (upstream) request.
	// Runs last so it captures the truly-final request dispatched to the provider.
	if recorder.Wants(typ.RecordUpstreamRequest) {
		transforms = append(transforms, NewTransformRecorder(c, recorder, StagePost))
	}
	return transforms
}

// mcpServesPair reports whether server tools (MCP) are offered for a client
// protocol on a provider protocol. They are offered only where their calls
// are also executed. Among the OpenAI pairs only Chat on Chat has a tool loop;
// on Chat on Responses, Responses on Chat and Responses on Responses injecting
// would leak the calls to the client or offer tools nobody runs, so MCP is
// skipped there and the request passes through untouched.
func mcpServesPair(source, target protocol.APIType) bool {
	openAI := func(api protocol.APIType) bool {
		return api == protocol.TypeOpenAIChat || api == protocol.TypeOpenAIResponses
	}
	if openAI(source) && openAI(target) {
		return source == protocol.TypeOpenAIChat && target == protocol.TypeOpenAIChat
	}
	return true
}

// noteMCPSkipped makes a skipped MCP visible: a debug log line, and for a
// debug-routing request an X-Tingly-MCP response header with the reason.
func noteMCPSkipped(c *gin.Context, source, target protocol.APIType) {
	reason := fmt.Sprintf("skipped: server tools are not supported for %s clients on %s providers", source, target)
	if c == nil {
		logrus.Debugf("MCP %s", reason)
		return
	}
	logrus.WithContext(c.Request.Context()).Debugf("MCP %s", reason)
	if c.GetHeader("X-Tingly-Debug-Routing") == "1" {
		c.Header("X-Tingly-MCP", reason)
	}
}
