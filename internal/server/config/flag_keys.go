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
)

// ScenarioFlags int field keys. Same contract as above but for
// GetScenarioIntFlag / SetScenarioIntFlag.
const (
	FlagSessionAffinity = "session_affinity"
)
