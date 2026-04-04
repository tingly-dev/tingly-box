package server

import (
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
)

func CollectMaskHistoryCredentialData(c *gin.Context) ([]string, []string) {
	if c == nil {
		return nil, nil
	}

	existing, ok := c.Get(guardrailscore.CredentialMaskStateContextKey)
	if !ok {
		return nil, nil
	}
	state, ok := existing.(*guardrailscore.CredentialMaskState)
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
	return sortedMaskHistoryKeys(refSet), sortedMaskHistoryKeys(aliasSet)
}

func sortedMaskHistoryKeys(values map[string]struct{}) []string {
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

func (s *Server) recordGuardrailsHistory(input guardrailscore.Input, result guardrailscore.Result, phase, blockMessage string) {
	if s.guardrailsRuntime == nil || s.guardrailsRuntime.History == nil {
		return
	}

	credentialRefs := guardrailsutils.CollectCredentialRefs(result)
	entry := guardrailsutils.Entry{
		Time:            time.Now(),
		Scenario:        input.Scenario,
		Model:           input.Model,
		Provider:        input.ProviderName(),
		Direction:       string(input.Direction),
		Phase:           phase,
		Verdict:         string(result.Verdict),
		BlockMessage:    blockMessage,
		Preview:         input.Content.LatestPreview(160),
		CredentialRefs:  credentialRefs,
		CredentialNames: s.resolveGuardrailsCredentialNames(credentialRefs),
		Reasons:         append([]guardrailscore.PolicyResult(nil), result.Reasons...),
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	s.guardrailsRuntime.History.Add(entry)
}

// recordGuardrailsMaskHistory stores a dedicated history row for credential
// aliasing events so masking can be audited separately from block/review events.
func (s *Server) recordGuardrailsMaskHistory(c *gin.Context, input guardrailscore.Input, phase string) {
	if s.guardrailsRuntime == nil || s.guardrailsRuntime.History == nil {
		return
	}
	credentialRefs, aliasHits := CollectMaskHistoryCredentialData(c)
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
		CredentialNames: s.resolveGuardrailsCredentialNames(credentialRefs),
		AliasHits:       aliasHits,
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	s.guardrailsRuntime.History.Add(entry)
}

// resolveGuardrailsCredentialNames maps credential ids to stable display names
// for the history API.
func (s *Server) resolveGuardrailsCredentialNames(ids []string) []string {
	return s.getCachedGuardrailsCredentialNames(ids)
}
