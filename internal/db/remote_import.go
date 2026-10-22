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
)

// Legacy file names the remote subsystem used to keep beside the database.
const (
	legacyChatsFile = "bot_chats.json"

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

// importLegacyRemoteJSON moves bot_chats.json into the database, then renames
// it so the import runs at most once. (bot_sessions.json, the old @cc
// sessions, is left alone: chats talk through Projects now.)
//
// It runs from initRemoteStores rather than from any one feature constructor:
// the server, the standalone CLI bot and `bot pair revoke` all open these
// stores, and a migration hooked to only one of them would let a CLI command
// read an empty table while the legacy file still sat next to it.
//
// It is deliberately best-effort per file: a failure importing one file is
// logged and leaves that file in place for a retry on the next start, rather
// than blocking startup. A missing file is the normal case after the first
// run and is not an error.
func importLegacyRemoteJSON(configDir string, chats *RemoteChatStore) error {
	if configDir == "" {
		return nil
	}

	var errs []error
	if err := importChats(filepath.Join(configDir, legacyChatsFile), chats); err != nil {
		errs = append(errs, fmt.Errorf("import %s: %w", legacyChatsFile, err))
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
