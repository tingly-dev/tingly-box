package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
)

// streamingMessageHandler implements agentboot.MessageStreamer for real-time message streaming.
// It also implements CompletionCallback for sending action keyboard when done.
type streamingMessageHandler struct {
	bot       imbot.Bot
	chatID    string
	replyTo   string
	mu        sync.Mutex
	formatter *claude.TextFormatter
	verbose   bool          // If false, only show final results (hide intermediate messages)
	meta      *ResponseMeta // Pointer so OnComplete sees updates from SmartGuideCompletionCallback

	// toolBuffer accumulates formatted tool-only renders between text-bearing
	// messages. Messages act as the splitting boundary: when a text-bearing
	// claude.Message arrives (assistant text, result, system notice), the
	// buffered tool renders are flushed as a single aggregated message before
	// the new text is sent. This avoids flooding the chat with one IM message
	// per tool call when the agent runs long tool chains.
	toolBuffer []string
}

// toolBufferFlushThreshold is the upper bound on buffered tool entries; when
// reached without an intervening text-bearing message, the buffer is flushed
// to ensure the user still sees progress on long-running tool chains.
const toolBufferFlushThreshold = 20

// quietToolPreviewCount is how many of the buffered tool entries are inlined
// into the aggregated summary shown in quiet mode. The rest are folded into
// an "(+N more)" suffix.
const quietToolPreviewCount = 3

// Ensure streamingMessageHandler implements required interfaces
var _ agentboot.MessageStreamer = (*streamingMessageHandler)(nil)
var _ agentboot.CompletionCallback = (*streamingMessageHandler)(nil)

// newStreamingMessageHandler creates a new streaming message handler
func newStreamingMessageHandler(bot imbot.Bot, chatID, replyTo string, verbose bool, meta *ResponseMeta) *streamingMessageHandler {
	return &streamingMessageHandler{
		bot:       bot,
		chatID:    chatID,
		replyTo:   replyTo,
		formatter: claude.NewTextFormatter(),
		verbose:   verbose,
		meta:      meta,
	}
}

// OnMessage implements agentboot.MessageHandler
func (h *streamingMessageHandler) OnMessage(msg interface{}) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"msgType": fmt.Sprintf("%T", msg),
		"chatID":  h.chatID,
	}).Debug("Received message from agent")

	// Try to handle as unified AgentMessage first
	if agentMsg, ok := msg.(agentboot.AgentMessage); ok {
		return h.handleAgentMessage(agentMsg)
	}

	// Handle specific types
	switch m := msg.(type) {
	case string:
		h.sendMessage(m)
		return nil
	case *claude.AssistantMessage:
		// Let TextFormatter decide what to display.
		// If formatted output is empty (e.g. tool_use only), handleClaudeMessage skips it.
		return h.handleClaudeMessage(m)

	case claude.Message:
		return h.handleClaudeMessage(m)

	case agentboot.Event:
		// Convert Event to AgentMessage and handle
		agentType := agentboot.AgentTypeMockAgent // default
		if at, ok := m.Data["agent_type"].(string); ok {
			agentType = agentboot.AgentType(at)
		}
		agentMsg := agentboot.MessageFromEvent(m, agentType)
		if agentMsg != nil {
			return h.handleAgentMessage(agentMsg)
		}
		return h.handleAgentbootEvent(m)

	case map[string]interface{}:
		// Handle raw map messages (legacy support)
		return h.handleMapMessage(m)

	default:
		// Skip unknown message types
		logrus.WithField("msgType", fmt.Sprintf("%T", msg)).Debug("Skipping unknown message type")
		return nil
	}
}

func (f *streamingMessageHandler) OnApproval(context.Context, agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	// This should not be called - use CompositeHandler with ApprovalHandler instead
	logrus.Warn("OnApproval called on streamingMessageHandler - use CompositeHandler instead")
	return agentboot.PermissionResult{Approved: true}, nil
}

func (f *streamingMessageHandler) OnAsk(context.Context, agentboot.AskRequest) (agentboot.AskResult, error) {
	// This should not be called - use CompositeHandler with AskHandler instead
	logrus.Warn("OnAsk called on streamingMessageHandler - use CompositeHandler instead")
	return agentboot.AskResult{Approved: true}, nil
}

