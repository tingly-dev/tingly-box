package constant

// Gin context key constants for per-request metadata. Defined here so that
// server/, routing/, middleware/, and protocol/ sub-packages can share the
// writer/reader contract without import cycles or duplicated string literals.
const (
	// Authentication metadata.
	CtxKeyAuthKind                  = "auth_kind"                   // string (see AuthKind* constants)
	CtxKeyUserID                    = "user_id"                     // string
	CtxKeyTeamID                    = "team_id"                     // string (authorized team identity)
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
	CtxKeyTraceID        = "tracking_trace_id"         // string (OTel trace id, set only when the request span is sampled)
	// CtxKeyOperation is the gen_ai.operation.name for the endpoint, declared
	// once where the route is registered so the metrics and trace pipelines
	// report the same operation instead of each deriving its own. Unset means
	// the default, "chat".
	CtxKeyOperation = "tracking_operation" // string

	// Protocol recording metadata.
	CtxKeyProtocolRecorder    = "protocol_recorder"     // *recording.ProtocolRecorder
	CtxKeyStreamEventRecorder = "stream_event_recorder" // protocol/stream.StreamEventRecorder

	// Guardrail runtime metadata.
	CtxKeyCredentialMaskState = "guardrails_credential_mask_state" // *guardrails/core.CredentialMaskState
)

// Authentication principal kinds recorded in CtxKeyAuthKind. Keep these
// separate from user_id: user_id identifies who generated usage, while the
// principal kind determines which model surfaces that identity may access.
const (
	AuthKindGlobalModelToken = "global_model_token"
	AuthKindSharingKey       = "sharing_key"
)
