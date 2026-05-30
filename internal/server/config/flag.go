package config

// Experimental feature flag names
const (
	FeatureSmartCompact = "smart_compact"
)

// VisionProxyServiceKey is the ScenarioConfig.Extensions key under which the
// scenario-level vision proxy target service ({provider, model}) is stored.
// Vision proxy is "enabled" for a scenario iff this service is present — there
// is no separate boolean flag (a configured model is the on state).
const VisionProxyServiceKey = "vision_proxy_service"
