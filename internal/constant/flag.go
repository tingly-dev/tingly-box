package constant

// ScenarioFlags bool field keys. These are the canonical string keys used
// when reading or writing typed ScenarioFlags fields through the
// GetScenarioFlag / SetScenarioFlag string-keyed API (e.g. the HTTP endpoint).
const (
	FlagUnified      = "unified"
	FlagSeparate     = "separate"
	FlagSmart        = "smart"
	FlagSmartCompact = "smart_compact"
	FlagSkipUsage    = "skip_usage"
)

// ScenarioFlags string field keys. Same contract as above but for
// GetScenarioStringFlag / SetScenarioStringFlag.
const (
	FlagThinkingEffort  = "thinking_effort"
	FlagRecordingV2     = "recording_v2"
	FlagCustomUserAgent = "custom_user_agent"
)

// ScenarioFlags int field keys. Same contract as above but for
// GetScenarioIntFlag / SetScenarioIntFlag. None are registered right now:
// session_affinity was downgraded to a rule-only flag (see
// internal/typ/flag_registry.go and the built-in rule seeds in init.go /
// migrate20260610). The generic int-flag get/set infra (and its HTTP endpoint)
// is retained for future scenario int flags — add the key const here plus a
// switch case in config.go's Get/SetScenarioIntFlag.

// ScenarioConfig.Extensions keys. These are the canonical string keys used
// when reading or writing feature toggles stored in the Extensions map
// (as opposed to typed ScenarioFlags fields).
const (
	ExtensionVisionProxyService = "vision_proxy_service"
	ExtensionGuardrails         = "guardrails"
	ExtensionMCP                = "mcp"
	ExtensionSkillUser          = "skill_user"
	ExtensionSkillIDE           = "skill_ide"
	ExtensionBench              = "bench"
)

// KnownExtensionBoolFlags is the single source of truth for which
// Extensions-map keys are a settable/readable bool toggle through
// Config.GetScenarioFlag / Config.SetScenarioFlag's generic fallback (the
// default case for anything that isn't one of the typed ScenarioFlags
// fields above). Adding a new global on/off feature toggle (e.g. a new row
// in frontend/src/components/GlobalExperimentalFeatures.tsx) means adding
// one key here — the getter and setter both pick it up automatically, with
// no switch/case to remember to touch on the backend.
//
// This existed as an asymmetry before: GetScenarioFlag already fell back to
// reading any key out of Extensions (defaulting to false when absent), but
// SetScenarioFlag required an explicit `case constant.ExtensionXxx:` for
// every key or it rejected the write with "unknown flag name" — so a flag
// could be read (silently false) but never turned on until someone
// remembered to add its case. That's what broke the bench flag on launch
// and had bitten skill_user/skill_ide/guardrails/mcp before it (see
// TestProfileScenarioFlagWriteInheritsBaseConfig's history). This map
// closes that gap by making both directions read the same registry.
//
// vision_proxy_service is deliberately excluded: it stores a structured
// object (provider/model selection), not a bool, and is read directly off
// ScenarioConfig.Extensions rather than through Get/SetScenarioFlag.
var KnownExtensionBoolFlags = map[string]bool{
	ExtensionGuardrails: true,
	ExtensionMCP:        true,
	ExtensionSkillUser:  true,
	ExtensionSkillIDE:   true,
	ExtensionBench:      true,
}
