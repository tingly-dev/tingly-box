package db

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/remote/peer"
)

// peerRecord persists one Peer (see remote/peer and .design/peer.md). Its
// own small table on purpose — not a Scenarios row and not a BotCapability
// merge.
type peerRecord struct {
	ID      uint   `gorm:"primaryKey;autoIncrement;column:id"`
	UUID    string `gorm:"uniqueIndex;column:uuid;not null;size:64"`
	Name    string `gorm:"uniqueIndex;column:name;not null;size:64"`
	BotUUID string `gorm:"index:idx_peers_bot;column:bot_uuid;not null;size:64"`
	ChatID  string `gorm:"column:chat_id;not null;size:256"`
	// No gorm defaults on the booleans: a default tag makes GORM skip
	// zero-valued fields on insert, silently turning enabled=false into true.
	Exclusive     bool      `gorm:"column:exclusive"`
	Enabled       bool      `gorm:"column:enabled"`
	TokenHash     string    `gorm:"index:idx_peers_token;column:token_hash;size:64"`
	AckedUpdateID int64     `gorm:"column:acked_update_id;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (peerRecord) TableName() string { return "peers" }

// peerUpdateRecord persists one inbound update. The autoincrement id doubles
// as the delivery cursor.
type peerUpdateRecord struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id"`
	PeerUUID     string    `gorm:"index:idx_peer_updates_peer;column:peer_uuid;not null;size:64"`
	BotUUID      string    `gorm:"column:bot_uuid;size:64"`
	Type         string    `gorm:"column:type;size:32"`
	ChatID       string    `gorm:"column:chat_id;size:256"`
	SenderID     string    `gorm:"column:sender_id;size:256"`
	MessageID    string    `gorm:"column:message_id;size:256"`
	Text         string    `gorm:"column:text"`
	ContextToken string    `gorm:"column:context_token;size:512"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (peerUpdateRecord) TableName() string { return "peer_updates" }

// PeerStore is the SQLite-backed peer.Store implementation.
type PeerStore struct {
	db *gorm.DB
}

// NewPeerStore builds the store over the shared DB handle.
func NewPeerStore(db *gorm.DB) *PeerStore {
	return &PeerStore{db: db}
}

func recordFromPeer(p peer.Peer) peerRecord {
	return peerRecord{
		UUID:          p.UUID,
		Name:          p.Name,
		BotUUID:       p.BotUUID,
		ChatID:        p.ChatID,
		Exclusive:     p.Exclusive,
		Enabled:       p.Enabled,
		TokenHash:     p.TokenHash,
		AckedUpdateID: p.AckedUpdateID,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

func (r peerRecord) toPeer() peer.Peer {
	return peer.Peer{
		UUID:          r.UUID,
		Name:          r.Name,
		BotUUID:       r.BotUUID,
		ChatID:        r.ChatID,
		Exclusive:     r.Exclusive,
		Enabled:       r.Enabled,
		TokenHash:     r.TokenHash,
		AckedUpdateID: r.AckedUpdateID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func (r peerUpdateRecord) toUpdate() peer.Update {
	return peer.Update{
		ID:           r.ID,
		PeerUUID:     r.PeerUUID,
		BotUUID:      r.BotUUID,
		Type:         r.Type,
		ChatID:       r.ChatID,
		SenderID:     r.SenderID,
		MessageID:    r.MessageID,
		Text:         r.Text,
		ContextToken: r.ContextToken,
		CreatedAt:    r.CreatedAt,
	}
}

func (s *PeerStore) Create(p *peer.Peer) error {
	if p.UUID == "" {
		p.UUID = uuid.NewString()
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	var count int64
	if err := s.db.Model(&peerRecord{}).Where("name = ?", p.Name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return peer.ErrNameTaken
	}
	rec := recordFromPeer(*p)
	return s.db.Create(&rec).Error
}

func (s *PeerStore) Get(peerUUID string) (peer.Peer, error) {
	var rec peerRecord
	err := s.db.Where("uuid = ?", peerUUID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return peer.Peer{}, peer.ErrNotFound
	}
	if err != nil {
		return peer.Peer{}, err
	}
	return rec.toPeer(), nil
}

func (s *PeerStore) GetByToken(tokenHash string) (peer.Peer, error) {
	if tokenHash == "" {
		return peer.Peer{}, peer.ErrNotFound
	}
	var rec peerRecord
	err := s.db.Where("token_hash = ?", tokenHash).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return peer.Peer{}, peer.ErrNotFound
	}
	if err != nil {
		return peer.Peer{}, err
	}
	return rec.toPeer(), nil
}

func (s *PeerStore) List() ([]peer.Peer, error) {
	var recs []peerRecord
	if err := s.db.Order("created_at DESC").Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]peer.Peer, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.toPeer())
	}
	return out, nil
}

func (s *PeerStore) ListByBot(botUUID string) ([]peer.Peer, error) {
	var recs []peerRecord
	if err := s.db.Where("bot_uuid = ?", botUUID).Order("created_at DESC").Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]peer.Peer, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.toPeer())
	}
	return out, nil
}

func (s *PeerStore) HasEnabledForBot(botUUID string) bool {
	var count int64
	if err := s.db.Model(&peerRecord{}).
		Where("bot_uuid = ? AND enabled = ?", botUUID, true).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (s *PeerStore) Save(p *peer.Peer) error {
	var count int64
	if err := s.db.Model(&peerRecord{}).
		Where("name = ? AND uuid <> ?", p.Name, p.UUID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return peer.ErrNameTaken
	}
	p.UpdatedAt = time.Now()
	res := s.db.Model(&peerRecord{}).Where("uuid = ?", p.UUID).Updates(map[string]any{
		"name":            p.Name,
		"bot_uuid":        p.BotUUID,
		"chat_id":         p.ChatID,
		"exclusive":       p.Exclusive,
		"enabled":         p.Enabled,
		"token_hash":      p.TokenHash,
		"acked_update_id": p.AckedUpdateID,
		"updated_at":      p.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return peer.ErrNotFound
	}
	return nil
}

func (s *PeerStore) Delete(peerUUID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("peer_uuid = ?", peerUUID).Delete(&peerUpdateRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", peerUUID).Delete(&peerRecord{}).Error
	})
}

func (s *PeerStore) AppendUpdate(u *peer.Update) (int, error) {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	rec := peerUpdateRecord{
		PeerUUID:     u.PeerUUID,
		BotUUID:      u.BotUUID,
		Type:         u.Type,
		ChatID:       u.ChatID,
		SenderID:     u.SenderID,
		MessageID:    u.MessageID,
		Text:         u.Text,
		ContextToken: u.ContextToken,
		CreatedAt:    u.CreatedAt,
	}
	dropped := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&peerUpdateRecord{}).
			Where("peer_uuid = ?", u.PeerUUID).
			Count(&count).Error; err != nil {
			return err
		}
		if over := count - peer.MaxQueuedUpdates; over > 0 {
			// Drop oldest-first down to the cap; the caller logs the drop.
			res := tx.Where("id IN (?)", tx.Model(&peerUpdateRecord{}).
				Select("id").
				Where("peer_uuid = ?", u.PeerUUID).
				Order("id ASC").
				Limit(int(over)),
			).Delete(&peerUpdateRecord{})
			if res.Error != nil {
				return res.Error
			}
			dropped = int(res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	u.ID = rec.ID
	return dropped, nil
}

func (s *PeerStore) UpdatesAfter(peerUUID string, afterID int64, limit int) ([]peer.Update, error) {
	if limit <= 0 {
		limit = 100
	}
	var recs []peerUpdateRecord
	if err := s.db.Where("peer_uuid = ? AND id > ?", peerUUID, afterID).
		Order("id ASC").Limit(limit).Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]peer.Update, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.toUpdate())
	}
	return out, nil
}

func (s *PeerStore) GetUpdate(peerUUID string, id int64) (peer.Update, error) {
	var rec peerUpdateRecord
	err := s.db.Where("peer_uuid = ? AND id = ?", peerUUID, id).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return peer.Update{}, peer.ErrNotFound
	}
	if err != nil {
		return peer.Update{}, err
	}
	return rec.toUpdate(), nil
}

func (s *PeerStore) AckUpdates(peerUUID string, upTo int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Never move the cursor backwards.
		res := tx.Model(&peerRecord{}).
			Where("uuid = ? AND acked_update_id < ?", peerUUID, upTo).
			Update("acked_update_id", upTo)
		if res.Error != nil {
			return res.Error
		}
		var count int64
		if err := tx.Model(&peerRecord{}).Where("uuid = ?", peerUUID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return peer.ErrNotFound
		}
		return tx.Where("peer_uuid = ? AND id <= ?", peerUUID, upTo).
			Delete(&peerUpdateRecord{}).Error
	})
}

// Compile-time contract with the remote layer, same shape as the chat/session
// stores: the dependency runs db → remote, never the reverse.
var _ peer.Store = (*PeerStore)(nil)
