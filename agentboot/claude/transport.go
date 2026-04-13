package claude

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// Transport implements agentboot.AgentTransport for Claude Code CLI.
// It decodes Claude's stream-json output and handles the bidirectional
// control protocol (permission requests, AskUserQuestion).
type Transport struct {
	accumulator *MessageAccumulator
	// execCtx carries per-execution routing context injected by the Runner
	// before the first event is processed.
	execCtx executionContext
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
func (t *Transport) SetExecutionContext(sessionID, chatID, platform, botUUID string) {
	t.execCtx = executionContext{
		sessionID: sessionID,
		chatID:    chatID,
		platform:  platform,
		botUUID:   botUUID,
	}
}

// Decode parses one JSON-decoded output value into an AgentEvent.
func (t *Transport) Decode(raw any) (agentboot.AgentEvent, error) {
	data, ok := raw.(map[string]any)
	if !ok {
		return agentboot.AgentEvent{}, nil
	}

	event := common.NewEventFromMap(data)
	logrus.Debugf("[Event] %s", event)

	isControl := strings.HasPrefix(event.Type, SDKControlPrefix)
	isTerminal := event.Type == SDKResultMessage

	return agentboot.AgentEvent{
		Raw:        event,
		IsControl:  isControl,
		IsTerminal: isTerminal,
	}, nil
}

// WrapMessage converts a decoded AgentEvent into the typed Message objects
// produced by the MessageAccumulator, to be forwarded to MessageHandler.OnMessage.
// Returns nil when there is nothing to forward (e.g. pure control events).
func (t *Transport) WrapMessage(ae agentboot.AgentEvent) interface{} {
	// Accumulate produces typed Messages; we return the first one (if any).
	// Control events and result events are not forwarded via OnMessage.
	msgs, _, _ := t.accumulator.AddEvent(ae.Raw)
	if len(msgs) == 0 || ae.IsControl || ae.IsTerminal {
		return nil
	}
	// Return slice so the runner can iterate; it accepts interface{}.
	return msgs
}

// AccumulateEvent runs the accumulator and returns all produced messages,
// hasResult, and resultSuccess flags. Used by the runner for fine-grained control.
func (t *Transport) AccumulateEvent(ae agentboot.AgentEvent) ([]Message, bool, bool) {
	return t.accumulator.AddEvent(ae.Raw)
}

// AccumulateAndForward implements agentboot.ClaudeTransporter.
// It accumulates the event and returns []interface{} messages for OnMessage forwarding.
// For control events and terminal events it returns nil messages.
func (t *Transport) AccumulateAndForward(ae agentboot.AgentEvent) ([]interface{}, bool, bool) {
	msgs, hasResult, success := t.accumulator.AddEvent(ae.Raw)

	// Control events are handled separately by HandleControl.
	// Terminal events are handled by the runner directly.
	if ae.IsControl || ae.IsTerminal {
		return nil, hasResult, success
	}

	if len(msgs) == 0 {
		return nil, false, false
	}

	out := make([]interface{}, len(msgs))
	for i, m := range msgs {
		out[i] = m
	}
	return out, hasResult, success
}

// HandleControl processes a control event and writes the response to the agent's stdin.
func (t *Transport) HandleControl(
	ctx context.Context,
	ae agentboot.AgentEvent,
	handler agentboot.MessageHandler,
	write func(any) error,
) error {
	event := ae.Raw

	controlData, ok := event.Data["request"].(map[string]interface{})
	if !ok {
		return nil
	}

	subtype, _ := controlData["subtype"].(string)
	requestID := getString(event.Data, "request_id")

	switch subtype {
	case ControlRequestSubtypeCanUseTool:
		toolName, _ := controlData["tool_name"].(string)

		if toolName == "AskUserQuestion" {
			req := t.parseAskRequestFromControl(controlData)
			req.ID = requestID

			logrus.WithFields(logrus.Fields{
				"platform":   req.Platform,
				"chat_id":    req.ChatID,
				"session_id": req.SessionID,
				"request_id": req.ID,
				"tool_name":  req.ToolName,
			}).Info("Processing AskUserQuestion control request")

			result, err := handler.OnAsk(ctx, req)
			if err != nil {
				logrus.Errorf("Ask handler error: %v", err)
				result = agentboot.AskResult{ID: requestID, Approved: false}
			}
			return write(t.buildAskResponse(requestID, result))
		}

		// Regular tool permission request.
		req := t.parsePermissionRequest(controlData)
		req.RequestID = requestID

		logrus.WithFields(logrus.Fields{
			"tool_name":  req.ToolName,
			"session_id": req.SessionID,
			"request_id": req.RequestID,
		}).Info("Processing can_use_tool control request")

		result, err := handler.OnApproval(ctx, req)
		if err != nil {
			logrus.Errorf("Permission handler error: %v", err)
			result = agentboot.PermissionResult{Approved: false}
		}
		return write(t.buildPermissionResponse(requestID, result, req.Input))

	default:
		logrus.Warnf("Unsupported control request subtype: %s", subtype)
	}
	return nil
}

// HandleAssistantAsk handles the legacy path where AskUserQuestion arrives as an
// assistant message content block (when --permission-prompt-tool is not "stdio").
// Returns (response, true) if a response was sent, otherwise (nil, false).
func (t *Transport) HandleAssistantAsk(
	ctx context.Context,
	ae agentboot.AgentEvent,
	msgs []Message,
	handler agentboot.MessageHandler,
	write func(any) error,
) (bool, error) {
	if ae.Raw.Type != SDKAssistantMessage {
		return false, nil
	}

	requestID := getString(ae.Raw.Data, "request_id")
	for _, msg := range msgs {
		assistant, ok := msg.(*AssistantMessage)
		if !ok {
			continue
		}
		for _, c := range assistant.Message.Content {
			if c.Name != "AskUserQuestion" {
				continue
			}
			req := t.parseAskRequestFromContentBlock(c)
			req.ID = requestID

			logrus.WithFields(logrus.Fields{
				"platform":   req.Platform,
				"chat_id":    req.ChatID,
				"session_id": req.SessionID,
				"request_id": req.ID,
			}).Info("Processing ask_user control request")

			result, err := handler.OnAsk(ctx, req)
			if err != nil {
				logrus.Errorf("Ask handler error: %v", err)
				result = agentboot.AskResult{ID: requestID, Approved: false}
			}
			if wErr := write(t.buildAskResponse(requestID, result)); wErr != nil {
				return true, wErr
			}
			return true, nil
		}
	}
	return false, nil
}

// --- request parsing helpers ------------------------------------------------

func (t *Transport) parsePermissionRequest(data map[string]interface{}) agentboot.PermissionRequest {
	input := getMap(data, "input")
	if input == nil {
		input = make(map[string]interface{})
	}

	if t.execCtx.chatID != "" {
		input["_chat_id"] = t.execCtx.chatID
	}
	if t.execCtx.platform != "" {
		input["_platform"] = t.execCtx.platform
	}

	return agentboot.PermissionRequest{
		RequestID: getString(data, "request_id"),
		AgentType: agentboot.AgentTypeClaude,
		ToolName:  getString(data, "tool_name"),
		Input:     input,
		SessionID: t.execCtx.sessionID,
		BotUUID:   t.execCtx.botUUID,
	}
}

func (t *Transport) parseAskRequestFromControl(controlData map[string]interface{}) agentboot.AskRequest {
	toolName, _ := controlData["tool_name"].(string)
	toolUseID, _ := controlData["tool_use_id"].(string)
	input, _ := controlData["input"].(map[string]interface{})

	return agentboot.AskRequest{
		Type:      ContentBlockTypeToolUse,
		AgentType: agentboot.AgentTypeClaude,
		Platform:  t.execCtx.platform,
		ChatID:    t.execCtx.chatID,
		BotUUID:   t.execCtx.botUUID,
		SessionID: t.execCtx.sessionID,
		ToolName:  toolName,
		Input:     input,
		CallID:    toolUseID,
	}
}

func (t *Transport) parseAskRequestFromContentBlock(c anthropic.ContentBlockUnion) agentboot.AskRequest {
	input := map[string]any{}
	_ = json.Unmarshal(c.Input, &input)
	return agentboot.AskRequest{
		Type:      c.Type,
		AgentType: agentboot.AgentTypeClaude,
		Platform:  t.execCtx.platform,
		ChatID:    t.execCtx.chatID,
		SessionID: t.execCtx.sessionID,
		ToolName:  c.Name,
		Input:     input,
		Message:   c.Text,
		CallID:    c.ID,
	}
}

// --- response builders -------------------------------------------------------

func (t *Transport) buildAskResponse(requestID string, result agentboot.AskResult) map[string]any {
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

func (t *Transport) buildPermissionResponse(requestID string, result agentboot.PermissionResult, originalInput map[string]interface{}) map[string]any {
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
