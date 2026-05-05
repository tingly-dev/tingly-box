package claude

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// Transport implements [agentboot.AgentTransport] for the Claude Code CLI.
//
// It is pure: it parses common.Event values into classifications and
// accumulated messages, and encodes control responses into the wire shape
// Claude expects on stdin. It performs no IO and owns no goroutines; the
// runner drives the lifecycle.
type Transport struct {
	accumulator *MessageAccumulator
	execCtx     executionContext
}

// executionContext holds caller-supplied routing metadata so that permission
// and ask requests can be annotated with chat_id / platform / bot_uuid.
type executionContext struct {
	sessionID string
	chatID    string
	platform  string
	botUUID   string
}

// NewTransport creates a new Claude Transport.
func NewTransport() *Transport {
	return &Transport{
		accumulator: NewMessageAccumulator(),
	}
}

// SetExecutionContext injects routing metadata before an execution begins.
// Implements [agentboot.AgentTransport].
func (t *Transport) SetExecutionContext(sessionID, chatID, platform, botUUID string) {
	t.execCtx = executionContext{
		sessionID: sessionID,
		chatID:    chatID,
		platform:  platform,
		botUUID:   botUUID,
	}
}

// Classify reports the kind of event and, for control events, the parsed
// StreamEvent ready to emit on the handle. Implements [agentboot.AgentTransport].
func (t *Transport) Classify(ev common.Event) (agentboot.EventKind, agentboot.StreamEvent) {
	if strings.HasPrefix(ev.Type, SDKControlPrefix) {
		parsed, err := t.parseControlRequest(ev)
		if err != nil {
			logrus.WithError(err).Warn("claude transport: parse control request")
			return agentboot.EventKindIgnore, nil
		}
		if parsed == nil {
			return agentboot.EventKindIgnore, nil
		}
		return agentboot.EventKindControl, parsed
	}

	if ev.Type == SDKResultMessage {
		if isError, _ := ev.Data["is_error"].(bool); isError {
			return agentboot.EventKindTerminalError, nil
		}
		return agentboot.EventKindTerminalSuccess, nil
	}

	return agentboot.EventKindMessage, nil
}

// AccumulateMessage feeds the event to the per-agent accumulator and returns
// 0+ rich claude.Message values for the runner to wrap as MessageEvents.
// Implements [agentboot.AgentTransport].
func (t *Transport) AccumulateMessage(ev common.Event) []any {
	msgs, _, _ := t.accumulator.AddEvent(ev)
	if len(msgs) == 0 {
		return nil
	}
	out := make([]any, len(msgs))
	for i, m := range msgs {
		out[i] = m
	}
	return out
}

// EncodeControlResponse converts a [agentboot.ControlResponse] into the wire
// payload Claude expects on stdin. Implements [agentboot.AgentTransport].
func (t *Transport) EncodeControlResponse(reqID string, resp agentboot.ControlResponse, originalInput map[string]any) any {
	switch r := resp.(type) {
	case agentboot.ApprovalResponse:
		return t.buildPermissionResponse(reqID, r, originalInput)
	case agentboot.AskResponse:
		return t.buildAskResponse(reqID, r)
	default:
		logrus.Warnf("claude transport: unknown ControlResponse type %T", resp)
		return nil
	}
}

// --- internal: control-event parsing ----------------------------------------

// parseControlRequest dispatches on the request subtype to produce either an
// ApprovalRequestEvent or AskRequestEvent. Returns (nil, nil) for subtypes
// the transport does not understand.
func (t *Transport) parseControlRequest(ev common.Event) (agentboot.StreamEvent, error) {
	controlData, ok := ev.Data["request"].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	subtype, _ := controlData["subtype"].(string)
	requestID := getString(ev.Data, "request_id")

	switch subtype {
	case ControlRequestSubtypeCanUseTool:
		toolName, _ := controlData["tool_name"].(string)
		input, _ := controlData["input"].(map[string]any)

		if toolName == "AskUserQuestion" {
			toolUseID, _ := controlData["tool_use_id"].(string)
			return agentboot.AskRequestEvent{
				ID:        requestID,
				AgentType: agentboot.AgentTypeClaude,
				Type:      ContentBlockTypeToolUse,
				ToolName:  toolName,
				Input:     input,
				CallID:    toolUseID,
				SessionID: t.execCtx.sessionID,
				ChatID:    t.execCtx.chatID,
				Platform:  t.execCtx.platform,
				BotUUID:   t.execCtx.botUUID,
			}, nil
		}

		stamped := input
		if stamped == nil {
			stamped = make(map[string]any)
		} else {
			cp := make(map[string]any, len(stamped)+2)
			for k, v := range stamped {
				cp[k] = v
			}
			stamped = cp
		}
		if t.execCtx.chatID != "" {
			stamped["_chat_id"] = t.execCtx.chatID
		}
		if t.execCtx.platform != "" {
			stamped["_platform"] = t.execCtx.platform
		}

		return agentboot.ApprovalRequestEvent{
			ID:        requestID,
			AgentType: agentboot.AgentTypeClaude,
			ToolName:  toolName,
			Input:     stamped,
			SessionID: t.execCtx.sessionID,
			ChatID:    t.execCtx.chatID,
			Platform:  t.execCtx.platform,
			BotUUID:   t.execCtx.botUUID,
		}, nil

	default:
		logrus.Warnf("claude transport: unsupported control subtype %q", subtype)
		return nil, nil
	}
}

// --- internal: control-response builders ------------------------------------

func (t *Transport) buildAskResponse(requestID string, result agentboot.AskResponse) map[string]any {
	innerResponse := map[string]interface{}{"request_id": requestID}

	if result.Approved {
		innerResponse["subtype"] = ResultSubtypeSuccess
		if result.UpdatedInput != nil {
			innerResponse["response"] = map[string]interface{}{
				"behavior":     "allow",
				"updatedInput": result.UpdatedInput,
			}
		} else {
			innerResponse["response"] = map[string]interface{}{"behavior": "allow"}
		}
	} else {
		innerResponse["subtype"] = ResultSubtypeError
		reason := result.Reason
		if reason == "" {
			reason = "User denied this request"
		}
		innerResponse["error"] = reason
	}

	return map[string]any{
		"request_id": requestID,
		"type":       ControlMsgTypeResponse,
		"response":   innerResponse,
	}
}

func (t *Transport) buildPermissionResponse(requestID string, result agentboot.ApprovalResponse, originalInput map[string]interface{}) map[string]any {
	innerResponse := map[string]interface{}{
		"subtype":    ResultSubtypeSuccess,
		"request_id": requestID,
	}

	if result.Approved {
		updatedInput := result.UpdatedInput
		if updatedInput == nil {
			updatedInput = originalInput
		}
		innerResponse["response"] = map[string]interface{}{
			"behavior":     "allow",
			"updatedInput": updatedInput,
		}
	} else {
		message := result.Reason
		if message == "" {
			message = "User denied this request"
		}
		innerResponse["response"] = map[string]interface{}{
			"behavior": "deny",
			"message":  message,
		}
	}

	return map[string]any{
		"request_id": requestID,
		"type":       ControlMsgTypeResponse,
		"response":   innerResponse,
	}
}
