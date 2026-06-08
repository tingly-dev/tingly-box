package config

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
	FlagCompactKeyword  = "compact_keyword"
)

// ScenarioFlags int field keys. Same contract as above but for
// GetScenarioIntFlag / SetScenarioIntFlag. None are registered right now:
// session_affinity was downgraded to a rule-only flag (see
// internal/typ/flag_registry.go and the built-in rule seeds in init.go /
// migrate20260610). The generic int-flag get/set infra (and its HTTP endpoint)
// is retained for future scenario int flags — add the key const here plus a
// switch case in config.go's Get/SetScenarioIntFlag.
