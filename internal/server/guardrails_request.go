package server

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) applyGuardrailsToToolResultV1(c *gin.Context, req *anthropic.MessageNewParams, actualModel string, provider *typ.Provider) {
	if s.guardrailsEngine == nil {
		return
	}
	_, _, _, requestModel, scenario, _, _ := GetTrackingContext(c)
	enabled := s.config.GetScenarioFlag(typ.RuleScenario(scenario), "guardrails") ||
		s.config.GetScenarioFlag(typ.ScenarioGlobal, "guardrails")
	if !enabled || scenario != string(typ.ScenarioClaudeCode) {
		return
	}

	toolResultText, toolResultBlocks, toolResultParts := extractToolResultTextV1(req.Messages)
	logrus.Debugf("Guardrails: tool_result detected (v1) blocks=%d parts=%d len=%d", toolResultBlocks, toolResultParts, len(toolResultText))
	if toolResultText == "" {
		return
	}

	input := guardrails.Input{
		Scenario:  scenario,
		Model:     actualModel,
		Direction: guardrails.DirectionRequest,
		Content: guardrails.Content{
			Text:     toolResultText,
			Messages: guardrailsMessagesFromAnthropicV1(req.System, req.Messages),
		},
		Metadata: map[string]interface{}{
			"provider":      provider.Name,
			"request_model": requestModel,
		},
	}

	result, err := s.guardrailsEngine.Evaluate(c.Request.Context(), input)
	if err != nil {
		return
	}
	if result.Verdict == guardrails.VerdictBlock {
		message := guardrailsBlockMessage(result)
		replaceToolResultContentV1(req.Messages, message)
		logrus.Debugf("Guardrails: tool_result replaced (v1) len=%d", len(message))
		return
	}
}

func (s *Server) applyGuardrailsToToolResultV1Beta(c *gin.Context, req *anthropic.BetaMessageNewParams, actualModel string, provider *typ.Provider) {
	if s.guardrailsEngine == nil {
		return
	}
	_, _, _, requestModel, scenario, _, _ := GetTrackingContext(c)
	enabled := s.config.GetScenarioFlag(typ.RuleScenario(scenario), "guardrails") ||
		s.config.GetScenarioFlag(typ.ScenarioGlobal, "guardrails")
	if !enabled || scenario != string(typ.ScenarioClaudeCode) {
		return
	}

	toolResultText, toolResultBlocks, toolResultParts := extractToolResultTextV1Beta(req.Messages)
	logrus.Debugf("Guardrails: tool_result detected (v1beta) blocks=%d parts=%d len=%d", toolResultBlocks, toolResultParts, len(toolResultText))
	if toolResultText == "" {
		return
	}

	input := guardrails.Input{
		Scenario:  scenario,
		Model:     actualModel,
		Direction: guardrails.DirectionRequest,
		Content: guardrails.Content{
			Text:     toolResultText,
			Messages: guardrailsMessagesFromAnthropicV1Beta(req.System, req.Messages),
		},
		Metadata: map[string]interface{}{
			"provider":      provider.Name,
			"request_model": requestModel,
		},
	}

	result, err := s.guardrailsEngine.Evaluate(c.Request.Context(), input)
	if err != nil {
		return
	}
	if result.Verdict == guardrails.VerdictBlock {
		message := guardrailsBlockMessage(result)
		replaceToolResultContentV1Beta(req.Messages, message)
		logrus.Debugf("Guardrails: tool_result replaced (v1beta) len=%d", len(message))
		return
	}
}

func extractToolResultTextV1(messages []anthropic.MessageParam) (string, int, int) {
	var b strings.Builder
	var blocks int
	var parts int
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			blocks++
			for _, content := range block.OfToolResult.Content {
				parts++
				if content.OfText != nil {
					b.WriteString(content.OfText.Text)
					continue
				}
				if raw, err := json.Marshal(content); err == nil {
					b.WriteString(string(raw))
				}
			}
		}
	}
	return b.String(), blocks, parts
}

func extractToolResultTextV1Beta(messages []anthropic.BetaMessageParam) (string, int, int) {
	var b strings.Builder
	var blocks int
	var parts int
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			blocks++
			for _, content := range block.OfToolResult.Content {
				parts++
				if content.OfText != nil {
					b.WriteString(content.OfText.Text)
					continue
				}
				if raw, err := json.Marshal(content); err == nil {
					b.WriteString(string(raw))
				}
			}
		}
	}
	return b.String(), blocks, parts
}

func replaceToolResultContentV1(messages []anthropic.MessageParam, message string) {
	for i := range messages {
		msg := &messages[i]
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfToolResult == nil {
				continue
			}
			block.OfToolResult.IsError = anthropic.Bool(true)
			block.OfToolResult.Content = []anthropic.ToolResultBlockParamContentUnion{
				{
					OfText: &anthropic.TextBlockParam{
						Text: message,
					},
				},
			}
		}
	}
}

func replaceToolResultContentV1Beta(messages []anthropic.BetaMessageParam, message string) {
	for i := range messages {
		msg := &messages[i]
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfToolResult == nil {
				continue
			}
			block.OfToolResult.IsError = anthropic.Bool(true)
			block.OfToolResult.Content = []anthropic.BetaToolResultBlockParamContentUnion{
				{
					OfText: &anthropic.BetaTextBlockParam{
						Text: message,
					},
				},
			}
		}
	}
}

func guardrailsTextResponse(model, message string) map[string]interface{} {
	return map[string]interface{}{
		"id":   "guardrails_blocked",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": message,
			},
		},
		"model":         model,
		"stop_reason":   "guardrails",
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": 0,
		},
	}
}
