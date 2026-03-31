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
	return s.enableRecording && recorder != nil
}

// BuildTransformChain builds the appropriate transform chain based on recording configuration
func (s *Server) BuildTransformChain(c *gin.Context, targetType protocol.APIType, providerURL string, scenarioFlags *typ.ScenarioFlags, recorder *ProtocolRecorder) (*transform.TransformChain, error) {

	recordMode := s.recordMode
	shouldRecord := s.ShouldRecording(recorder)

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

	// 4. Vendor transform
	transforms = append(transforms, transform.NewVendorTransform(providerURL))

	return transform.NewTransformChain(transforms), nil
}
