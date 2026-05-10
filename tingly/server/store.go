// Package server implements the tingly platform: a small WebSocket router
// that connects bots and chat clients, persists message history per (bot,
// chat) pair, and forwards events between them.
//
// The server is intentionally minimal. It reuses pkg/jsonstore for
// persistence and gorilla/websocket for transport, and depends on the imbot
// wire package for protocol types.
package server

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/wire"
	"github.com/tingly-dev/tingly-box/pkg/jsonstore"
)

// chatLog is the persisted record for a single (bot, chat) conversation.
// Entries are stored in chronological order; edits/deletes/reactions append
// new entries rather than mutating prior ones, preserving an audit trail.
type chatLog struct {
	BotID   string              `json:"botId"`
	ChatID  string              `json:"chatId"`
	Entries []wire.HistoryEntry `json:"entries"`
}

// Store is the persistence layer for tingly conversations. Backed by a
// single jsonstore keyed by "{botID}/{chatID}", it is safe for concurrent
// callers.
type Store struct {
	js     *jsonstore.Store[chatLog]
	mu     sync.Mutex // serializes read-modify-write on a key
	idSeq  atomic.Int64
}

// NewStore opens (or creates) the on-disk store at filePath. The directory
// is created if missing. Tests typically pass a t.TempDir()-derived path.
func NewStore(filePath string) (*Store, error) {
	if filePath == "" {
		return nil, fmt.Errorf("tingly store: filePath is required")
	}
	js, err := jsonstore.New[chatLog](filePath)
	if err != nil {
		return nil, fmt.Errorf("tingly store: %w", err)
	}
	return &Store{js: js}, nil
}

// Close flushes and releases the store.
func (s *Store) Close() error {
	if s == nil || s.js == nil {
		return nil
	}
	return s.js.Close()
}

// nextMessageID mints a globally unique synthetic message id.
func (s *Store) nextMessageID() string {
	n := s.idSeq.Add(1)
	return fmt.Sprintf("ty-%d-%d", time.Now().Unix(), n)
}

// key builds the jsonstore key for a (bot, chat) pair.
func storeKey(botID, chatID string) string {
	return botID + "/" + chatID
}

// Append records a frame into the chat log and persists synchronously. The
// caller is responsible for having already populated f.Bot, f.Chat, f.ID,
// and f.Data.
func (s *Store) Append(f wire.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(f.Bot, f.Chat)
	cl := s.js.Get(key)
	if cl == nil {
		cl = &chatLog{BotID: f.Bot, ChatID: f.Chat}
	}
	cl.Entries = append(cl.Entries, wire.HistoryEntry{Frame: f})
	if err := s.js.Set(key, cl); err != nil {
		return err
	}
	return s.js.ForceSave()
}

// History returns up to limit recent history entries for (bot, chat). When
// limit <= 0, all entries are returned.
func (s *Store) History(botID, chatID string, limit int) []wire.HistoryEntry {
	cl := s.js.Get(storeKey(botID, chatID))
	if cl == nil || len(cl.Entries) == 0 {
		return nil
	}
	if limit <= 0 || limit >= len(cl.Entries) {
		out := make([]wire.HistoryEntry, len(cl.Entries))
		copy(out, cl.Entries)
		return out
	}
	out := make([]wire.HistoryEntry, limit)
	copy(out, cl.Entries[len(cl.Entries)-limit:])
	return out
}

// AllChats returns all (bot, chat) keys known to the store.
func (s *Store) AllChats() []string {
	return s.js.Keys()
}

