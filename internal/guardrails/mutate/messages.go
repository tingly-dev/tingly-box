package mutate

import (
	"encoding/json"
	"strings"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
)

// BlockMessageWithSnippet formats a block message for text responses.
func BlockMessageWithSnippet(result guardrailscore.Result, snippet string) string {
	prefix := "Blocked by guardrails. Content: text."
	suffix := ""
	if snippet != "" {
		suffix = " Snippet: \"" + snippet + "\""
	}
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return prefix + " Reason: " + result.Reasons[0].Reason + "." + suffix
	}
	if suffix != "" {
		return prefix + suffix
	}
	return prefix
}

// BlockMessageForToolResult formats a block message for tool_result filtering.
func BlockMessageForToolResult(result guardrailscore.Result) string {
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return "Blocked by guardrails. Content: tool_result. Output redacted. Reason: " + result.Reasons[0].Reason
	}
	return "Blocked by guardrails. Content: tool_result. Output redacted."
}

// BlockMessageForCommand formats a block message for tool_use command blocking.
func BlockMessageForCommand(result guardrailscore.Result, name string, args map[string]interface{}) string {
	command := formatGuardrailsCommand(name, args)
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return "Blocked by guardrails. Content: command. Command: " + command + ". Reason: " + result.Reasons[0].Reason
	}
	return "Blocked by guardrails. Content: command. Command: " + command + "."
}

// BlockMessageForEvaluation builds the user-visible block text from a completed
// evaluation. Command responses get a command-specific message; plain text
// responses use a preview snippet.
func BlockMessageForEvaluation(evaluation guardrailsevaluate.Evaluation) string {
	if evaluation.Input.Content.Command != nil {
		return BlockMessageForCommand(
			evaluation.Result,
			evaluation.Input.Content.Command.Name,
			evaluation.Input.Content.Command.Arguments,
		)
	}
	return BlockMessageWithSnippet(evaluation.Result, evaluation.Input.Content.Preview(120))
}

func formatGuardrailsCommand(name string, args map[string]interface{}) string {
	if name == "" {
		return "<unknown>"
	}
	cmd := &guardrailscore.Command{
		Name:      name,
		Arguments: args,
	}
	cmd.AttachDerivedFields()
	if cmd.Normalized != nil {
		parts := []string{name}
		if len(cmd.Normalized.Actions) > 0 {
			parts = append(parts, "actions="+strings.Join(cmd.Normalized.Actions, ","))
		}
		if len(cmd.Normalized.Resources) > 0 {
			parts = append(parts, "resources="+strings.Join(cmd.Normalized.Resources, ","))
		}
		if cmd.Normalized.Raw != "" {
			raw := cmd.Normalized.Raw
			const maxRawLen = 180
			if len(raw) > maxRawLen {
				raw = raw[:maxRawLen] + "..."
			}
			parts = append(parts, "raw="+raw)
		}
		return strings.Join(parts, " ")
	}
	if len(args) == 0 {
		return name + " {}"
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return name + " {\"error\":\"marshal\"}"
	}
	const maxLen = 300
	payload := string(raw)
	if len(payload) > maxLen {
		payload = payload[:maxLen] + "..."
	}
	return name + " " + payload
}
