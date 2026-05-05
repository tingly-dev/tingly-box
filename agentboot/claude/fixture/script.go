package fixture

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// Step is one entry in a [Script]. Each Step produces one JSON-encoded line
// of Claude wire-format output when the fixture runs.
//
// Steps that emit a control_request event (Permission, Ask) report
// needsResponse = true; the fixture waits for one stdin message after
// emitting them, simulating Claude's behavior of blocking until the
// permission tool replies.
type Step interface {
	encode() ([]byte, error)
	needsResponse() (id string, ok bool)
}

// System emits an init system event with the given session_id and cwd.
// Most scripts start with a System step, mirroring real Claude Code output.
func System(sessionID, cwd string) Step { return systemStep{SessionID: sessionID, Cwd: cwd} }

type systemStep struct {
	SessionID string
	Cwd       string
}

func (s systemStep) needsResponse() (string, bool) { return "", false }
func (s systemStep) encode() ([]byte, error) {
	return json.Marshal(map[string]any{
		"type":       claude.SDKSystemMessage,
		"subtype":    "init",
		"session_id": s.SessionID,
		"cwd":        s.Cwd,
	})
}

// AssistantText emits an assistant message with a single text content block.
func AssistantText(text string) Step { return assistantTextStep{Text: text} }

type assistantTextStep struct {
	Text string
}

func (s assistantTextStep) needsResponse() (string, bool) { return "", false }
func (s assistantTextStep) encode() ([]byte, error) {
	return json.Marshal(map[string]any{
		"type": claude.SDKAssistantMessage,
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": s.Text},
			},
		},
	})
}

// PermissionRequest emits a control_request / can_use_tool event for the
// given tool. The fixture blocks after emission until the test responds
// via [agentboot.ExecutionHandle.Respond] (the response arrives on stdin).
//
// If reqID is empty, a random UUID is generated.
func PermissionRequest(reqID, toolName string, input map[string]any) Step {
	if reqID == "" {
		reqID = uuid.New().String()
	}
	return permissionStep{ID: reqID, Tool: toolName, Input: input}
}

type permissionStep struct {
	ID    string
	Tool  string
	Input map[string]any
}

func (s permissionStep) needsResponse() (string, bool) { return s.ID, true }
func (s permissionStep) encode() ([]byte, error) {
	if s.Input == nil {
		s.Input = map[string]any{}
	}
	return json.Marshal(map[string]any{
		"type":       claude.SDKControlRequestMessage,
		"request_id": s.ID,
		"request": map[string]any{
			"subtype":   claude.ControlRequestSubtypeCanUseTool,
			"tool_name": s.Tool,
			"input":     s.Input,
		},
	})
}

// AskQuestionStep emits a control_request / can_use_tool event with
// tool_name = "AskUserQuestion". The Input map should contain "questions"
// describing the question/options shape that IMPrompter consumes.
//
// If reqID is empty, a random UUID is generated.
func AskQuestionStep(reqID, toolUseID string, input map[string]any) Step {
	if reqID == "" {
		reqID = uuid.New().String()
	}
	if toolUseID == "" {
		toolUseID = "tool_" + uuid.New().String()[:8]
	}
	return askStep{ID: reqID, ToolUseID: toolUseID, Input: input}
}

type askStep struct {
	ID        string
	ToolUseID string
	Input     map[string]any
}

func (s askStep) needsResponse() (string, bool) { return s.ID, true }
func (s askStep) encode() ([]byte, error) {
	if s.Input == nil {
		s.Input = map[string]any{}
	}
	return json.Marshal(map[string]any{
		"type":       claude.SDKControlRequestMessage,
		"request_id": s.ID,
		"request": map[string]any{
			"subtype":     claude.ControlRequestSubtypeCanUseTool,
			"tool_name":   "AskUserQuestion",
			"tool_use_id": s.ToolUseID,
			"input":       s.Input,
		},
	})
}

// Result emits a terminal result event. Pass success=false for an error
// terminal (the runner records this as TerminalError and Wait returns an error).
func Result(success bool) Step { return resultStep{Success: success} }

type resultStep struct {
	Success bool
}

func (s resultStep) needsResponse() (string, bool) { return "", false }
func (s resultStep) encode() ([]byte, error) {
	subtype := "success"
	if !s.Success {
		subtype = "error"
	}
	return json.Marshal(map[string]any{
		"type":     claude.SDKResultMessage,
		"subtype":  subtype,
		"is_error": !s.Success,
	})
}

// Raw emits an arbitrary JSON-shaped event for tests that need exact wire
// control. Use the typed builders above when possible.
func Raw(payload map[string]any) Step { return rawStep{Payload: payload} }

type rawStep struct {
	Payload map[string]any
}

func (s rawStep) needsResponse() (string, bool) { return "", false }
func (s rawStep) encode() ([]byte, error) {
	if s.Payload == nil {
		return nil, fmt.Errorf("fixture.Raw: nil payload")
	}
	return json.Marshal(s.Payload)
}
