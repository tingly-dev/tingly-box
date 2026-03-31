package serverguardrails

import (
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

func CollectHistoryCredentialRefs(result guardrails.Result) []string {
	refSet := make(map[string]struct{})

	for _, reason := range result.Reasons {
		rawRefs, ok := reason.Evidence["credential_refs"]
		if !ok {
			continue
		}
		switch typed := rawRefs.(type) {
		case []string:
			for _, ref := range typed {
				if ref != "" {
					refSet[ref] = struct{}{}
				}
			}
		case []interface{}:
			for _, item := range typed {
				if ref, ok := item.(string); ok && ref != "" {
					refSet[ref] = struct{}{}
				}
			}
		}
	}

	return sortedKeys(refSet)
}

func CollectMaskHistoryCredentialData(c *gin.Context) ([]string, []string) {
	if c == nil {
		return nil, nil
	}

	existing, ok := c.Get(guardrails.CredentialMaskStateContextKey)
	if !ok {
		return nil, nil
	}
	state, ok := existing.(*guardrails.CredentialMaskState)
	if !ok || state == nil {
		return nil, nil
	}

	refSet := make(map[string]struct{})
	aliasSet := make(map[string]struct{})
	for _, ref := range state.UsedRefs {
		if ref != "" {
			refSet[ref] = struct{}{}
		}
	}
	for alias := range state.AliasToReal {
		if alias != "" {
			aliasSet[alias] = struct{}{}
		}
	}
	return sortedKeys(refSet), sortedKeys(aliasSet)
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
