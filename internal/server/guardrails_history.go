package server

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type guardrailsHistoryEntry struct {
	Time         time.Time                 `json:"time"`
	Scenario     string                    `json:"scenario"`
	Model        string                    `json:"model"`
	Provider     string                    `json:"provider"`
	Direction    string                    `json:"direction"`
	Phase        string                    `json:"phase"`
	Verdict      string                    `json:"verdict"`
	BlockMessage string                    `json:"block_message,omitempty"`
	Preview      string                    `json:"preview,omitempty"`
	CommandName  string                    `json:"command_name,omitempty"`
	Reasons      []guardrails.PolicyResult `json:"reasons,omitempty"`
}

type guardrailsHistoryStore struct {
	mu         sync.RWMutex
	maxEntries int
	path       string
	entries    []guardrailsHistoryEntry
}

func newGuardrailsHistoryStore(maxEntries int, path string) *guardrailsHistoryStore {
	if maxEntries <= 0 {
		maxEntries = 200
	}

	store := &guardrailsHistoryStore{
		maxEntries: maxEntries,
		path:       path,
	}
	store.load()
	return store
}

func (s *guardrailsHistoryStore) load() {
	if s.path == "" {
		return
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.WithError(err).Warnf("Guardrails history: failed to read %s", s.path)
		}
		return
	}

	var entries []guardrailsHistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logrus.WithError(err).Warnf("Guardrails history: failed to decode %s", s.path)
		return
	}

	if len(entries) > s.maxEntries {
		entries = entries[:s.maxEntries]
	}

	s.entries = entries
}

func (s *guardrailsHistoryStore) Add(entry guardrailsHistoryEntry) {
	s.mu.Lock()
	s.entries = append([]guardrailsHistoryEntry{entry}, s.entries...)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[:s.maxEntries]
	}
	snapshot := append([]guardrailsHistoryEntry(nil), s.entries...)
	s.mu.Unlock()

	s.persist(snapshot)
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
	s.entries = nil
	s.mu.Unlock()

	s.persist([]guardrailsHistoryEntry{})
}

// History writes are intentionally best-effort. Guardrails enforcement should
// keep working even if the local history file cannot be updated.
func (s *guardrailsHistoryStore) persist(entries []guardrailsHistoryEntry) {
	if s.path == "" {
		return
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		logrus.WithError(err).Warn("Guardrails history: failed to encode entries")
		return
	}
	if err := writeFileAtomic(s.path, data); err != nil {
		logrus.WithError(err).Warnf("Guardrails history: failed to persist %s", s.path)
	}
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
		Preview:      input.Content.LatestPreview(160),
		Reasons:      append([]guardrails.PolicyResult(nil), result.Reasons...),
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
