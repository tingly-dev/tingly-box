package pipeline

import (
	"sort"
	"time"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
)

func recordGuardrailsMaskHistory(
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	state *guardrailscore.CredentialMaskState,
	phase string,
) {
	if runtime == nil || runtime.History == nil || state == nil {
		return
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

	credentialRefs := sortedKeys(refSet)
	aliasHits := sortedKeys(aliasSet)
	if len(credentialRefs) == 0 && len(aliasHits) == 0 {
		return
	}

	entry := guardrailsutils.Entry{
		Time:            time.Now(),
		Scenario:        input.Scenario,
		Model:           input.Model,
		Provider:        input.ProviderName(),
		Direction:       string(input.Direction),
		Phase:           phase,
		Verdict:         string(guardrailscore.VerdictMask),
		Preview:         input.Content.LatestPreview(160),
		CredentialRefs:  credentialRefs,
		CredentialNames: runtime.CredentialNames(credentialRefs),
		AliasHits:       aliasHits,
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	runtime.History.Add(entry)
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
