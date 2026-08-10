package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tingly-dev/tingly-box/remote/session"
)

// Compile-time proof that this store can back the remote session manager.
var _ session.SessionStore = (*RemoteSessionStore)(nil)

// RemoteSessionStore persists remote-control sessions across two media,
// chosen per access pattern rather than by preference:
//
//   - the session INDEX (binding, status, timestamps) goes in the shared
//     SQLite database. Not for lookup speed — the manager keeps live sessions
//     in memory and scans that first — but because it is small mutable state
//     that several processes write concurrently: a session's status turns over
//     repeatedly, and the CLI and the server must not overwrite each other the
//     way two holders of a JSON file did;
//   - the message TRANSCRIPT goes in an append-only file per session, because
//     it is write-once, read whole, never queried by content, and unbounded.
//     See session.Transcript for the full argument.
type RemoteSessionStore struct {
	db         *gorm.DB
	transcript *session.Transcript
}

// NewRemoteSessionStore builds a store over an existing DB handle plus a
// transcript directory. The handle is owned by the StoreManager; Close here
// does not close it. A nil transcript simply drops history.
func NewRemoteSessionStore(db *gorm.DB, transcript *session.Transcript) *RemoteSessionStore {
	return &RemoteSessionStore{db: db, transcript: transcript}
}

func (s *RemoteSessionStore) ready() bool { return s != nil && s.db != nil }

// ---------- mapping ----------

func toSessionRecord(sess *session.Session) *RemoteSessionRecord {
	return &RemoteSessionRecord{
		ID:             sess.ID,
		ChatID:         sess.ChatID,
		Agent:          sess.Agent,
		Project:        sess.Project,
		Status:         string(sess.Status),
		Request:        sess.Request,
		Response:       sess.Response,
		Error:          sess.Error,
		PermissionMode: sess.PermissionMode,
		CreatedAt:      sess.CreatedAt,
		LastActivity:   sess.LastActivity,
		ExpiresAt:      sess.ExpiresAt,
	}
}

func fromSessionRecord(rec *RemoteSessionRecord) *session.Session {
	return &session.Session{
		ID:             rec.ID,
		ChatID:         rec.ChatID,
		Agent:          rec.Agent,
		Project:        rec.Project,
		Status:         session.Status(rec.Status),
		Request:        rec.Request,
		Response:       rec.Response,
		Error:          rec.Error,
		PermissionMode: rec.PermissionMode,
		CreatedAt:      rec.CreatedAt,
		LastActivity:   rec.LastActivity,
		ExpiresAt:      rec.ExpiresAt,
	}
}

// ---------- reads ----------

// Get retrieves a session's index record by ID. Messages are not loaded —
// call Messages when the transcript is actually needed. A missing session is
// (nil, nil), matching the store it replaces.
func (s *RemoteSessionStore) Get(sessionID string) (*session.Session, error) {
	if !s.ready() || sessionID == "" {
		return nil, nil
	}

	var rec RemoteSessionRecord
	if err := s.db.Where("id = ?", sessionID).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get session %s: %w", sessionID, err)
	}

	return fromSessionRecord(&rec), nil
}

// List returns the session index records worth warming the manager's in-memory
// map with. Two bounds matter here, both because this runs synchronously at
// startup: it must not read transcripts (that would pull every conversation
// ever held into memory), and it skips terminal sessions, which accumulate
// forever — Manager.Close leaves closed rows behind on purpose.
//
// Skipping them changes nothing observable: Manager.FindBy ignores closed and
// expired sessions anyway, and GetOrLoad still fetches any session by id.
func (s *RemoteSessionStore) List() []*session.Session {
	if !s.ready() {
		return []*session.Session{}
	}

	var recs []RemoteSessionRecord
	if err := s.db.
		Where("status NOT IN ?", []string{string(session.StatusClosed), string(session.StatusExpired)}).
		Find(&recs).Error; err != nil {
		logrus.WithError(err).Error("remote session: list failed")
		return []*session.Session{}
	}
	return toSessions(recs)
}

