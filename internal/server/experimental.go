package server

import (
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ApplySmartCompact checks if smart_compact should be applied for a scenario
func (s *Server) ApplySmartCompact(scenario typ.RuleScenario) bool {
	cfg := s.config.GetScenarioConfig(scenario)
	if cfg == nil {
		return false
	}
	return cfg.GetDefaultFlags().SmartCompact
}

// ApplyRecording checks if recording should be applied for a scenario
func (s *Server) ApplyRecording(scenario typ.RuleScenario) bool {
	return s.config.IsScenarioRecordingEnabled(scenario)
}

