package claude

// MessageType constants for Claude Code stream JSON
// all of these types defined in python sdk: https://platform.claude.com/docs/en/agent-sdk/python#message-types
// or TypeScript sdk: https://platform.claude.com/docs/en/agent-sdk/typescript#message-types
// type SDKMessage =
//
//	| SDKAssistantMessage
//	| SDKUserMessage
//	| SDKUserMessageReplay
//	| SDKResultMessage
//	| SDKSystemMessage
//	| SDKPartialAssistantMessage
//	| SDKCompactBoundaryMessage
//	| SDKStatusMessage
//	| SDKLocalCommandOutputMessage
//	| SDKHookStartedMessage
//	| SDKHookProgressMessage
//	| SDKHookResponseMessage
//	| SDKToolProgressMessage
//	| SDKAuthStatusMessage
//	| SDKTaskNotificationMessage
//	| SDKTaskStartedMessage
//	| SDKTaskProgressMessage
//	| SDKFilesPersistedEvent
//	| SDKToolUseSummaryMessage
//	| SDKRateLimitEvent
//	| SDKPromptSuggestionMessage;
const (
	SDKTextMessage               = "text"
	SDKSystemMessage             = "system"
	SDKAssistantMessage          = "assistant"
	SDKUserMessage               = "user"
	SDKToolUseMessage            = "tool_use"
	SDKToolResultMessage         = "tool_result"
	SDKResultMessage             = "result"
	SDKStreamEventMessage        = "stream_event"
	SDKControlRequestMessage     = "control_request"
	SDKUserMessageReplayMessage  = "user_message_replay"
	SDKPartialAssistantMessage   = "partial_assistant"
	SDKCompactBoundaryMessage    = "compact_boundary"
	SDKStatusMessage             = "status"
	SDKLocalCommandOutputMessage = "local_command_output"
	SDKHookStartedMessage        = "hook_started"
	SDKHookProgressMessage       = "hook_progress"
	SDKHookResponseMessage       = "hook_response"
	SDKToolProgressMessage       = "tool_progress"
	SDKAuthStatusMessage         = "auth_status"
	SDKTaskNotificationMessage   = "task_notification"
	SDKTaskStartedMessage        = "task_started"
	SDKTaskProgressMessage       = "task_progress"
	SDKFilesPersistedMessage     = "files_persisted"
	SDKToolUseSummaryMessage     = "tool_use_summary"
	// SDKRateLimitEvent is the top-level type emitted by CC v2.1+ when the
	// upstream rate-limit state changes. The old name was "rate_limit" but the
	// actual wire value observed in v2.1.157 is "rate_limit_event".
	SDKRateLimitEvent          = "rate_limit_event"
	SDKPromptSuggestionMessage = "prompt_suggestion"
	SDKPostTurnSummaryMessage  = "post_turn_summary"
)

const SDKControlPrefix = "control_"

// System message subtypes
const (
	SystemSubtypeInit          = "init"
	SystemSubtypeTaskCompleted = "task_completed"
	// SystemSubtypeAPIRetry is emitted by the Claude Code CLI when an upstream
	// API call fails with a retryable error (overload, rate limit, transient
	// network) and the CLI automatically retries. Surfacing it tells the user
	// why the agent appears to stall instead of leaving a silent gap.
	SystemSubtypeAPIRetry = "api_retry"
	// SystemSubtypeRateLimit is emitted when the CLI is rate limited upstream.
	SystemSubtypeRateLimit = "rate_limit"
)

// assistant message error
// ref: https://platform.claude.com/docs/en/agent-sdk/python#assistant-message-error
const (
	AssistantErrorAuthFailed     = "authentication_failed"
	AssistantErrorBilling        = "billing_error"
	AssistantErrorRateLimit      = "rate_limit"
	AssistantErrorInvalidRequest = "invalid_request"
	AssistantErrorServer         = "server_error"
	AssistantErrorMaxTokens      = "max_output_tokens"
	AssistantErrorUnknown        = "unknown"
)

// Control message types
const (
	ControlMsgTypeResponse           = "control_response"
	ControlMsgTypeCancelNotification = "cancel_notification"
	ControlMsgTypeCancelRequest      = "control_cancel_request"
)

// Control request subtypes
const (
	ControlRequestSubtypeCanUseTool = "can_use_tool"
	ControlRequestSubtypeInterrupt  = "interrupt"
)

// Control request types
const (
	ControlRequestTypePermission = "permission"
	ControlRequestTypeCancel     = "cancel"
)

// Result subtypes
const (
	ResultSubtypeSuccess = "success"
	ResultSubtypeError   = "error"
)

// Content block types
const (
	ContentBlockTypeText       = "text"
	ContentBlockTypeToolUse    = "tool_use"
	ContentBlockTypeThinking   = "thinking"
	ContentBlockTypeToolResult = "tool_result"
)
