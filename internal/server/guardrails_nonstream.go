package server

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// applyGuardrailsToAnthropicV1NonStreamResponse evaluates a fully assembled
// Anthropic v1 response and rewrites it to a text block when guardrails block it.
func (s *Server) applyGuardrailsToAnthropicV1NonStreamResponse(c *gin.Context, actualModel string, provider *typ.Provider, messageHistory []guardrailscore.Message, resp *anthropic.Message) bool {
	if resp == nil {
		return false
	}
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if !s.guardrailsEnabledForScenario(scenario) {
		return false
	}

	input := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messageHistory)
	input.Content = guardrailscore.Content{
		Messages: messageHistory,
		Text:     anthropicResponseText(resp.Content),
		Command:  anthropicResponseCommand(resp.Content),
	}

	var hookResult GuardrailsHookResult
	done := NewNonStreamGuardrailsHook(
		s.guardrailsRuntime,
		input,
		WithGuardrailsContext(context.Background()),
		WithGuardrailsOnVerdict(func(result GuardrailsHookResult) {
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
	if hookResult.Result.Verdict != guardrailscore.VerdictBlock {
		return false
	}

	blockMessage := BlockMessageWithSnippet(hookResult.Result, input.Content.Preview(120))
	if input.Content.Command != nil {
		blockMessage = BlockMessageForCommand(hookResult.Result, input.Content.Command.Name, input.Content.Command.Arguments)
	}
	c.Set("guardrails_block_message", blockMessage)
	s.recordGuardrailsHistory(input, hookResult.Result, "response", blockMessage)
	overwriteAnthropicResponse(resp, blockMessage)
	return true
}

// applyGuardrailsToAnthropicV1BetaNonStreamResponse is the beta equivalent of
// applyGuardrailsToAnthropicV1NonStreamResponse.
func (s *Server) applyGuardrailsToAnthropicV1BetaNonStreamResponse(c *gin.Context, actualModel string, provider *typ.Provider, messageHistory []guardrailscore.Message, resp *anthropic.BetaMessage) bool {
	if resp == nil {
		return false
	}
	_, _, _, _, scenario, _, _ := GetTrackingContext(c)
	if !s.guardrailsEnabledForScenario(scenario) {
		return false
	}

	input := s.buildGuardrailsBaseInput(c, actualModel, provider, guardrailscore.DirectionResponse, messageHistory)
	input.Content = guardrailscore.Content{
		Messages: messageHistory,
		Text:     anthropicBetaResponseText(resp.Content),
		Command:  anthropicBetaResponseCommand(resp.Content),
	}

	var hookResult GuardrailsHookResult
	done := NewNonStreamGuardrailsHook(
		s.guardrailsRuntime,
		input,
		WithGuardrailsContext(context.Background()),
		WithGuardrailsOnVerdict(func(result GuardrailsHookResult) {
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
	if hookResult.Result.Verdict != guardrailscore.VerdictBlock {
		return false
	}

	blockMessage := BlockMessageWithSnippet(hookResult.Result, input.Content.Preview(120))
	if input.Content.Command != nil {
		blockMessage = BlockMessageForCommand(hookResult.Result, input.Content.Command.Name, input.Content.Command.Arguments)
	}
	c.Set("guardrails_block_message", blockMessage)
	s.recordGuardrailsHistory(input, hookResult.Result, "response", blockMessage)
	overwriteAnthropicBetaResponse(resp, blockMessage)
	return true
}

// anthropicResponseText collects the response-side text payload used for content
// policy evaluation. Thinking text is included because it is part of the returned
// model output in non-stream mode.
func anthropicResponseText(blocks []anthropic.ContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if (block.Type == "text" || block.Type == "thinking") && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// anthropicBetaResponseText collects text and thinking content from beta
// responses for privacy/content evaluation.
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

// anthropicResponseCommand extracts the first tool_use-like block and adapts it
// into the shared guardrails command shape.
func anthropicResponseCommand(blocks []anthropic.ContentBlockUnion) *guardrailscore.Command {
	for _, block := range blocks {
		if block.Type != "tool_use" && block.Type != "server_tool_use" {
			continue
		}
		return &guardrailscore.Command{
			Name:      block.Name,
			Arguments: parseAnthropicInput(block.Input),
		}
	}
	return nil
}

// anthropicBetaResponseCommand extracts the first beta tool_use-like block for
// command evaluation.
func anthropicBetaResponseCommand(blocks []anthropic.BetaContentBlockUnion) *guardrailscore.Command {
	for _, block := range blocks {
		if block.Type != "tool_use" && block.Type != "server_tool_use" {
			continue
		}
		return &guardrailscore.Command{
			Name:      block.Name,
			Arguments: parseAnthropicInput(block.Input),
		}
	}
	return nil
}

// parseAnthropicInput best-effort decodes raw tool input into a map so command
// policies can evaluate structured arguments.
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

// overwriteAnthropicResponse replaces the original non-stream response content
// with a single text block carrying the guardrails block message.
func overwriteAnthropicResponse(resp *anthropic.Message, message string) {
	resp.Content = []anthropic.ContentBlockUnion{{
		Type: "text",
		Text: message,
	}}
	resp.StopReason = anthropic.StopReasonEndTurn
}

// overwriteAnthropicBetaResponse replaces the original beta non-stream response
// content with a single text block carrying the guardrails block message.
func overwriteAnthropicBetaResponse(resp *anthropic.BetaMessage, message string) {
	resp.Content = []anthropic.BetaContentBlockUnion{{
		Type: "text",
		Text: message,
	}}
	resp.StopReason = anthropic.BetaStopReasonEndTurn
}
