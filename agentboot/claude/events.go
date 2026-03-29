package claude

// Claude-specific event types (matching SDK stream types)
const (
	EventTypeUser        = "user"
	EventTypeAssistant   = "assistant"
	EventTypeSystem      = "system"
	EventTypeResult      = "result"
	EventTypeControl     = "control_"
	EventTypeStreamEvent = "stream_event"
)

// Control message types
const (
	ControlMsgTypeResponse           = "control_response"
	ControlMsgTypeCancelNotification = "cancel_notification"
	ControlMsgTypeCancelRequest      = "control_cancel_request"
)

// Control subtypes
const (
	ControlSubtypeRequest  = "request"
	ControlSubtypeResponse = "response"
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

// System message subtypes
const (
	SystemSubtypeInit          = "init"
	SystemSubtypeShutdown      = "shutdown"
	SystemSubtypeError         = "error"
	SystemSubtypeTaskStarted   = "task_started"
	SystemSubtypeTaskCompleted = "task_completed"
)

// ToolCallInfo represents a tool call
type ToolCallInfo struct {
	CallID    string      `json:"call_id"`
	ToolName  string      `json:"tool_name"`
	Input     interface{} `json:"input"`
	Result    interface{} `json:"result"`
	Completed bool        `json:"completed"`
}
