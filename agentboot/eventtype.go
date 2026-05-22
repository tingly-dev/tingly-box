package agentboot

// Event type constants for the string Type field of common.Event values.
// Agents map their internal events to these standard types so consumers can
// switch on a stable vocabulary regardless of the source agent.
const (
	EventTypeInit              = "init"
	EventTypeSystem            = "system"
	EventTypeAssistant         = "assistant"
	EventTypeUser              = "user"
	EventTypeToolUse           = "tool_use"
	EventTypeToolResult        = "tool_result"
	EventTypePermissionRequest = "permission_request"
	EventTypePermissionResult  = "permission_result"
	EventTypeResult            = "result"
	EventTypeError             = "error"
	EventTypeStreamDelta       = "stream_delta"
)
