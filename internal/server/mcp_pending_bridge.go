package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	mcpmodule "github.com/tingly-dev/tingly-box/internal/server/module/mcp"
)

// StashPendingVirtualToolResults bridges generic MCP module results into server pending-result store.
func (s *Server) StashPendingVirtualToolResults(anchorIDs []string, results []mcpmodule.VirtualToolExecutionResult) {
	if len(results) == 0 {
		return
	}
	internalResults := make([]virtualToolExecutionResult, 0, len(results))
	for _, r := range results {
		internalResults = append(internalResults, virtualToolExecutionResult{
			ToolUseID: r.ToolUseID,
			Content:   r.Content,
			IsError:   r.IsError,
		})
	}
	s.stashPendingVirtualToolResults(anchorIDs, internalResults)
}

// InjectPendingVirtualResultsAnthropicV1 bridges generic MCP module injection into server logic.
func (s *Server) InjectPendingVirtualResultsAnthropicV1(req *anthropic.MessageNewParams) {
	s.injectPendingVirtualResultsAnthropicV1(req)
}

// InjectPendingVirtualResultsAnthropicBeta bridges generic MCP module injection into server logic.
func (s *Server) InjectPendingVirtualResultsAnthropicBeta(req *anthropic.BetaMessageNewParams) {
	s.injectPendingVirtualResultsAnthropicBeta(req)
}

// InjectPendingVirtualResultsOpenAI bridges generic MCP module injection into server logic.
func (s *Server) InjectPendingVirtualResultsOpenAI(req *openai.ChatCompletionNewParams) {
	s.injectPendingVirtualResultsOpenAI(req)
}
