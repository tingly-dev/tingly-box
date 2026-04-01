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
	SDKRateLimitMessage          = "rate_limit"
	SDKPromptSuggestionMessage   = "prompt_suggestion"
)

const SDKControlPrefix = "control_"

// System message subtypes
const (
	SystemSubtypeInit          = "init"
	SystemSubtypeTaskCompleted = "task_completed"
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
