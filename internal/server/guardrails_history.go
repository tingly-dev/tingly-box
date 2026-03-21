package server

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type guardrailsHistoryEntry struct {
	Time            time.Time                 `json:"time"`
	Scenario        string                    `json:"scenario"`
	Model           string                    `json:"model"`
	Provider        string                    `json:"provider"`
	Direction       string                    `json:"direction"`
	Phase           string                    `json:"phase"`
	Verdict         string                    `json:"verdict"`
	BlockMessage    string                    `json:"block_message,omitempty"`
	Preview         string                    `json:"preview,omitempty"`
	CommandName     string                    `json:"command_name,omitempty"`
	CredentialRefs  []string                  `json:"credential_refs,omitempty"`
	CredentialNames []string                  `json:"credential_names,omitempty"`
	AliasHits       []string                  `json:"alias_hits,omitempty"`
	Reasons         []guardrails.PolicyResult `json:"reasons,omitempty"`
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
	if len(s.entries) > 0 && sameGuardrailsHistoryEntry(s.entries[0], entry) {
		s.mu.Unlock()
		return
	}
	s.entries = append([]guardrailsHistoryEntry{entry}, s.entries...)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[:s.maxEntries]
	}
	snapshot := append([]guardrailsHistoryEntry(nil), s.entries...)
	s.mu.Unlock()

	s.persist(snapshot)
}

func sameGuardrailsHistoryEntry(a, b guardrailsHistoryEntry) bool {
	return a.Scenario == b.Scenario &&
		a.Model == b.Model &&
		a.Provider == b.Provider &&
		a.Direction == b.Direction &&
		a.Phase == b.Phase &&
		a.Verdict == b.Verdict &&
		a.BlockMessage == b.BlockMessage &&
		a.Preview == b.Preview &&
		a.CommandName == b.CommandName &&
		reflect.DeepEqual(a.CredentialRefs, b.CredentialRefs) &&
		reflect.DeepEqual(a.CredentialNames, b.CredentialNames) &&
		reflect.DeepEqual(a.AliasHits, b.AliasHits) &&
		reflect.DeepEqual(a.Reasons, b.Reasons)
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

func (s *Server) recordGuardrailsHistory(c *gin.Context, session guardrailsSession, input guardrails.Input, result guardrails.Result, phase, blockMessage string) {
	if s.guardrailsHistory == nil {
		return
	}

	credentialRefs, aliasHits := collectGuardrailsHistoryCredentialData(c, result)
	entry := guardrailsHistoryEntry{
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
		AliasHits:       aliasHits,
		Reasons:         append([]guardrails.PolicyResult(nil), result.Reasons...),
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	s.guardrailsHistory.Add(entry)
}

func (s *Server) recordGuardrailsMaskHistory(c *gin.Context, session guardrailsSession, input guardrails.Input, phase string) {
	if s.guardrailsHistory == nil {
		return
	}
	credentialRefs, aliasHits := collectGuardrailsHistoryCredentialData(c, guardrails.Result{})
	if len(credentialRefs) == 0 && len(aliasHits) == 0 {
		return
	}
	entry := guardrailsHistoryEntry{
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
	s.guardrailsHistory.Add(entry)
}

func collectGuardrailsHistoryCredentialData(c *gin.Context, result guardrails.Result) ([]string, []string) {
	refSet := make(map[string]struct{})
	aliasSet := make(map[string]struct{})

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

	if c != nil {
		if existing, ok := c.Get(guardrails.CredentialMaskStateContextKey); ok {
			if state, ok := existing.(*guardrails.CredentialMaskState); ok && state != nil {
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
			}
		}
	}

	refs := sortedKeys(refSet)
	aliases := sortedKeys(aliasSet)
	return refs, aliases
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

func (s *Server) resolveGuardrailsCredentialNames(ids []string) []string {
	return s.getCachedGuardrailsCredentialNames(ids)
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
