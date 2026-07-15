package typ

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
)

type ScenarioTransport string

const (
	TransportOpenAI    ScenarioTransport = "openai"
	TransportAnthropic ScenarioTransport = "anthropic"
	TransportEmbed     ScenarioTransport = "embed"
	TransportImageGen  ScenarioTransport = "imagegen"
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
	// SupportsProfiles indicates whether this scenario supports named profiles.
	SupportsProfiles bool `json:"supports_profiles" yaml:"supports_profiles"`
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
			SupportedTransport: []ScenarioTransport{TransportOpenAI, TransportEmbed, TransportImageGen},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioEmbed:
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: []ScenarioTransport{TransportEmbed},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioImageGen:
		return ScenarioDescriptor{
			ID: scenario,
			// Two parallel surfaces: TransportOpenAI for /responses & /chat/completions
			// (image_generation tool), TransportImageGen for /images/generations.
			// The caller chooses; tingly-box does not probe the upstream.
			SupportedTransport: []ScenarioTransport{TransportOpenAI, TransportImageGen},
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
	case ScenarioAgent, ScenarioClaudeDesktop, ScenarioSmartGuide:
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: []ScenarioTransport{TransportOpenAI},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioTeam:
		// Centrally deployed model shared across a team. Accepts both OpenAI
		// and Anthropic transports so any agent/client can point at
		// /tingly/team regardless of protocol.
		return ScenarioDescriptor{
			ID:                 scenario,
			SupportedTransport: []ScenarioTransport{TransportOpenAI, TransportAnthropic},
			AllowRuleBinding:   true,
			AllowDirectPathUse: true,
		}
	case ScenarioCodex, ScenarioOpenCode, ScenarioXcode, ScenarioVSCode:
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
			SupportsProfiles:   true,
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
	base, profileID := ParseScenarioProfile(scenario)
	if profileID != "" {
		// Profiled scenario: resolve from base scenario's descriptor
		return getBaseScenarioDescriptor(base)
	}

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

// ScenarioSupportsTransport reports whether the given scenario's descriptor
// declares support for the specified transport.
func ScenarioSupportsTransport(scenario RuleScenario, transport ScenarioTransport) bool {
	descriptor, ok := GetScenarioDescriptor(scenario)
	if !ok {
		return false
	}
	return slices.Contains(descriptor.SupportedTransport, transport)
}

// Base returns the base scenario, stripping any profile suffix.
// "claude_code:p1".Base() == "claude_code"; "claude_code".Base() == "claude_code".
func (s RuleScenario) Base() RuleScenario {
	base, _ := ParseScenarioProfile(s)
	return base
}

// Is reports whether the scenario's base equals the given base scenario.
// Equivalent to s.Base() == base, but reads more naturally at call sites.
func (s RuleScenario) Is(base RuleScenario) bool {
	return s.Base() == base
}

// ProfileSeparator is used to split "scenario:profile_id" strings.
const ProfileSeparator = ":"

// ParseScenarioProfile splits "claude_code:p1" into base scenario and profile ID.
// "claude_code" returns ("claude_code", "").
func ParseScenarioProfile(raw RuleScenario) (base RuleScenario, profileID string) {
	rawStr := string(raw)
	if idx := strings.Index(rawStr, ProfileSeparator); idx >= 0 {
		return RuleScenario(rawStr[:idx]), rawStr[idx+1:]
	}
	return raw, ""
}

// IsProfiledScenario returns true if the scenario string contains a profile suffix.
func IsProfiledScenario(raw RuleScenario) bool {
	return strings.Contains(string(raw), ProfileSeparator)
}

// ProfiledScenarioName combines base scenario and profile ID into "base:profileID".
func ProfiledScenarioName(base RuleScenario, profileID string) RuleScenario {
	return RuleScenario(string(base) + ProfileSeparator + profileID)
}

// IsSimpleProfileAlias reports whether s is a URL-friendly profile alias that
// the profile-alias middleware is allowed to resolve to a profile ID.
//
// A profile is addressed as the "<alias>" half of "/tingly/<base>:<alias>", so
// the alias has to be a clean URL path token. We whitelist the RFC 3986
// unreserved slug subset — ASCII letters, digits, '-' and '_' — which needs no
// escaping and contains no path/profile separators. Names that fail this check
// are not routable by name; callers must address those profiles by ID.
func IsSimpleProfileAlias(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '_', ch == '-':
		default:
			return false
		}
	}
	return true
}

// looksLikeProfileID reports whether name has the reserved auto-generated ID
// shape "p"<digits> (e.g. "p1", "p07"). Allowing such a name would let it
// shadow ID-based routing, since ResolveProfileAlias matches IDs first. IDs are
// minted as fmt.Sprintf("p%d", n), so a trailing unsigned integer is exactly
// the shape to reserve.
func looksLikeProfileID(name string) bool {
	rest, ok := strings.CutPrefix(name, "p")
	if !ok {
		return false
	}
	_, err := strconv.ParseUint(rest, 10, 64)
	return err == nil
}

// ValidateProfileName enforces, at profile creation/rename time, that a name is
// a simple alias usable directly in a route ("/tingly/<base>:<name>"). Pushing
// the constraint to the write path is the primary defense: every stored
// profile is then guaranteed routable by name, and IsSimpleProfileAlias on the
// routing path only has to guard legacy data created before this check. The
// "default" and the "p"<digits> ID shape are additionally reserved so user
// names cannot collide with system-managed profile identities.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if !IsSimpleProfileAlias(name) {
		return fmt.Errorf("profile name %q must contain only letters, digits, '-' and '_' so it can be used directly in a route like '/tingly/<scenario>:%s'", name, name)
	}
	if strings.EqualFold(name, "default") {
		return fmt.Errorf("profile name %q is reserved for the local default profile", name)
	}
	if looksLikeProfileID(name) {
		return fmt.Errorf("profile name %q is reserved: it has the shape of an auto-generated profile ID — choose a different name", name)
	}
	return nil
}

// getBaseScenarioDescriptor resolves the descriptor for a plain (non-profiled) scenario.
func getBaseScenarioDescriptor(base RuleScenario) (ScenarioDescriptor, bool) {
	scenarioRegistryMu.RLock()
	defer scenarioRegistryMu.RUnlock()
	descriptor, ok := scenarioRegistry[base]
	if !ok {
		return ScenarioDescriptor{}, false
	}
	return cloneScenarioDescriptor(descriptor), true
}
