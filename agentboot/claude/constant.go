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
//	| SDKFilesPersistedEvent
//	| SDKToolUseSummaryMessage
//	| SDKRateLimitEvent
//	| SDKPromptSuggestionMessage;
//
// Task lifecycle wire values are system-message subtypes in current Claude
// Code. Deprecated SDKTask* aliases are retained below for source compatibility.
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

// Execution-context metadata keys understood by the Claude transport.
const (
	ContextKeyChatID   = "chat_id"
	ContextKeyPlatform = "platform"
	ContextKeyBotUUID  = "bot_uuid"
)

// System message subtypes
const (
	SystemSubtypeInit             = "init"
	SystemSubtypeTaskStarted      = "task_started"
	SystemSubtypeTaskProgress     = "task_progress"
	SystemSubtypeTaskNotification = "task_notification"
	SystemSubtypeTaskUpdated      = "task_updated"
	// SystemSubtypeTaskCompleted is emitted by older Claude Code versions.
	SystemSubtypeTaskCompleted = "task_completed"
	// SystemSubtypeAPIRetry is emitted by the Claude Code CLI when an upstream
	// API call fails with a retryable error (overload, rate limit, transient
	// network) and the CLI automatically retries. Surfacing it tells the user
	// why the agent appears to stall instead of leaving a silent gap.
	SystemSubtypeAPIRetry = "api_retry"
	// SystemSubtypeRateLimit is emitted when the CLI is rate limited upstream.
	SystemSubtypeRateLimit = "rate_limit"
)

// Deprecated task names retained as aliases for callers that used them before
// these wire values were correctly classified as system-message subtypes.
const (
	SDKTaskStartedMessage      = SystemSubtypeTaskStarted
	SDKTaskProgressMessage     = SystemSubtypeTaskProgress
	SDKTaskNotificationMessage = SystemSubtypeTaskNotification
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
	ControlMsgTypeResponse = "control_response"
)

// Control request subtypes
const (
	ControlRequestSubtypeCanUseTool = "can_use_tool"
	ControlRequestSubtypeInterrupt  = "interrupt"
)

// Control-response subtypes
const (
	ControlResponseSubtypeSuccess = "success"
	ControlResponseSubtypeError   = "error"
)

// Terminal result subtypes
const (
	ResultSubtypeSuccess                         = "success"
	ResultSubtypeErrorMaxTurns                   = "error_max_turns"
	ResultSubtypeErrorDuringExecution            = "error_during_execution"
	ResultSubtypeErrorMaxBudgetUSD               = "error_max_budget_usd"
	ResultSubtypeErrorMaxStructuredOutputRetries = "error_max_structured_output_retries"

	// ResultSubtypeError was historically used for control responses. Terminal
	// result errors use the error_* constants above.
	// Deprecated: use ControlResponseSubtypeError.
	ResultSubtypeError = ControlResponseSubtypeError
)

// Content block types
const (
	ContentBlockTypeText                          = "text"
	ContentBlockTypeToolUse                       = "tool_use"
	ContentBlockTypeThinking                      = "thinking"
	ContentBlockTypeRedactedThinking              = "redacted_thinking"
	ContentBlockTypeToolResult                    = "tool_result"
	ContentBlockTypeServerToolUse                 = "server_tool_use"
	ContentBlockTypeWebSearchToolResult           = "web_search_tool_result"
	ContentBlockTypeWebFetchToolResult            = "web_fetch_tool_result"
	ContentBlockTypeCodeExecutionToolResult       = "code_execution_tool_result"
	ContentBlockTypeBashCodeExecutionToolResult   = "bash_code_execution_tool_result"
	ContentBlockTypeTextEditorExecutionToolResult = "text_editor_code_execution_tool_result"
	ContentBlockTypeToolSearchToolResult          = "tool_search_tool_result"
	ContentBlockTypeContainerUpload               = "container_upload"
)
