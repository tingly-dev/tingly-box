package constant

// Gin context key constants for per-request tracking metadata.
// Defined here so that server/, routing/, and middleware/ sub-packages
// can all reference the same values without import cycles.
const (
	CtxKeyRule           = "tracking_rule"             // *typ.Rule
	CtxKeyProvider       = "tracking_provider"         // *typ.Provider
	CtxKeyModel          = "tracking_model"            // string (actual model used)
	CtxKeyRequestModel   = "tracking_request_model"    // string (model requested by user)
	CtxKeyScenario       = "tracking_scenario"         // string (extracted from request path)
	CtxKeyStreamed       = "tracking_streamed"         // bool
	CtxKeyStartTime      = "tracking_start_time"       // time.Time
	CtxKeyFirstTokenTime = "tracking_first_token_time" // time.Time (for TTFT calculation)
	CtxKeyCacheHit       = "tracking_cache_hit"        // bool (cache hit status)
	CtxKeySessionID      = "tracking_session_id"       // string (resolved session ID for affinity)
	CtxKeyAffinityKey    = "tracking_affinity_key"     // string (scoped affinity store key: session + matched smart partition)
	CtxKeyLBServiceID    = "tracking_lb_service_id"    // string (selected upstream, e.g. "provider-uuid:model")
	CtxKeyLBTactic       = "tracking_lb_tactic"        // string (tactic name, e.g. "token_based")
)
