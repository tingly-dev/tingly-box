package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type guardrailsHistoryEntry struct {
	Time           time.Time                `json:"time"`
	Scenario       string                   `json:"scenario"`
	Model          string                   `json:"model"`
	Provider       string                   `json:"provider"`
	Direction      string                   `json:"direction"`
	Phase          string                   `json:"phase"`
	Verdict        string                   `json:"verdict"`
	BlockMessage   string                   `json:"block_message,omitempty"`
	Preview        string                   `json:"preview,omitempty"`
	CommandName    string                   `json:"command_name,omitempty"`
	Reasons        []guardrails.RuleResult  `json:"reasons,omitempty"`
}

type guardrailsHistoryStore struct {
	mu         sync.RWMutex
	maxEntries int
	entries    []guardrailsHistoryEntry
}

func newGuardrailsHistoryStore(maxEntries int) *guardrailsHistoryStore {
	if maxEntries <= 0 {
		maxEntries = 200
	}
	return &guardrailsHistoryStore{maxEntries: maxEntries}
}

func (s *guardrailsHistoryStore) Add(entry guardrailsHistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append([]guardrailsHistoryEntry{entry}, s.entries...)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[:s.maxEntries]
	}
}

func (s *guardrailsHistoryStore) List(limit int) []guardrailsHistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.entries) {
		limit = len(s.entries)
	}
	out := make([]guardrailsHistoryEntry, limit)
	copy(out, s.entries[:limit])
	return out
}

func (s *guardrailsHistoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
}

func (s *Server) recordGuardrailsHistory(session guardrailsSession, input guardrails.Input, result guardrails.Result, phase, blockMessage string) {
	if s.guardrailsHistory == nil {
		return
	}

	entry := guardrailsHistoryEntry{
		Time:         time.Now(),
		Scenario:     session.Scenario,
		Model:        session.Model,
		Provider:     session.ProviderName,
		Direction:    string(input.Direction),
		Phase:        phase,
		Verdict:      string(result.Verdict),
		BlockMessage: blockMessage,
		Preview:      input.Content.Preview(160),
		Reasons:      append([]guardrails.RuleResult(nil), result.Reasons...),
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	s.guardrailsHistory.Add(entry)
}

func (s *Server) GetGuardrailsHistory(c *gin.Context) {
	if s.guardrailsHistory == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    []guardrailsHistoryEntry{},
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
		s.guardrailsHistory.Clear()
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}
