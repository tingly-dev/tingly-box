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

// Control subtypes
const (
	ControlSubtypeRequest  = "request"
	ControlSubtypeResponse = "response"
)

// Result subtypes
const (
	ResultSubtypeSuccess = "success"
	ResultSubtypeError   = "error"
)

// ToolCallInfo represents a tool call
type ToolCallInfo struct {
	CallID    string      `json:"call_id"`
	ToolName  string      `json:"tool_name"`
	Input     interface{} `json:"input"`
	Result    interface{} `json:"result"`
	Completed bool        `json:"completed"`
}
