package config

// ScenarioFlags bool field keys. These are the canonical string keys used
// when reading or writing typed ScenarioFlags fields through the
// GetScenarioFlag / SetScenarioFlag string-keyed API (e.g. the HTTP endpoint).
const (
	FlagUnified            = "unified"
	FlagSeparate           = "separate"
	FlagSmart              = "smart"
	FlagSmartCompact       = "smart_compact"
	FlagDisableStreamUsage = "disable_stream_usage"
	FlagCleanHeader        = "clean_header"
)

// ScenarioFlags string field keys. Same contract as above but for
// GetScenarioStringFlag / SetScenarioStringFlag.
const (
	FlagThinkingEffort = "thinking_effort"
	FlagRecordingV2    = "recording_v2"
)