// OnComplete is called when the agent completes its task
func (f *streamingMessageHandler) OnComplete(result *agentboot.CompletionResult) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Drain any tool renders that never met a trailing text message so the
	// user sees them before the completion banner.
	f.flushToolBufferLocked()

	// Build action keyboard
	kb := feature.BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	// Prepare completion message based on verbose mode
	completionText := IconDone + " " + MsgTaskDone + ". " + MsgContinueOrHelp + BuildFooter(f.meta.AgentType, f.meta.ProjectPath)
	if !f.verbose {
		completionText = IconDone + " " + MsgTaskDone + ". (Quiet mode: /verbose to show details) " + MsgContinueOrHelp + BuildFooter(f.meta.AgentType, f.meta.ProjectPath)
	}

	_, err := f.bot.SendMessage(context.Background(), f.chatID, &imbot.SendMessageOptions{
		Text:     completionText,
		Metadata: buildActionCardMetadata(tgKeyboard, feature.BuildActionCard()),
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to send action keyboard")
	}
}

// handleAgentMessage processes unified agentboot.AgentMessage
func (h *streamingMessageHandler) handleAgentMessage(msg agentboot.AgentMessage) error {
	logrus.WithFields(logrus.Fields{
		"type":      msg.GetType(),
		"agentType": msg.GetAgentType(),
		"chatID":    h.chatID,
		"verbose":   h.verbose,
	}).Debug("Handling unified agent message")

	if h.bufferToolEvent(msg.GetType(), toolFieldsFromRaw(msg.GetRawData())) {
		return nil
	}

	switch msg.GetType() {
	case agentboot.EventTypeAssistant:
		if assistantMsg, ok := msg.(*agentboot.AssistantMessage); ok {
			h.sendText(assistantMsg.GetText())
		}
		return nil

	case agentboot.EventTypePermissionRequest:
		// Permission requests are handled by IMPrompter directly
		// In verbose mode, log for visibility; in quiet mode, silently handle
		if permMsg, ok := msg.(*agentboot.PermissionRequestMessage); ok {
			logrus.WithFields(logrus.Fields{
				"request_id": permMsg.RequestID,
				"tool_name":  permMsg.ToolName,
				"step":       permMsg.Step,
				"total":      permMsg.Total,
			}).Info("Permission request received (handled by IMPrompter)")
			// In quiet mode, don't show anything to user - IMPrompter will handle it
		}
		return nil

	case agentboot.EventTypePermissionResult:
		if permResultMsg, ok := msg.(*agentboot.PermissionResultMessage); ok {
			status := "denied"
			if permResultMsg.Approved {
				status = "approved"
			}
			logrus.WithFields(logrus.Fields{
				"request_id": permResultMsg.RequestID,
				"status":     status,
			}).Debug("Permission result received")
			// In quiet mode, don't show permission results to user
		}
		return nil

	case agentboot.EventTypeResult:
		// Result events are handled by OnComplete; still flush so a trailing
		// run of tool events shows up before OnComplete's completion banner.
		h.flushToolBufferLocked()
		if resultMsg, ok := msg.(*agentboot.ResultMessage); ok {
			logrus.WithFields(logrus.Fields{
				"status":  resultMsg.Status,
				"message": resultMsg.Message,
			}).Info("Agent result event received")
			// In quiet mode, result is shown by OnComplete, not here
		}
		return nil

	case agentboot.EventTypeInit:
		logrus.WithField("agentType", msg.GetAgentType()).Debug("Agent init event received")
		return nil

	case agentboot.EventTypeStreamDelta:
		if deltaMsg, ok := msg.(*agentboot.StreamDeltaMessage); ok {
			// For streaming, we could accumulate or send directly
			// In quiet mode, don't show stream deltas
			logrus.WithField("delta", deltaMsg.Delta).Debug("Stream delta received")
		}
		return nil

	default:
		logrus.WithField("type", msg.GetType()).Debug("Unhandled agent message type")
		return nil
	}
}

// maxToolHintLen bounds the input hint appended to a tool-use render.
const maxToolHintLen = 80

