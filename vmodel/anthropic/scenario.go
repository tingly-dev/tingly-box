package anthropic

import "github.com/tingly-dev/tingly-box/vmodel"

// MockScenario is retained as an alias of the common mock configuration.
type MockScenario = vmodel.SharedMockSpec

func NewMockFromScenario(s *MockScenario) VirtualModel {
	cfg := *s
	if cfg.ToolCall != nil {
		call := *cfg.ToolCall
		if call.ID == "" {
			call.ID = "tool_virtual"
		}
		cfg.ToolCall = &call
	}
	cfg.Name = cfg.ID
	if cfg.Description == "" {
		cfg.Description = "A scenario-based virtual model"
	}
	return NewMockModel(&cfg)
}
