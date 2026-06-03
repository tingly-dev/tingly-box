package server

import (
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ApplyRecording checks if recording should be applied for a scenario
func (s *Server) ApplyRecording(scenario typ.RuleScenario) bool {
	return s.config.IsScenarioRecordingEnabled(scenario)
}

