package typ

type ScenarioTransport string

const (
	TransportOpenAI    ScenarioTransport = "openai"
	TransportAnthropic ScenarioTransport = "anthropic"
)

type ScenarioDescriptor struct {
	ID                 RuleScenario        `json:"id" yaml:"id"`
	SupportedTransport []ScenarioTransport `json:"supported_transport" yaml:"supported_transport"`
	AllowRuleBinding   bool                `json:"allow_rule_binding" yaml:"allow_rule_binding"`
	AllowDirectPathUse bool                `json:"allow_direct_path_use" yaml:"allow_direct_path_use"`
}

var builtinScenarioDescriptors = []ScenarioDescriptor{
	{
		ID:                 ScenarioOpenAI,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioAnthropic,
		SupportedTransport: []ScenarioTransport{TransportAnthropic},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioGeneral,
		SupportedTransport: []ScenarioTransport{TransportOpenAI, TransportAnthropic},
		AllowRuleBinding:   true,
		AllowDirectPathUse: false,
	},
	{
		ID:                 ScenarioAgent,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioCodex,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioClaudeCode,
		SupportedTransport: []ScenarioTransport{TransportAnthropic},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioOpenCode,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioXcode,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioVSCode,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioSmartGuide,
		SupportedTransport: []ScenarioTransport{TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: true,
	},
	{
		ID:                 ScenarioGlobal,
		SupportedTransport: nil,
		AllowRuleBinding:   false,
		AllowDirectPathUse: false,
	},
}

func BuiltinScenarioDescriptors() []ScenarioDescriptor {
	out := make([]ScenarioDescriptor, len(builtinScenarioDescriptors))
	copy(out, builtinScenarioDescriptors)
	return out
}

func GetScenarioDescriptor(scenario RuleScenario) (ScenarioDescriptor, bool) {
	for _, descriptor := range builtinScenarioDescriptors {
		if descriptor.ID == scenario {
			return descriptor, true
		}
	}
	return ScenarioDescriptor{}, false
}

func IsRegisteredScenario(scenario RuleScenario) bool {
	_, ok := GetScenarioDescriptor(scenario)
	return ok
}

func CanBindRulesToScenario(scenario RuleScenario) bool {
	descriptor, ok := GetScenarioDescriptor(scenario)
	return ok && descriptor.AllowRuleBinding
}

func CanUseScenarioInPath(scenario RuleScenario) bool {
	descriptor, ok := GetScenarioDescriptor(scenario)
	return ok && descriptor.AllowDirectPathUse
}

func ScenarioSupportsTransport(scenario RuleScenario, transport ScenarioTransport) bool {
	descriptor, ok := GetScenarioDescriptor(scenario)
	if !ok {
		return false
	}
	for _, supported := range descriptor.SupportedTransport {
		if supported == transport {
			return true
		}
	}
	return false
}

func TransportForScenarioPath(scenario RuleScenario) (ScenarioTransport, bool) {
	switch scenario {
	case ScenarioAnthropic, ScenarioClaudeCode:
		return TransportAnthropic, true
	case ScenarioOpenAI, ScenarioAgent, ScenarioCodex, ScenarioOpenCode, ScenarioXcode, ScenarioVSCode, ScenarioSmartGuide:
		return TransportOpenAI, true
	default:
		return "", false
	}
}

func RuleLookupScenariosForPath(requested RuleScenario) []RuleScenario {
	candidates := []RuleScenario{requested}
	switch requested {
	case ScenarioOpenAI, ScenarioAnthropic:
		candidates = append(candidates, ScenarioGeneral)
	}
	return candidates
}
