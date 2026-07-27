package smart_guide

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/pkg/fs"
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
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	logrus.WithField("dataDir", dataDir).Info("Created SmartGuide session store (anthropic-native)")
	return &SessionStore{dir: dataDir}, nil
}

// path returns the on-disk file for a chat's history.
//
// The chat ID goes through fs.SafeFileKey because it comes straight from the IM
// platform and is not filename-safe: Feishu ids carry punctuation and WhatsApp
// JIDs contain '@' and '/'.
func (s *SessionStore) path(chatID string) string {
	return filepath.Join(s.dir, fs.SafeFileKey(chatID)+"-smartguide.json")
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
	// 0600: this file holds the user's full conversation with the model, and
	// the rest of the remote state files are already owner-only.
	p := s.path(chatID)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return err
	}
	// WriteFile only applies the mode when creating, so tighten files that
	// were written as 0644 by earlier versions.
	if err := os.Chmod(p, 0o600); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).Debug("Failed to tighten SmartGuide session file mode")
	}
	logrus.WithFields(logrus.Fields{"chatID": chatID, "msgCount": len(messages)}).Debug("Saved SmartGuide session")
	return nil
}

// Clear ends a chat's current Smart Guide session: the live history file is
// archived (renamed with a timestamp suffix) rather than deleted, so /clear
// deactivates the conversation instead of destroying it — the same "closed,
// not erased" semantics remote/session.Manager.Close gives @cc sessions. The
// next Load for chatID sees no file and starts fresh; the archived file is
// left on disk as that session's log.
func (s *SessionStore) Clear(chatID string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Rename(s.path(chatID), s.archivePath(chatID)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	logrus.WithField("chatID", chatID).Debug("Archived SmartGuide session")
	return nil
}

// archivePath returns a unique on-disk location for an archived (cleared)
// history file, distinct from path()'s canonical live-session location.
func (s *SessionStore) archivePath(chatID string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s-smartguide.%d.json", fs.SafeFileKey(chatID), time.Now().UnixNano()))
}
