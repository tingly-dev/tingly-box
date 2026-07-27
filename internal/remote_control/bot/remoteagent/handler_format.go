package remoteagent

// formatResponseWithFooter adds a compact footer (agent + path) to the response
func (h *BotHandler) formatResponseWithFooter(meta ResponseMeta, response string) string {
	return response + BuildFooter(meta.AgentType, meta.ProjectPath)
}

// newStreamingMessageHandler creates a new streaming message handler
func (h *BotHandler) newStreamingMessageHandler(hCtx HandlerContext, meta *ResponseMeta) *streamingMessageHandler {
	return newStreamingMessageHandler(hCtx.Bot, hCtx.ChatID, hCtx.MessageID, h.GetVerbose(hCtx.ChatID), meta)
}
