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

// BuildTransformChain builds the appropriate transform chain based on recording configuration
func (s *Server) BuildTransformChain(c *gin.Context, targetType protocol.APIType, providerURL string, scenarioType typ.RuleScenario, scenarioFlags *typ.ScenarioFlags, recorder *ProtocolRecorder) (*transform.TransformChain, error) {

	recordMode := s.GetScenarioRecordMode(scenarioType)
	shouldRecord := s.ShouldRecording(recorder)

	var transforms []transform.Transform

	requestRecordingEnabled := recordMode == obs.RecordModeAll ||
		recordMode == obs.RecordModeScenario ||
		recordMode == obs.RecordModeRequestOnly ||
		recordMode == obs.RecordModeRequestResponse ||
		recordMode == obs.RecordModeStagedRequestResponse

	// 1. Pre-transform recording (if request recording is enabled)
	if shouldRecord && requestRecordingEnabled {
		transforms = append(transforms, NewPreTransformRecorder(c, recorder))
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
	transforms = append(transforms, transform.NewVendorTransform(providerURL))

	// 3. Post-transform recording (if request recording is enabled)
	if shouldRecord && requestRecordingEnabled {
		transforms = append(transforms, NewPostTransformRecorder(recorder, c))
	}

	return transform.NewTransformChain(transforms), nil
}
