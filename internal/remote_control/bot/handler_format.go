package bot

import (
	"fmt"
	"strings"
)

func (h *BotHandler) formatResponseWithHeader(meta ResponseMeta, response string, showMeta bool) string {
	var buf strings.Builder

	// Show meta information only when explicitly requested
	if showMeta {
		// Show agent indicator
		if meta.AgentType != "" {
			buf.WriteString(fmt.Sprintf(FormatAgentLine, GetAgentIcon(meta.AgentType), GetAgentDisplayName(meta.AgentType)))
		}

		// Always show project path (shortened)
		if meta.ProjectPath != "" {
			buf.WriteString(fmt.Sprintf(FormatProjectLine, IconProject, ShortenPath(meta.ProjectPath)))
		}

		// Show IDs for transparency
		if meta.ChatID != "" {
			buf.WriteString(fmt.Sprintf(FormatDebugLine, IconChat, meta.ChatID))
		}
		if meta.UserID != "" {
			buf.WriteString(fmt.Sprintf(FormatDebugLine, IconUser, meta.UserID))
		}
		if meta.SessionID != "" {
			buf.WriteString(fmt.Sprintf(FormatDebugLine, IconSession, ShortenID(meta.SessionID, 8)))
		}

		buf.WriteString(SeparatorLine + "\n\n")
	}

	return buf.String() + response
}

// formatResponseWithFooter adds a compact footer (agent + path) to the response
func (h *BotHandler) formatResponseWithFooter(meta ResponseMeta, response string) string {
	return response + BuildFooter(meta.AgentType, meta.ProjectPath)
}

// getOutputBehaviorForChat returns the output behavior for a specific chat
// Combines bot-level defaults with chat-level overrides
func (h *BotHandler) getOutputBehaviorForChat(chatID string) OutputBehavior {
	return h.botSetting.GetOutputBehavior()
}

// newStreamingMessageHandler creates a new streaming message handler
func (h *BotHandler) newStreamingMessageHandler(hCtx HandlerContext, meta *ResponseMeta) *streamingMessageHandler {
	return newStreamingMessageHandler(hCtx.Bot, hCtx.ChatID, hCtx.MessageID, h.GetVerbose(hCtx.ChatID), meta)
}
