package config

// Experimental feature flag names
const (
	FeatureSmartCompact = "smart_compact"
	FeatureVisionProxy  = "vision_proxy"
)

// VisionProxyServiceKey is the ScenarioConfig.Extensions key under which the
// scenario-level vision proxy target service ({provider, model}) is stored.
const VisionProxyServiceKey = "vision_proxy_service"
