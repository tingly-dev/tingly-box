package server

import (
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Experimental feature flag names
const (
	ExperimentalFeatureSmartCompact = "smart_compact"
	ExperimentalFeatureRecording    = "recording"
)

// IsExperimentalFeatureEnabled checks if an experimental feature is enabled for a scenario
func (s *Server) IsExperimentalFeatureEnabled(scenario typ.RuleScenario, feature string) bool {
	if s.config == nil {
		return false
	}
	return s.config.GetScenarioFlag(scenario, feature)
}

// ApplySmartCompact checks if smart_compact should be applied for a scenario
func (s *Server) ApplySmartCompact(scenario typ.RuleScenario) bool {
	return s.IsExperimentalFeatureEnabled(scenario, ExperimentalFeatureSmartCompact)
}

// ApplyRecording checks if recording should be applied for a scenario
func (s *Server) ApplyRecording(scenario typ.RuleScenario) bool {
	return s.IsExperimentalFeatureEnabled(scenario, ExperimentalFeatureRecording)
}
