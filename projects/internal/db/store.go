package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotFound is returned when a lookup by ID finds nothing.
var ErrNotFound = errors.New("not found")

// Store wraps the gorm handle with the queries the modules need.
type Store struct {
	db *gorm.DB
}

// Open opens (creating if needed) the SQLite database at path and migrates
// the schema. Pass ":memory:" for an in-memory database in tests.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
		path += "?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL"
	}
	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if path == ":memory:" {
		// Each new connection to :memory: is a fresh database; pin to one.
		sqlDB, err := g.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(1)
	}
	if err := g.AutoMigrate(&Project{}, &ProjectMessage{}, &Thread{}, &ThreadEvent{}, &Conversation{}, &Message{}, &Setting{}, &Workdir{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: g}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func newID() string { return uuid.NewString() }

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func likePattern(q string) string {
	q = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	return "%" + q + "%"
}

// ---- Conversations ----

// ConversationFilter narrows ListConversations.
type ConversationFilter struct {
	Query   string
	Starred bool
	Limit   int
}

// ListConversations returns conversations, most recently updated first. The
// query matches titles and message contents.
func (s *Store) ListConversations(f ConversationFilter) ([]Conversation, error) {
	var convs []Conversation
	tx := s.db.Model(&Conversation{})
	if f.Starred {
		tx = tx.Where("starred = ?", true)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		p := likePattern(q)
		matching := s.db.Model(&Message{}).Select("conversation_id").Where(`content LIKE ? ESCAPE '\'`, p)
		tx = tx.Where(`title LIKE ? ESCAPE '\' OR id IN (?)`, p, matching)
	}
	if f.Limit > 0 {
		tx = tx.Limit(f.Limit)
	}
	err := tx.Order("updated_at DESC").Find(&convs).Error
	return convs, err
}

// CreateConversation inserts a conversation.
func (s *Store) CreateConversation(c *Conversation) error {
	c.ID = newID()
	return s.db.Create(c).Error
}

// GetConversation loads a conversation by ID.
func (s *Store) GetConversation(id string) (*Conversation, error) {
	var c Conversation
	if err := s.db.First(&c, "id = ?", id).Error; err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

// SaveConversation writes all fields of c.
func (s *Store) SaveConversation(c *Conversation) error {
	return s.db.Save(c).Error
}

// DeleteConversation removes a conversation and its messages.
func (s *Store) DeleteConversation(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", id).Delete(&Message{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", id).Delete(&Conversation{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ---- Messages ----

// ListMessages returns a conversation's messages in order.
func (s *Store) ListMessages(conversationID string) ([]Message, error) {
	var msgs []Message
	err := s.db.Where("conversation_id = ?", conversationID).Order("created_at ASC, rowid ASC").Find(&msgs).Error
	return msgs, err
}

// CountMessages returns how many messages a conversation has.
func (s *Store) CountMessages(conversationID string) (int64, error) {
	var n int64
	err := s.db.Model(&Message{}).Where("conversation_id = ?", conversationID).Count(&n).Error
	return n, err
}

// CreateMessage inserts a message and bumps its conversation's UpdatedAt.
func (s *Store) CreateMessage(m *Message) error {
	m.ID = newID()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		return tx.Model(&Conversation{}).Where("id = ?", m.ConversationID).Update("updated_at", time.Now()).Error
	})
}

// DeleteMessages removes the given messages.
func (s *Store) DeleteMessages(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.db.Where("id IN ?", ids).Delete(&Message{}).Error
}

// ---- Settings ----

// GetSetting decodes the JSON value stored under key into out. It reports
// false when the key has never been written.
func (s *Store) GetSetting(key string, out any) (bool, error) {
	var row Setting
	if err := s.db.First(&row, "key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, json.Unmarshal([]byte(row.Value), out)
}

// PutSetting stores v JSON-encoded under key.
func (s *Store) PutSetting(key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.db.Save(&Setting{Key: key, Value: string(b), UpdatedAt: time.Now()}).Error
}

// SetConversationTitle renames a conversation without bumping UpdatedAt.
func (s *Store) SetConversationTitle(id, title string) error {
	return s.db.Model(&Conversation{}).Where("id = ?", id).UpdateColumn("title", title).Error
}

// SetConversationModel records the model a conversation last used.
func (s *Store) SetConversationModel(id, model string) error {
	return s.db.Model(&Conversation{}).Where("id = ?", id).UpdateColumn("model", model).Error
}

// SaveConversationMeta writes a conversation's editable fields without
// bumping UpdatedAt, so renaming or starring keeps its place in "recent".
func (s *Store) SaveConversationMeta(c *Conversation) error {
	return s.db.Model(&Conversation{}).Where("id = ?", c.ID).UpdateColumns(map[string]any{
		"title":   c.Title,
		"starred": c.Starred,
		"model":   c.Model,
	}).Error
}