// renderToolUseSummary produces a one-line summary of a tool invocation for
// the main-path handlers, which don't carry the rich claude.TextFormatter.
func renderToolUseSummary(name string, input map[string]interface{}) string {
	if name == "" {
		name = "tool"
	}
	summary := IconTool + " " + name
	if hint := briefInputHint(input); hint != "" {
		summary += " " + hint
	}
	return summary
}

// renderToolResultSummary mirrors renderToolUseSummary for tool_result events.
func renderToolResultSummary(name string, isError bool) string {
	label := "ok"
	if isError {
		label = "error"
	}
	if name != "" {
		label = name + " " + label
	}
	return IconToolResult + " " + label
}

// briefInputHint picks a short, recognizable string from a tool's input map
// (file path, command, URL, query). The result is rune-safe truncated so a
// multibyte path or command can't be sliced mid-rune.
func briefInputHint(input map[string]interface{}) string {
	if input == nil {
		return ""
	}
	for _, k := range []string{"command", "file_path", "path", "url", "query"} {
		v, ok := input[k].(string)
		if !ok || v == "" {
			continue
		}
		if utf8.RuneCountInString(v) > maxToolHintLen {
			v = string([]rune(v)[:maxToolHintLen-1]) + "…"
		}
		return v
	}
	return ""
}

// toolEventFields is the source-agnostic shape the main-path handlers feed
// into bufferToolEvent. Per-source extraction lives in toolFieldsFrom*.
type toolEventFields struct {
	Name    string
	Input   map[string]interface{}
	IsError bool
}

// toolFieldsFromRaw reads tool fields from a flat map — the shape produced by
// agentboot.AgentMessage.GetRawData() and agentboot.Event.Data.
func toolFieldsFromRaw(raw map[string]interface{}) toolEventFields {
	if raw == nil {
		return toolEventFields{}
	}
	name, _ := raw["tool_name"].(string)
	input, _ := raw["input"].(map[string]interface{})
	isError, _ := raw["is_error"].(bool)
	return toolEventFields{Name: name, Input: input, IsError: isError}
}

// toolFieldsFromNestedMap reads tool fields from a map message whose fields
// may live either at the top level or nested under a "data" sub-object.
func toolFieldsFromNestedMap(m map[string]interface{}) toolEventFields {
	name, _ := mapNestedField[string](m, "tool_name")
	input, _ := mapNestedField[map[string]interface{}](m, "input")
	isError, _ := mapNestedField[bool](m, "is_error")
	return toolEventFields{Name: name, Input: input, IsError: isError}
}

// assistantTextFromMap resolves assistant text from a map message: a "message"
// field (top-level or nested under "data") preferred over a top-level "text".
func assistantTextFromMap(m map[string]interface{}) string {
	if v, ok := mapNestedField[string](m, "message"); ok && strings.TrimSpace(v) != "" {
		return v
	}
	text, _ := m["text"].(string)
	return text
}

// bufferToolEvent appends the render for a tool_use / tool_result event to the
// shared tool buffer. Returns true when the event was a tool event and fully
// handled, so callers can early-return. The caller must hold h.mu.
func (h *streamingMessageHandler) bufferToolEvent(eventType string, f toolEventFields) bool {
	switch eventType {
	case agentboot.EventTypeToolUse:
		h.appendToolBuffer(renderToolUseSummary(f.Name, f.Input))
		return true
	case agentboot.EventTypeToolResult:
		h.appendToolBuffer(renderToolResultSummary(f.Name, f.IsError))
		return true
	}
	return false
}

// sendText flushes any buffered tool renders and then sends text to the user.
// No-op when text is effectively empty. The caller must hold h.mu.
func (h *streamingMessageHandler) sendText(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	h.flushToolBufferLocked()
	h.sendMessage(text)
}

