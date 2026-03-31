package serverguardrails

import (
	"encoding/json"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type HistoryEntry struct {
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

type HistoryStore struct {
	mu         sync.RWMutex
	maxEntries int
	path       string
	entries    []HistoryEntry
}

func NewHistoryStore(maxEntries int, path string) *HistoryStore {
	if maxEntries <= 0 {
		maxEntries = 200
	}

	store := &HistoryStore{
		maxEntries: maxEntries,
		path:       path,
	}
	store.load()
	return store
}

func (s *HistoryStore) load() {
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

	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logrus.WithError(err).Warnf("Guardrails history: failed to decode %s", s.path)
		return
	}

	if len(entries) > s.maxEntries {
		entries = entries[:s.maxEntries]
	}

	s.entries = entries
}

func (s *HistoryStore) Add(entry HistoryEntry, persist func(path string, data []byte) error) {
	s.mu.Lock()
	if len(s.entries) > 0 && sameHistoryEntry(s.entries[0], entry) {
		s.mu.Unlock()
		return
	}
	s.entries = append([]HistoryEntry{entry}, s.entries...)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[:s.maxEntries]
	}
	snapshot := append([]HistoryEntry(nil), s.entries...)
	s.mu.Unlock()

	s.persist(snapshot, persist)
}

func (s *HistoryStore) List(limit int) []HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.entries) {
		limit = len(s.entries)
	}
	out := make([]HistoryEntry, limit)
	copy(out, s.entries[:limit])
	return out
}

func (s *HistoryStore) Clear(persist func(path string, data []byte) error) {
	s.mu.Lock()
	s.entries = nil
	s.mu.Unlock()

	s.persist([]HistoryEntry{}, persist)
}

func (s *HistoryStore) persist(entries []HistoryEntry, persist func(path string, data []byte) error) {
	if s.path == "" {
		return
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		logrus.WithError(err).Warn("Guardrails history: failed to encode entries")
		return
	}
	if err := persist(s.path, data); err != nil {
		logrus.WithError(err).Warnf("Guardrails history: failed to persist %s", s.path)
	}
}

func sameHistoryEntry(a, b HistoryEntry) bool {
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
