package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ShouldRecording determines if recording should be used
func (s *Server) ShouldRecording(recorder *ProtocolRecorder) bool {
	return recorder != nil
}

// ShouldRecordingV3 determines if V3 recording should be used
func (s *Server) ShouldRecordingV3(recorder *UnifiedRecorder) bool {
	return recorder != nil
}

// BuildTransformChain builds the appropriate transform chain based on recording configuration
func (s *Server) BuildTransformChain(c *gin.Context, targetType protocol.APIType, providerURL string, scenarioFlags *typ.ScenarioFlags, recorder *UnifiedRecorder) (*transform.TransformChain, error) {

	recordMode := s.recordMode
	shouldRecord := s.ShouldRecordingV3(recorder)

	var transforms []transform.Transform

	// 1. Pre-transform recording (if request recording is enabled)
	if shouldRecord && (recordMode == obs.RecordModeAll || recordMode == obs.RecordModeScenario) {
		transforms = append(transforms, NewPreTransformRecorder(c, recorder))
	}

	// 2. Base transform (protocol conversion)
	transforms = append(transforms, transform.NewBaseTransform(targetType))
	transforms = append(transforms, transform.NewVendorTransform(providerURL))

	// 3. Post-transform recording (if request recording is enabled)
	if shouldRecord && (recordMode == obs.RecordModeAll || recordMode == obs.RecordModeScenario) {
		transforms = append(transforms, NewPostTransformRecorder(recorder, c))
	}

	return transform.NewTransformChain(transforms), nil
}
