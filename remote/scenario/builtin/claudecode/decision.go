package claudecode

import "github.com/tingly-dev/tingly-box/remote/interaction"

// encodeDecision converts an interaction.Reply into the JSON shape
// Claude Code hooks expect on stdout. For permission flows that's
// hookSpecificOutput.permissionDecision; for AskUserQuestion it's the
// answers map under hookSpecificOutput.
func encodeDecision(input HookInput, reply interaction.Reply) map[string]any {
	if input.ToolName == "AskUserQuestion" {
		answers := map[string]any{}
		if reply.Meta != nil {
			if updated, ok := reply.Meta["updated_input"].(map[string]interface{}); ok {
				if a, ok := updated["answers"].(map[string]interface{}); ok {
					for k, v := range a {
						answers[k] = v
					}
				}
			}
		}
		return map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName": input.HookEventName,
				"answers":       answers,
			},
			"systemMessage": summarizeAnswers(answers),
		}
	}

	decision := "deny"
	switch reply.Selected {
	case "allow":
		decision = "allow"
	case "deny":
		decision = "deny"
	default:
		if reply.Status == interaction.StatusCancelled {
			decision = "deny"
		}
	}
	reason := reply.FreeText
	if reason == "" {
		switch decision {
		case "allow":
			reason = "Approved via IM"
		default:
			if reply.Status == interaction.StatusCancelled {
				reason = "Cancelled via IM"
			} else {
				reason = "Denied via IM"
			}
		}
	}
	return map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            input.HookEventName,
			"permissionDecision":       decision,
			"permissionDecisionReason": reason,
		},
		"systemMessage": reason,
	}
}

// fallbackDecision builds the hook output for a timeout / disconnect
// path. policy is one of "allow" / "deny" / "ask".
func fallbackDecision(input HookInput, policy string, reason string) map[string]any {
	if policy == "" {
		policy = "deny"
	}
	if reason == "" {
		reason = "no IM response within budget"
	}
	return map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            input.HookEventName,
			"permissionDecision":       policy,
			"permissionDecisionReason": reason,
		},
		"systemMessage": reason,
	}
}

func summarizeAnswers(answers map[string]any) string {
	if len(answers) == 0 {
		return "User did not answer"
	}
	out := "User answered:"
	for q, a := range answers {
		out += "\n- " + q + ": "
		if s, ok := a.(string); ok {
			out += s
		}
	}
	return out
}