// FindByChatAgentProject returns the most recently active non-terminal session
// bound to the tuple. Manager.FindBy consults its in-memory map first, so this
// is the cold path: a session written by another process, or one the manager
// has not loaded since restart.
func (s *RemoteSessionStore) FindByChatAgentProject(chatID, agent, project string) (*session.Session, error) {
	if !s.ready() {
		return nil, nil
	}

	var rec RemoteSessionRecord
	err := s.db.
		Where("chat_id = ? AND agent = ? AND project = ?", chatID, agent, project).
		Where("status NOT IN ?", []string{string(session.StatusClosed), string(session.StatusExpired)}).
		Order("last_activity DESC").
		First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find session by binding: %w", err)
	}

	return fromSessionRecord(&rec), nil
}

// ListByChat returns all sessions for a chat, newest activity first.
func (s *RemoteSessionStore) ListByChat(chatID string) ([]*session.Session, error) {
	if !s.ready() {
		return nil, nil
	}

	var recs []RemoteSessionRecord
	if err := s.db.Where("chat_id = ?", chatID).Order("last_activity DESC").Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("list sessions by chat: %w", err)
	}
	return toSessions(recs), nil
}

// toSessions maps a batch of index records.
func toSessions(recs []RemoteSessionRecord) []*session.Session {
	out := make([]*session.Session, 0, len(recs))
	for i := range recs {
		out = append(out, fromSessionRecord(&recs[i]))
	}
	return out
}

// ---------- transcript ----------

// AppendMessage adds one message to a session's transcript file.
func (s *RemoteSessionStore) AppendMessage(sessionID string, msg session.Message) error {
	if s == nil {
		return nil
	}
	return s.transcript.Append(sessionID, msg)
}

// Messages reads a session's transcript on demand.
func (s *RemoteSessionStore) Messages(sessionID string) ([]session.Message, error) {
	if s == nil {
		return nil, nil
	}
	return s.transcript.Load(sessionID)
}

// ---------- writes ----------

// Set upserts a session's index record.
//
// This writes one row. It used to also reconcile the message rows — counting
// what was stored, appending only the new tail, trimming a shortened history —
// machinery that existed purely because the transcript was a table. With
// messages in an append-only file, AppendMessage stands on its own and this
// stays a single upsert.
func (s *RemoteSessionStore) Set(sessionID string, sess *session.Session) error {
	if sess != nil && sessionID != "" {
		sess.ID = sessionID
	}
	return s.write(sess, true)
}

// Import stores a session exactly as given, without stamping LastActivity.
//
// Migration needs this: LastActivity orders FindByChatAgentProject and drives
// retention, so touching it on import would make every migrated session look
// like it was just active and reorder conversations that had been dormant
// for weeks.
func (s *RemoteSessionStore) Import(sess *session.Session) error {
	if sess == nil {
		return nil
	}
	return s.write(sess, false)
}

func (s *RemoteSessionStore) write(sess *session.Session, touch bool) error {
	if !s.ready() || sess == nil {
		return nil
	}
	if sess.ID == "" {
		return fmt.Errorf("session id is required")
	}

	if touch {
		sess.LastActivity = time.Now().UTC()
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(toSessionRecord(sess)).Error; err != nil {
		return fmt.Errorf("upsert session %s: %w", sess.ID, err)
	}
	return nil
}

// Delete removes a session's index record and its transcript.
//
// The row goes first: an orphaned transcript file is inert (nothing can reach
// it without an index entry) whereas an index entry whose transcript is gone
// would surface in listings as a session whose history silently vanished.
func (s *RemoteSessionStore) Delete(sessionID string) error {
	if !s.ready() || sessionID == "" {
		return nil
	}
	if err := s.db.Where("id = ?", sessionID).
		Delete(&RemoteSessionRecord{}).Error; err != nil {
		return fmt.Errorf("delete session %s: %w", sessionID, err)
	}
	if err := s.transcript.Delete(sessionID); err != nil {
		return fmt.Errorf("delete transcript %s: %w", sessionID, err)
	}
	return nil
}
