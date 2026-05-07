package mcp

import "fmt"

func validateAndNormalizeMixedStash(
	externalIDs []string,
	results []ToolExecutionResult,
) ([]ToolExecutionResult, error) {
	hasExternalAnchor := false
	for _, id := range externalIDs {
		if id != "" {
			hasExternalAnchor = true
			break
		}
	}
	if !hasExternalAnchor {
		return nil, fmt.Errorf("no valid external tool anchors")
	}

	normalized := make([]ToolExecutionResult, 0, len(results))
	for _, r := range results {
		if r.ToolUseID != "" {
			normalized = append(normalized, r)
		}
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("no valid virtual tool results")
	}

	return normalized, nil
}
