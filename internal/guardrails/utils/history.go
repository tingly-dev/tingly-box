package utils

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

type Entry struct {
	Time            time.Time                     `json:"time"`
	Scenario        string                        `json:"scenario"`
	Model           string                        `json:"model"`
	Provider        string                        `json:"provider"`
	Direction       string                        `json:"direction"`
	Phase           string                        `json:"phase"`
	Verdict         string                        `json:"verdict"`
	BlockMessage    string                        `json:"block_message,omitempty"`
	Preview         string                        `json:"preview,omitempty"`
	CommandName     string                        `json:"command_name,omitempty"`
	CredentialRefs  []string                      `json:"credential_refs,omitempty"`
	CredentialNames []string                      `json:"credential_names,omitempty"`
	AliasHits       []string                      `json:"alias_hits,omitempty"`
	Reasons         []guardrailscore.PolicyResult `json:"reasons,omitempty"`
}

type Store struct {
	mu         sync.RWMutex
	maxEntries int
	path       string
	entries    []Entry
}

func NewStore(maxEntries int, path string) *Store {
	if maxEntries <= 0 {
		maxEntries = 200
	}

	store := &Store{
		maxEntries: maxEntries,
		path:       path,
	}
	store.load()
	return store
}

func (s *Store) load() {
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

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		logrus.WithError(err).Warnf("Guardrails history: failed to decode %s", s.path)
		return
	}

	if len(entries) > s.maxEntries {
		entries = entries[:s.maxEntries]
	}

	s.entries = entries
}

func (s *Store) Add(entry Entry, persist func(path string, data []byte) error) {
	s.mu.Lock()
	if len(s.entries) > 0 && sameEntry(s.entries[0], entry) {
		s.mu.Unlock()
		return
	}
	s.entries = append([]Entry{entry}, s.entries...)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[:s.maxEntries]
	}
	snapshot := append([]Entry(nil), s.entries...)
	s.mu.Unlock()

	s.persist(snapshot, persist)
}

func (s *Store) List(limit int) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.entries) {
		limit = len(s.entries)
	}
	out := make([]Entry, limit)
	copy(out, s.entries[:limit])
	return out
}

func (s *Store) Clear(persist func(path string, data []byte) error) {
	s.mu.Lock()
	s.entries = nil
	s.mu.Unlock()

	s.persist([]Entry{}, persist)
}

func (s *Store) persist(entries []Entry, persist func(path string, data []byte) error) {
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

func CollectCredentialRefs(result guardrailscore.Result) []string {
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

func sameEntry(a, b Entry) bool {
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
