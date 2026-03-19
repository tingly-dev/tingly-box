package server

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ShouldRecording determines if recording should be used
func (s *Server) ShouldRecording(recorder *ProtocolRecorder) bool {
	return s.enableRecording && recorder != nil
}

// BuildTransformChainWithRecording constructs a transform chain with recording transforms
func (s *Server) BuildTransformChainWithRecording(c *gin.Context, targetType transform.TargetAPIStyle, providerURL string, scenarioFlags *typ.ScenarioFlags, isStreaming bool, recorder *ProtocolRecorder, recordMode obs.RecordMode) (*transform.TransformChain, error) {

	transforms := []transform.Transform{}

	// 1. Pre-transform recording (if request recording is enabled)
	if recordMode == obs.RecordModeAll || recordMode == obs.RecordModeScenario {
		transforms = append(transforms, NewPreTransformRecorder(c, recorder))
	}

	// 2. Base transform (protocol conversion)
	transforms = append(transforms, transform.NewBaseTransform(targetType))

	// 3. Post-transform recording (if request recording is enabled)
	if recordMode == obs.RecordModeAll || recordMode == obs.RecordModeScenario {
		transforms = append(transforms, NewPostTransformRecorder(recorder, c))
	}

	// 4. Vendor transform
	transforms = append(transforms, transform.NewVendorTransform(providerURL))

	return transform.NewTransformChain(transforms), nil
}

// BuildTransformChainWithoutRecording constructs a transform chain without recording
func (s *Server) BuildTransformChainWithoutRecording(
	targetType transform.TargetAPIStyle,
	providerURL string,
) (*transform.TransformChain, error) {

	transforms := []transform.Transform{}
	transforms = append(transforms, transform.NewBaseTransform(targetType))
	transforms = append(transforms, transform.NewVendorTransform(providerURL))

	return transform.NewTransformChain(transforms), nil
}

// ShouldUseProtocolRecording determines if recording should be used
func (s *Server) ShouldUseProtocolRecording(recorder *ProtocolRecorder) bool {
	return s.enableRecording && recorder != nil
}

// BuildTransformChain builds the appropriate transform chain based on recording configuration
func (s *Server) BuildTransformChain(c *gin.Context, targetType transform.TargetAPIStyle, providerURL string, scenarioFlags *typ.ScenarioFlags, isStreaming bool, recorder *ProtocolRecorder) (*transform.TransformChain, error) {

	// Use recording if enabled and recorder is available
	if s.ShouldUseProtocolRecording(recorder) {
		return s.BuildTransformChainWithRecording(c, targetType, providerURL, scenarioFlags, isStreaming, recorder, s.recordMode)
	}

	// Fall back to non-recording chain
	return s.BuildTransformChainWithoutRecording(targetType, providerURL)
}
