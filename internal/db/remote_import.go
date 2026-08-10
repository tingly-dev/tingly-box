package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// Legacy file names the remote subsystem used to keep beside the database.
const (
	legacyChatsFile    = "bot_chats.json"
	legacySessionsFile = "bot_sessions.json"

	// migratedSuffix marks an imported file. Renaming rather than deleting
	// keeps a rollback path: the data is still on disk if the import turns
	// out to have gone wrong.
	migratedSuffix = ".migrated"
)

// legacyStoreFile mirrors pkg/jsonstore's on-disk envelope. The importer
// parses the files itself rather than going through jsonstore so that the
// package — and the whole JSON-store code path — can be deleted while this
// migration keeps working.
type legacyStoreFile[T any] struct {
	Version int           `json:"version"`
	Items   map[string]*T `json:"items"`
	Updated time.Time     `json:"updated"`
}

// legacySession mirrors the old session.Session encoding. The struct it came
// from had no json tags, so the keys on disk are the Go field names — which is
// exactly why this had to be pinned down before the type could ever be
// renamed. Message likewise.
type legacySession struct {
	ID             string
	ChatID         string
	Agent          string
	Project        string
	Status         string
	Request        string
	Response       string
	Error          string
	CreatedAt      time.Time
	LastActivity   time.Time
	ExpiresAt      time.Time
	Messages       []legacyMessage
	PermissionMode string
}

type legacyMessage struct {
	Role      string
	Content   string
	Summary   string
	Timestamp time.Time
}

// importLegacyRemoteJSON moves bot_chats.json and bot_sessions.json into the
// database, then renames them so the import runs at most once.
//
// It runs from initRemoteStores rather than from any one feature constructor:
// the server, the standalone CLI bot and `remote pair revoke` all open these
// stores, and a migration hooked to only one of them would let a CLI command
// read an empty table while the legacy file still sat next to it.
//
// It is deliberately best-effort per file: a failure importing one file is
// logged and leaves that file in place for a retry on the next start, rather
// than blocking startup. A missing file is the normal case after the first
// run and is not an error.
func importLegacyRemoteJSON(configDir string, chats *RemoteChatStore, sessions *RemoteSessionStore) error {
	if configDir == "" {
		return nil
	}

	var errs []error
	if err := importChats(filepath.Join(configDir, legacyChatsFile), chats); err != nil {
		errs = append(errs, fmt.Errorf("import %s: %w", legacyChatsFile, err))
	}
	if err := importSessions(filepath.Join(configDir, legacySessionsFile), sessions); err != nil {
		errs = append(errs, fmt.Errorf("import %s: %w", legacySessionsFile, err))
	}
	return errors.Join(errs...)
}

// readLegacy loads and decodes a legacy store file. It reports ok=false when
// the file is absent, which is the steady state after migration.
func readLegacy[T any](path string) (items map[string]*T, ok bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read: %w", err)
	}

	var envelope legacyStoreFile[T]
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, false, fmt.Errorf("parse: %w", err)
	}
	return envelope.Items, true, nil
}

// markMigrated renames an imported file out of the way.
func markMigrated(path string) error {
	if err := os.Rename(path, path+migratedSuffix); err != nil {
		return fmt.Errorf("mark migrated: %w", err)
	}
	return nil
}

func importChats(path string, store *RemoteChatStore) error {
	if store == nil {
		return nil
	}
	items, ok, err := readLegacy[bot.Chat](path)
	if err != nil || !ok {
		return err
	}

	imported := 0
	for id, chat := range items {
		if chat == nil {
			continue
		}
		if chat.ChatID == "" {
			chat.ChatID = id
		}
		// Existing rows win: a chat already in the database was written by
		// the current code path and is newer than anything in the file.
		existing, err := store.GetChat(chat.ChatID)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		if err := store.ImportChat(chat); err != nil {
			return fmt.Errorf("upsert chat %s: %w", chat.ChatID, err)
		}
		imported++
	}

	if err := markMigrated(path); err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{"imported": imported, "file": path}).
		Info("Imported legacy chat store into the database")
	return nil
}

func importSessions(path string, store *RemoteSessionStore) error {
	if store == nil {
		return nil
	}
	items, ok, err := readLegacy[legacySession](path)
	if err != nil || !ok {
		return err
	}

	imported := 0
	for id, old := range items {
		if old == nil {
			continue
		}
		if old.ID == "" {
			old.ID = id
		}
		existing, err := store.Get(old.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		sess, msgs := toSession(old)

		// Transcript first, index row last. The row is what marks a session as
		// imported, so writing it only after the history is complete makes a
		// crash mid-import replayable: the next run sees no row, and redoes
		// both halves. The reverse order would leave a session flagged as
		// imported with a truncated transcript, permanently.
		if err := store.transcript.Delete(sess.ID); err != nil {
			return fmt.Errorf("reset transcript for session %s: %w", sess.ID, err)
		}
		for _, m := range msgs {
			if err := store.AppendMessage(sess.ID, m); err != nil {
				return fmt.Errorf("append message for session %s: %w", sess.ID, err)
			}
		}
		if err := store.Import(sess); err != nil {
			return fmt.Errorf("insert session %s: %w", old.ID, err)
		}
		imported++
	}

	if err := markMigrated(path); err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{"imported": imported, "file": path}).
		Info("Imported legacy session store into the database")
	return nil
}

// toSession splits a legacy record into the index part and the transcript
// part, which now live in different places.
func toSession(old *legacySession) (*session.Session, []session.Message) {
	sess := &session.Session{
		ID:             old.ID,
		ChatID:         old.ChatID,
		Agent:          old.Agent,
		Project:        old.Project,
		Status:         session.Status(old.Status),
		Request:        old.Request,
		Response:       old.Response,
		Error:          old.Error,
		CreatedAt:      old.CreatedAt,
		LastActivity:   old.LastActivity,
		ExpiresAt:      old.ExpiresAt,
		PermissionMode: old.PermissionMode,
	}
	msgs := make([]session.Message, 0, len(old.Messages))
	for _, m := range old.Messages {
		msgs = append(msgs, session.Message{
			Role:      m.Role,
			Content:   m.Content,
			Summary:   m.Summary,
			Timestamp: m.Timestamp,
		})
	}
	return sess, msgs
}