// handleClaudeMessage processes claude-specific messages.
//
// Messages are the splitting boundary for output: tool-only renders accumulate
// in toolBuffer, and a text-bearing message flushes the buffer (as a single
// aggregated message) before being sent itself. Quiet mode renders the
// flushed buffer as a short summary; verbose mode keeps one line per tool.
func (h *streamingMessageHandler) handleClaudeMessage(claudeMsg claude.Message) error {
	formatted := h.formatter.Format(claudeMsg)
	d, _ := json.Marshal(claudeMsg.GetRawData())
	logrus.Infof("[bot] Raw: %s", d)
	logrus.Infof("[bot] Formatted: %s", formatted)

	if strings.TrimSpace(formatted) == "" {
		logrus.WithField("msgType", claudeMsg.GetType()).Debug("Skipping empty formatted message")
		return nil
	}

	if isToolOnlyClaudeMessage(claudeMsg) {
		h.appendToolBuffer(formatted)
		return nil
	}

	// Text-bearing message. Flush the buffered tool renders first so messages
	// always act as the splitting boundary, even when the current message is
	// itself going to be suppressed by the quiet filter below.
	h.flushToolBufferLocked()

	// In quiet mode, only assistant text and final agent results reach the
	// chat; user echoes, system events, and stream-event noise are dropped.
	if !h.verbose {
		msgType := claudeMsg.GetType()
		if msgType != "result" && msgType != "assistant" {
			logrus.WithField("msgType", msgType).Debug("Quiet mode: suppressing non-result message")
			return nil
		}
	}

	h.sendMessage(formatted)
	return nil
}

