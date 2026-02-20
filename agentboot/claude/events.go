package claude

// Claude-specific event types
const (
	EventTypeTextDelta      = "text_delta"
	EventTypeToolCallStart  = "tool_call_start"
	EventTypeToolCallEnd    = "tool_call_end"
	EventTypePermissionReq  = "permission_request"
	EventTypeStatus         = "status"
	EventTypeThinking       = "thinking"
	EventTypeError          = "error"
	EventTypeControlRequest = "control_request"
	EventTypeControlResp    = "control_response"
)

// ToolCallInfo represents a tool call
type ToolCallInfo struct {
	CallID    string      `json:"call_id"`
	ToolName  string      `json:"tool_name"`
	Input     interface{} `json:"input"`
	Result    interface{} `json:"result"`
	Completed bool        `json:"completed"`
}
