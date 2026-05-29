package smart_guide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

// SessionStore persists Smart Guide conversation history as native Anthropic
// message params, one JSON file per chat. We are anthropic-first, so there is
// no neutral message type: the stored shape is exactly what the model API
// consumes, which round-trips losslessly through encoding/json.
type SessionStore struct {
	dir string
	mu  sync.Mutex
}

// NewSessionStore creates a session store rooted at dataDir. A blank dataDir
// disables persistence (returns nil, nil), mirroring the previous behavior.
func NewSessionStore(dataDir string) (*SessionStore, error) {
	if dataDir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	logrus.WithField("dataDir", dataDir).Info("Created SmartGuide session store (anthropic-native)")
	return &SessionStore{dir: dataDir}, nil
}

// path returns the on-disk file for a chat's history.
func (s *SessionStore) path(chatID string) string {
	return filepath.Join(s.dir, chatID+"-smartguide.json")
}

// Load returns the stored history for a chat, or an empty slice if none exists.
// A corrupt or unreadable file is treated as empty (logged, not fatal) so a
// single bad session never blocks the user.
func (s *SessionStore) Load(chatID string) ([]anthropic.BetaMessageParam, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path(chatID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		logrus.WithError(err).WithField("chatID", chatID).Debug("SmartGuide session read failed, treating as empty")
		return nil, nil
	}

	var msgs []anthropic.BetaMessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Warn("SmartGuide session deserialize failed, treating as empty")
		return nil, nil
	}
	return msgs, nil
}

// Save overwrites the stored history for a chat.
func (s *SessionStore) Save(chatID string, messages []anthropic.BetaMessageParam) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path(chatID), data, 0o644); err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{"chatID": chatID, "msgCount": len(messages)}).Debug("Saved SmartGuide session")
	return nil
}

// Delete removes a chat's stored history.
func (s *SessionStore) Delete(chatID string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path(chatID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
