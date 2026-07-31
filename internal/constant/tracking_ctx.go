package constant

// Gin context key constants for per-request metadata. Defined here so that
// server/, routing/, middleware/, and protocol/ sub-packages can share the
// writer/reader contract without import cycles or duplicated string literals.
const (
	// Authentication metadata.
	CtxKeyUserID                    = "user_id"                     // string
	CtxKeyEnterpriseUserID          = "enterprise_user_id"          // string
	CtxKeyEnterpriseDepartmentID    = "enterprise_department_id"    // string
	CtxKeyEnterpriseKeyPrefix       = "enterprise_key_prefix"       // string
	CtxKeyEnterpriseUserTier        = "enterprise_user_tier"        // string
	CtxKeyEnterpriseContextJTI      = "enterprise_context_jti"      // string
	CtxKeyEnterpriseContextVerified = "enterprise_context_verified" // bool
	CtxKeyRemoteClientID            = "client_id"                   // string
	CtxKeyRemoteClaims              = "claims"                      // *auth.Claims
	CtxKeyRequestID                 = "request_id"                  // string

	// Request tracking metadata.
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
	CtxKeyLBTactic       = "tracking_lb_tactic"        // string (tactic name, e.g. "random")

	// Protocol recording metadata.
	CtxKeyProtocolRecorder    = "protocol_recorder"     // *recording.ProtocolRecorder
	CtxKeyStreamEventRecorder = "stream_event_recorder" // protocol/stream.StreamEventRecorder

	// Guardrail runtime metadata.
	CtxKeyCredentialMaskState = "guardrails_credential_mask_state" // *guardrails/core.CredentialMaskState
)
