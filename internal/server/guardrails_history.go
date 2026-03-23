package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	serverguardrails "github.com/tingly-dev/tingly-box/internal/server/guardrails"
)

func (s *Server) recordGuardrailsHistory(c *gin.Context, session guardrailsSession, input guardrails.Input, result guardrails.Result, phase, blockMessage string) {
	if s.guardrailsHistory == nil {
		return
	}

	credentialRefs := serverguardrails.CollectHistoryCredentialRefs(result)
	entry := serverguardrails.HistoryEntry{
		Time:            time.Now(),
		Scenario:        session.Scenario,
		Model:           session.Model,
		Provider:        session.ProviderName,
		Direction:       string(input.Direction),
		Phase:           phase,
		Verdict:         string(result.Verdict),
		BlockMessage:    blockMessage,
		Preview:         input.Content.LatestPreview(160),
		CredentialRefs:  credentialRefs,
		CredentialNames: s.resolveGuardrailsCredentialNames(credentialRefs),
		Reasons:         append([]guardrails.PolicyResult(nil), result.Reasons...),
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	s.guardrailsHistory.Add(entry, writeFileAtomic)
}

func (s *Server) recordGuardrailsMaskHistory(c *gin.Context, session guardrailsSession, input guardrails.Input, phase string) {
	if s.guardrailsHistory == nil {
		return
	}
	credentialRefs, aliasHits := serverguardrails.CollectMaskHistoryCredentialData(c)
	if len(credentialRefs) == 0 && len(aliasHits) == 0 {
		return
	}
	entry := serverguardrails.HistoryEntry{
		Time:            time.Now(),
		Scenario:        session.Scenario,
		Model:           session.Model,
		Provider:        session.ProviderName,
		Direction:       string(input.Direction),
		Phase:           phase,
		Verdict:         string(guardrails.VerdictMask),
		Preview:         input.Content.LatestPreview(160),
		CredentialRefs:  credentialRefs,
		CredentialNames: s.resolveGuardrailsCredentialNames(credentialRefs),
		AliasHits:       aliasHits,
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	s.guardrailsHistory.Add(entry, writeFileAtomic)
}

func (s *Server) resolveGuardrailsCredentialNames(ids []string) []string {
	return s.getCachedGuardrailsCredentialNames(ids)
}

func (s *Server) GetGuardrailsHistory(c *gin.Context) {
	if s.guardrailsHistory == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    []serverguardrails.HistoryEntry{},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    s.guardrailsHistory.List(200),
	})
}

func (s *Server) ClearGuardrailsHistory(c *gin.Context) {
	if s.guardrailsHistory != nil {
		s.guardrailsHistory.Clear(writeFileAtomic)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}
