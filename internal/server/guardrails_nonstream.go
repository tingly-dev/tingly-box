package server

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	serverguardrails "github.com/tingly-dev/tingly-box/internal/server/guardrails"
)

func (s *Server) applyGuardrailsToAnthropicV1NonStreamResponse(c *gin.Context, session guardrailsSession, messageHistory []guardrails.Message, resp *anthropic.Message) bool {
	if resp == nil || !s.guardrailsEnabledForSession(session) {
		return false
	}

	input := s.buildGuardrailsBaseInput(session, guardrails.DirectionResponse, messageHistory)
	input.Content = guardrails.Content{
		Messages: messageHistory,
		Text:     anthropicResponseText(resp.Content),
		Command:  anthropicResponseCommand(resp.Content),
	}

	var hookResult serverguardrails.GuardrailsHookResult
	done := serverguardrails.NewNonStreamGuardrailsHook(
		s.guardrailsEngine,
		input,
		serverguardrails.WithGuardrailsContext(context.Background()),
		serverguardrails.WithGuardrailsOnVerdict(func(result serverguardrails.GuardrailsHookResult) {
			hookResult = result
		}),
	)
	if done != nil {
		done()
	}
	if hookResult.Err != nil {
		c.Set("guardrails_error", hookResult.Err.Error())
		return false
	}
	c.Set("guardrails_result", hookResult.Result)
	if hookResult.Result.Verdict != guardrails.VerdictBlock {
		return false
	}

	blockMessage := serverguardrails.BlockMessageWithSnippet(hookResult.Result, input.Content.Preview(120))
	if input.Content.Command != nil {
		blockMessage = serverguardrails.BlockMessageForCommand(hookResult.Result, input.Content.Command.Name, input.Content.Command.Arguments)
	}
	c.Set("guardrails_block_message", blockMessage)
	s.recordGuardrailsHistory(c, session, input, hookResult.Result, "response", blockMessage)
	overwriteAnthropicResponse(resp, blockMessage)
	return true
}

func (s *Server) applyGuardrailsToAnthropicV1BetaNonStreamResponse(c *gin.Context, session guardrailsSession, messageHistory []guardrails.Message, resp *anthropic.BetaMessage) bool {
	if resp == nil || !s.guardrailsEnabledForSession(session) {
		return false
	}

	input := s.buildGuardrailsBaseInput(session, guardrails.DirectionResponse, messageHistory)
	input.Content = guardrails.Content{
		Messages: messageHistory,
		Text:     anthropicBetaResponseText(resp.Content),
		Command:  anthropicBetaResponseCommand(resp.Content),
	}

	var hookResult serverguardrails.GuardrailsHookResult
	done := serverguardrails.NewNonStreamGuardrailsHook(
		s.guardrailsEngine,
		input,
		serverguardrails.WithGuardrailsContext(context.Background()),
		serverguardrails.WithGuardrailsOnVerdict(func(result serverguardrails.GuardrailsHookResult) {
			hookResult = result
		}),
	)
	if done != nil {
		done()
	}
	if hookResult.Err != nil {
		c.Set("guardrails_error", hookResult.Err.Error())
		return false
	}
	c.Set("guardrails_result", hookResult.Result)
	if hookResult.Result.Verdict != guardrails.VerdictBlock {
		return false
	}

	blockMessage := serverguardrails.BlockMessageWithSnippet(hookResult.Result, input.Content.Preview(120))
	if input.Content.Command != nil {
		blockMessage = serverguardrails.BlockMessageForCommand(hookResult.Result, input.Content.Command.Name, input.Content.Command.Arguments)
	}
	c.Set("guardrails_block_message", blockMessage)
	s.recordGuardrailsHistory(c, session, input, hookResult.Result, "response", blockMessage)
	overwriteAnthropicBetaResponse(resp, blockMessage)
	return true
}

func anthropicResponseText(blocks []anthropic.ContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if (block.Type == "text" || block.Type == "thinking") && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func anthropicBetaResponseText(blocks []anthropic.BetaContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		case "thinking":
			if strings.TrimSpace(block.Thinking) != "" {
				parts = append(parts, block.Thinking)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func anthropicResponseCommand(blocks []anthropic.ContentBlockUnion) *guardrails.Command {
	for _, block := range blocks {
		if block.Type != "tool_use" && block.Type != "server_tool_use" {
			continue
		}
		return &guardrails.Command{
			Name:      block.Name,
			Arguments: parseAnthropicInput(block.Input),
		}
	}
	return nil
}

func anthropicBetaResponseCommand(blocks []anthropic.BetaContentBlockUnion) *guardrails.Command {
	for _, block := range blocks {
		if block.Type != "tool_use" && block.Type != "server_tool_use" {
			continue
		}
		return &guardrails.Command{
			Name:      block.Name,
			Arguments: parseAnthropicInput(block.Input),
		}
	}
	return nil
}

func parseAnthropicInput(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err == nil {
		return parsed
	}
	return map[string]interface{}{"_raw": string(raw)}
}

func overwriteAnthropicResponse(resp *anthropic.Message, message string) {
	resp.Content = []anthropic.ContentBlockUnion{{
		Type: "text",
		Text: message,
	}}
	resp.StopReason = anthropic.StopReasonEndTurn
}

func overwriteAnthropicBetaResponse(resp *anthropic.BetaMessage, message string) {
	resp.Content = []anthropic.BetaContentBlockUnion{{
		Type: "text",
		Text: message,
	}}
	resp.StopReason = anthropic.BetaStopReasonEndTurn
}