// isToolOnlyClaudeMessage reports whether the message carries only tool
// activity (no user-facing text). Such messages are buffered until the next
// text-bearing message flushes them.
func isToolOnlyClaudeMessage(msg claude.Message) bool {
	switch m := msg.(type) {
	case *claude.ToolUseMessage, *claude.ToolResultMessage:
		return true
	case *claude.AssistantMessage:
		if m == nil {
			return false
		}
		for _, c := range m.Message.Content {
			if c.Type == claude.ContentBlockTypeText && strings.TrimSpace(c.Text) != "" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// appendToolBuffer records a formatted tool render. The caller must hold h.mu.
// When the buffer reaches toolBufferFlushThreshold it is flushed immediately
// so the user still sees progress on long tool chains.
func (h *streamingMessageHandler) appendToolBuffer(formatted string) {
	if strings.TrimSpace(formatted) == "" {
		return
	}
	h.toolBuffer = append(h.toolBuffer, formatted)
	if len(h.toolBuffer) >= toolBufferFlushThreshold {
		h.flushToolBufferLocked()
	}
}

// flushToolBufferLocked sends any buffered tool renders as a single aggregated
// message and clears the buffer. The caller must hold h.mu.
func (h *streamingMessageHandler) flushToolBufferLocked() {
	if len(h.toolBuffer) == 0 {
		return
	}
	text := h.renderToolBuffer(h.toolBuffer)
	h.toolBuffer = h.toolBuffer[:0]
	if strings.TrimSpace(text) == "" {
		return
	}
	h.sendMessage(text)
}

// renderToolBuffer turns the accumulated tool renders into the single message
// that will be sent to the user. Verbose mode keeps every entry on its own
// line; quiet mode collapses to a count + preview of the first few entries.
func (h *streamingMessageHandler) renderToolBuffer(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if h.verbose {
		return strings.Join(items, "\n")
	}

	previewN := quietToolPreviewCount
	if previewN > len(items) {
		previewN = len(items)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %d tool call(s)\n", IconTool, len(items))
	for _, p := range items[:previewN] {
		b.WriteString("• ")
		b.WriteString(p)
		b.WriteString("\n")
	}
	if rest := len(items) - previewN; rest > 0 {
		fmt.Fprintf(&b, "…(+%d more)", rest)
	}
	return strings.TrimRight(b.String(), "\n")
}

// handleAgentbootEvent processes agentboot.Event messages (fallback for unknown event types)
func (h *streamingMessageHandler) handleAgentbootEvent(event agentboot.Event) error {
	logrus.WithFields(logrus.Fields{
		"eventType": event.Type,
		"chatID":    h.chatID,
	}).Debug("Handling agentboot event")

	if h.bufferToolEvent(event.Type, toolFieldsFromRaw(event.Data)) {
		return nil
	}

	switch event.Type {
	case agentboot.EventTypeAssistant:
		h.sendText(assistantTextFromMap(event.Data))
	case agentboot.EventTypePermissionRequest:
		logrus.WithFields(logrus.Fields{
			"request_id": event.Data["request_id"],
			"tool_name":  event.Data["tool_name"],
		}).Info("Permission request event received (handled by IMPrompter)")
	case agentboot.EventTypePermissionResult:
		logrus.WithField("request_id", event.Data["request_id"]).Debug("Permission result event")
	case agentboot.EventTypeResult:
		// Flush trailing tool renders before OnComplete emits the banner.
		h.flushToolBufferLocked()
		status, _ := event.Data["status"].(string)
		logrus.WithField("status", status).Info("Agent result event received")
	case agentboot.EventTypeInit, agentboot.EventTypeSystem:
		logrus.WithField("data", event.Data).Debug("System/init event received")
	default:
		logrus.WithField("eventType", event.Type).Debug("Unhandled event type")
	}
	return nil
}

// handleMapMessage processes raw map messages (legacy support)
func (h *streamingMessageHandler) handleMapMessage(m map[string]interface{}) error {
	msgType, ok := m["type"].(string)
	if !ok {
		logrus.WithField("map", m).Debug("Map message without type field")
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"type":   msgType,
		"chatID": h.chatID,
	}).Debug("Handling map message")

	if h.bufferToolEvent(msgType, toolFieldsFromNestedMap(m)) {
		return nil
	}

	switch msgType {
	case agentboot.EventTypePermissionRequest:
		// Permission requests come from mock agent before going through IMPrompter
		data, _ := m["data"].(map[string]interface{})
		if data != nil {
			logrus.WithFields(logrus.Fields{
				"request_id": data["request_id"],
				"tool_name":  data["tool_name"],
			}).Info("Permission request received (will be handled by IMPrompter)")
		}
	case agentboot.EventTypeAssistant:
		h.sendText(assistantTextFromMap(m))
	case agentboot.EventTypeResult:
		// Flush trailing tool renders before OnComplete's banner.
		h.flushToolBufferLocked()
	default:
		logrus.WithField("type", msgType).Debug("Unhandled map message type")
	}
	return nil
}

// mapNestedField reads a typed field from a map message, looking at the top
// level first and then under a "data" sub-object. Map messages from different
// agents put fields at either depth.
func mapNestedField[T any](m map[string]interface{}, key string) (T, bool) {
	if v, ok := m[key].(T); ok {
		return v, true
	}
	if data, ok := m["data"].(map[string]interface{}); ok {
		if v, ok := data[key].(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// OnError implements agentboot.MessageStreamer
func (h *streamingMessageHandler) OnError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Surface any buffered tool activity before the error so the user has
	// context on what was happening when the failure occurred.
	h.flushToolBufferLocked()

	errStr := err.Error()
	var errMsg string

	// Check for session ID conflict error and provide helpful message
	if strings.Contains(errStr, "Session ID") && strings.Contains(errStr, "already in use") {
		errMsg = fmt.Sprintf("⚠️ **Session ID Conflict**\n\nThe Claude CLI reported: %v\n\nThis usually means:\n• Another Claude Code process is using this session\n• The session file is locked\n\nTry using `/stop` to end the current session, then retry.", err)
	} else {
		errMsg = fmt.Sprintf("[ERROR] %v", err)
	}

	h.sendMessage(errMsg)
}

// GetOutput returns the accumulated output (for compatibility, returns empty as we stream immediately)
func (h *streamingMessageHandler) GetOutput() string {
	return ""
}

// sendMessage sends a message to the bot
// Note: Platform handles chunking internally via BaseBot.ChunkText()
func (h *streamingMessageHandler) sendMessage(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	_, err := h.bot.SendMessage(context.Background(), h.chatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		ReplyTo:   h.replyTo,
	})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"chatID":  h.chatID,
			"replyTo": h.replyTo,
			"error":   err,
			"textLen": len(text),
		}).Error("Failed to send streaming message")
		return
	}
	logrus.WithField("chatID", h.chatID).WithField("textLen", len(text)).Debug("Sent streaming message")
}
