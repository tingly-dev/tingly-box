package typ

import (
	"fmt"
	"slices"
	"sync"
)

type ScenarioTransport string

const (
	TransportOpenAI    ScenarioTransport = "openai"
	TransportAnthropic ScenarioTransport = "anthropic"
)

type ScenarioDescriptor struct {
	// ID is the stable scenario identifier stored on rules and scenario configs.
	ID RuleScenario `json:"id" yaml:"id"`
	// SupportedTransport declares which protocol surfaces may resolve rules bound to this scenario.
	SupportedTransport []ScenarioTransport `json:"supported_transport" yaml:"supported_transport"`
	// AllowRuleBinding controls whether API/CLI callers may create or update rules under this scenario.
	AllowRuleBinding bool `json:"allow_rule_binding" yaml:"allow_rule_binding"`
	// AllowDirectPathUse controls whether scenario-scoped HTTP paths like /openai/{scenario}/... are valid.
	AllowDirectPathUse bool `json:"allow_direct_path_use" yaml:"allow_direct_path_use"`
}

var (
	scenarioRegistryMu sync.RWMutex
	scenarioRegistry   = map[RuleScenario]ScenarioDescriptor{}
)

func init() {
	for _, descriptor := range BuiltinScenarioDescriptors() {
		scenarioRegistry[descriptor.ID] = cloneScenarioDescriptor(descriptor)
	}
}

func BuiltinScenarioDescriptors() []ScenarioDescriptor {
	descriptors := make([]ScenarioDescriptor, 0, len(BuiltinScenarios()))
	for _, scenario := range BuiltinScenarios() {
		descriptors = append(descriptors, builtinScenarioDescriptorFor(scenario))
	}
	return descriptors
}

func builtinScenarioDescriptorFor(scenario RuleScenario) ScenarioDescriptor {
	switch scenario {
	case ScenarioOpenAI:
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: []ScenarioTransport{TransportOpenAI},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioAnthropic:
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: []ScenarioTransport{TransportAnthropic},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioAgent, ScenarioCodex, ScenarioOpenCode, ScenarioXcode, ScenarioVSCode, ScenarioSmartGuide:
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: []ScenarioTransport{TransportOpenAI},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioClaudeCode:
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: []ScenarioTransport{TransportAnthropic},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioGlobal:
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: nil,
			AllowRuleBinding:   false,
			AllowDirectPathUse: false,
		}
	default:
		return ScenarioDescriptor{ID: scenario}
	}
}

func cloneScenarioDescriptor(descriptor ScenarioDescriptor) ScenarioDescriptor {
	out := descriptor
	out.SupportedTransport = slices.Clone(descriptor.SupportedTransport)
	return out
}

func RegisterScenario(descriptor ScenarioDescriptor) error {
	if descriptor.ID == "" {
		return fmt.Errorf("scenario id is required")
	}

	descriptor = cloneScenarioDescriptor(descriptor)

	scenarioRegistryMu.Lock()
	defer scenarioRegistryMu.Unlock()

	if existing, ok := scenarioRegistry[descriptor.ID]; ok {
		if existing.AllowRuleBinding == descriptor.AllowRuleBinding &&
			existing.AllowDirectPathUse == descriptor.AllowDirectPathUse &&
			slices.Equal(existing.SupportedTransport, descriptor.SupportedTransport) {
			return nil
		}
		return fmt.Errorf("scenario %s already registered with different descriptor", descriptor.ID)
	}

	scenarioRegistry[descriptor.ID] = descriptor
	return nil
}

func RegisteredScenarioDescriptors() []ScenarioDescriptor {
	scenarioRegistryMu.RLock()
	defer scenarioRegistryMu.RUnlock()

	descriptors := make([]ScenarioDescriptor, 0, len(scenarioRegistry))
	for _, descriptor := range scenarioRegistry {
		descriptors = append(descriptors, cloneScenarioDescriptor(descriptor))
	}
	slices.SortFunc(descriptors, func(a, b ScenarioDescriptor) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return descriptors
}

func GetScenarioDescriptor(scenario RuleScenario) (ScenarioDescriptor, bool) {
	scenarioRegistryMu.RLock()
	defer scenarioRegistryMu.RUnlock()

	descriptor, ok := scenarioRegistry[scenario]
	if !ok {
		return ScenarioDescriptor{}, false
	}
	return cloneScenarioDescriptor(descriptor), true
}

func CanBindRulesToScenario(scenario RuleScenario) bool {
	descriptor, ok := GetScenarioDescriptor(scenario)
	return ok && descriptor.AllowRuleBinding
}

func CanUseScenarioInPath(scenario RuleScenario) bool {
	descriptor, ok := GetScenarioDescriptor(scenario)
	return ok && descriptor.AllowDirectPathUse
}
