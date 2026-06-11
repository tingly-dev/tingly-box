package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	servertransform "github.com/tingly-dev/tingly-box/internal/server/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ShouldRecording determines if recording should be used
func (s *Server) ShouldRecording(recorder *ProtocolRecorder) bool {
	return recorder != nil
}

// BuildTransformChain assembles the canonical transform chain in a single place,
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
func (s *Server) BuildTransformChain(c *gin.Context, targetType protocol.APIType, providerURL string, scenarioType typ.RuleScenario, scenarioFlags *typ.ScenarioFlags, recorder *ProtocolRecorder, preBase []transform.Transform, preVendor []transform.Transform) (*transform.TransformChain, error) {

	recordMode := s.GetScenarioRecordMode(scenarioType)
	shouldRecord := s.ShouldRecording(recorder)

	var transforms []transform.Transform

	requestRecordingEnabled := recordMode == obs.RecordModeAll ||
		recordMode == obs.RecordModeScenario ||
		recordMode == obs.RecordModeRequestOnly ||
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
	if s.mcpEnabled() {
		transforms = append(transforms, servertransform.NewMCPToolInjectionTransform(s.mcpRuntime))
		transforms = append(transforms, servertransform.NewNativeWebSearchStripTransform(s.mcpRuntime))
		transforms = append(transforms, servertransform.NewMCPToolStripGuardTransform(s.mcpRuntime, s.mcpStripDisabledToolsEnabled()))
	}
	// 3. Consistency transform (cross-provider normalization including message alignment)
	transforms = append(transforms, transform.NewConsistencyTransform(targetType))

	// preVendor slot: rule transforms that act on the converted, upstream-bound
	// shape. Placed after Consistency (so its param clamping still applies) and
	// before Vendor (so Vendor remains the final, immutable step).
	transforms = append(transforms, preVendor...)

	transforms = append(transforms, transform.NewVendorTransform(providerURL))

	// 4. Post-transform recording (if request recording is enabled). Runs last so
	// it snapshots the truly-final request dispatched to the provider.
	if shouldRecord && requestRecordingEnabled {
		transforms = append(transforms, NewTransformRecorder(c, recorder, StagePost))
	}

	return transform.NewTransformChain(transforms), nil
}
