package db

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/remote/subscription"
)

// subscriptionRecord persists one Subscription (see remote/subscription and
// .design/subscription.md). Its own small table on purpose — not a Scenarios
// row and not a BotCapability merge.
type subscriptionRecord struct {
	ID           uint      `gorm:"primaryKey;autoIncrement;column:id"`
	UUID         string    `gorm:"uniqueIndex;column:uuid;not null;size:64"`
	Name         string    `gorm:"uniqueIndex;column:name;not null;size:64"`
	BotUUID      string    `gorm:"index:idx_subscriptions_bot;column:bot_uuid;not null;size:64"`
	ChatID       string    `gorm:"column:chat_id;not null;size:256"`
	// No gorm defaults on the booleans: a default tag makes GORM skip
	// zero-valued fields on insert, silently turning enabled=false into true.
	Exclusive bool `gorm:"column:exclusive"`
	Enabled   bool `gorm:"column:enabled"`
	TokenHash    string    `gorm:"index:idx_subscriptions_token;column:token_hash;size:64"`
	AckedEventID int64     `gorm:"column:acked_event_id;default:0"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (subscriptionRecord) TableName() string { return "subscriptions" }

// subscriptionEventRecord persists one inbound mailbox event. The
// autoincrement id doubles as the delivery cursor.
type subscriptionEventRecord struct {
	ID               int64     `gorm:"primaryKey;autoIncrement;column:id"`
	SubscriptionUUID string    `gorm:"index:idx_subscription_events_sub;column:subscription_uuid;not null;size:64"`
	BotUUID          string    `gorm:"column:bot_uuid;size:64"`
	ChatID           string    `gorm:"column:chat_id;size:256"`
	SenderID         string    `gorm:"column:sender_id;size:256"`
	MessageID        string    `gorm:"column:message_id;size:256"`
	Text             string    `gorm:"column:text"`
	ContextToken     string    `gorm:"column:context_token;size:512"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (subscriptionEventRecord) TableName() string { return "subscription_events" }

// SubscriptionStore is the SQLite-backed subscription.Store implementation.
type SubscriptionStore struct {
	db *gorm.DB
}

// NewSubscriptionStore builds the store over the shared DB handle.
func NewSubscriptionStore(db *gorm.DB) *SubscriptionStore {
	return &SubscriptionStore{db: db}
}

func recordFromSubscription(sub subscription.Subscription) subscriptionRecord {
	return subscriptionRecord{
		UUID:         sub.UUID,
		Name:         sub.Name,
		BotUUID:      sub.BotUUID,
		ChatID:       sub.ChatID,
		Exclusive:    sub.Exclusive,
		Enabled:      sub.Enabled,
		TokenHash:    sub.TokenHash,
		AckedEventID: sub.AckedEventID,
		CreatedAt:    sub.CreatedAt,
		UpdatedAt:    sub.UpdatedAt,
	}
}

func (r subscriptionRecord) toSubscription() subscription.Subscription {
	return subscription.Subscription{
		UUID:         r.UUID,
		Name:         r.Name,
		BotUUID:      r.BotUUID,
		ChatID:       r.ChatID,
		Exclusive:    r.Exclusive,
		Enabled:      r.Enabled,
		TokenHash:    r.TokenHash,
		AckedEventID: r.AckedEventID,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func (s *SubscriptionStore) Create(sub *subscription.Subscription) error {
	if sub.UUID == "" {
		sub.UUID = uuid.NewString()
	}
	now := time.Now()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now

	var count int64
	if err := s.db.Model(&subscriptionRecord{}).Where("name = ?", sub.Name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return subscription.ErrNameTaken
	}
	rec := recordFromSubscription(*sub)
	return s.db.Create(&rec).Error
}

func (s *SubscriptionStore) Get(subUUID string) (subscription.Subscription, error) {
	var rec subscriptionRecord
	err := s.db.Where("uuid = ?", subUUID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	if err != nil {
		return subscription.Subscription{}, err
	}
	return rec.toSubscription(), nil
}

func (s *SubscriptionStore) GetByToken(tokenHash string) (subscription.Subscription, error) {
	if tokenHash == "" {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	var rec subscriptionRecord
	err := s.db.Where("token_hash = ?", tokenHash).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	if err != nil {
		return subscription.Subscription{}, err
	}
	return rec.toSubscription(), nil
}

func (s *SubscriptionStore) List() ([]subscription.Subscription, error) {
	var recs []subscriptionRecord
	if err := s.db.Order("created_at DESC").Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]subscription.Subscription, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.toSubscription())
	}
	return out, nil
}

func (s *SubscriptionStore) ListByBot(botUUID string) ([]subscription.Subscription, error) {
	var recs []subscriptionRecord
	if err := s.db.Where("bot_uuid = ?", botUUID).Order("created_at DESC").Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]subscription.Subscription, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.toSubscription())
	}
	return out, nil
}

func (s *SubscriptionStore) HasEnabledForBot(botUUID string) bool {
	var count int64
	if err := s.db.Model(&subscriptionRecord{}).
		Where("bot_uuid = ? AND enabled = ?", botUUID, true).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (s *SubscriptionStore) Update(sub *subscription.Subscription) error {
	var count int64
	if err := s.db.Model(&subscriptionRecord{}).
		Where("name = ? AND uuid <> ?", sub.Name, sub.UUID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return subscription.ErrNameTaken
	}
	sub.UpdatedAt = time.Now()
	res := s.db.Model(&subscriptionRecord{}).Where("uuid = ?", sub.UUID).Updates(map[string]any{
		"name":           sub.Name,
		"bot_uuid":       sub.BotUUID,
		"chat_id":        sub.ChatID,
		"exclusive":      sub.Exclusive,
		"enabled":        sub.Enabled,
		"token_hash":     sub.TokenHash,
		"acked_event_id": sub.AckedEventID,
		"updated_at":     sub.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return subscription.ErrNotFound
	}
	return nil
}

func (s *SubscriptionStore) Delete(subUUID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_uuid = ?", subUUID).Delete(&subscriptionEventRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", subUUID).Delete(&subscriptionRecord{}).Error
	})
}

func (s *SubscriptionStore) AppendEvent(ev *subscription.Event) (int, error) {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	rec := subscriptionEventRecord{
		SubscriptionUUID: ev.SubscriptionUUID,
		BotUUID:          ev.BotUUID,
		ChatID:           ev.ChatID,
		SenderID:         ev.SenderID,
		MessageID:        ev.MessageID,
		Text:             ev.Text,
		ContextToken:     ev.ContextToken,
		CreatedAt:        ev.CreatedAt,
	}
	dropped := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&subscriptionEventRecord{}).
			Where("subscription_uuid = ?", ev.SubscriptionUUID).
			Count(&count).Error; err != nil {
			return err
		}
		if over := count - subscription.MaxQueuedEvents; over > 0 {
			// Drop oldest-first down to the cap; the caller logs the drop.
			res := tx.Where("id IN (?)", tx.Model(&subscriptionEventRecord{}).
				Select("id").
				Where("subscription_uuid = ?", ev.SubscriptionUUID).
				Order("id ASC").
				Limit(int(over)),
			).Delete(&subscriptionEventRecord{})
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
	ev.ID = rec.ID
	return dropped, nil
}

func (s *SubscriptionStore) EventsAfter(subscriptionUUID string, afterID int64, limit int) ([]subscription.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	var recs []subscriptionEventRecord
	if err := s.db.Where("subscription_uuid = ? AND id > ?", subscriptionUUID, afterID).
		Order("id ASC").Limit(limit).Find(&recs).Error; err != nil {
		return nil, err
	}
	out := make([]subscription.Event, 0, len(recs))
	for _, rec := range recs {
		out = append(out, subscription.Event{
			ID:               rec.ID,
			SubscriptionUUID: rec.SubscriptionUUID,
			BotUUID:          rec.BotUUID,
			ChatID:           rec.ChatID,
			SenderID:         rec.SenderID,
			MessageID:        rec.MessageID,
			Text:             rec.Text,
			ContextToken:     rec.ContextToken,
			CreatedAt:        rec.CreatedAt,
		})
	}
	return out, nil
}

func (s *SubscriptionStore) GetEvent(subscriptionUUID string, id int64) (subscription.Event, error) {
	var rec subscriptionEventRecord
	err := s.db.Where("subscription_uuid = ? AND id = ?", subscriptionUUID, id).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return subscription.Event{}, subscription.ErrNotFound
	}
	if err != nil {
		return subscription.Event{}, err
	}
	return subscription.Event{
		ID:               rec.ID,
		SubscriptionUUID: rec.SubscriptionUUID,
		BotUUID:          rec.BotUUID,
		ChatID:           rec.ChatID,
		SenderID:         rec.SenderID,
		MessageID:        rec.MessageID,
		Text:             rec.Text,
		ContextToken:     rec.ContextToken,
		CreatedAt:        rec.CreatedAt,
	}, nil
}

func (s *SubscriptionStore) AckEvents(subscriptionUUID string, upTo int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Never move the cursor backwards.
		res := tx.Model(&subscriptionRecord{}).
			Where("uuid = ? AND acked_event_id < ?", subscriptionUUID, upTo).
			Update("acked_event_id", upTo)
		if res.Error != nil {
			return res.Error
		}
		var count int64
		if err := tx.Model(&subscriptionRecord{}).Where("uuid = ?", subscriptionUUID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return subscription.ErrNotFound
		}
		return tx.Where("subscription_uuid = ? AND id <= ?", subscriptionUUID, upTo).
			Delete(&subscriptionEventRecord{}).Error
	})
}

// Compile-time contract with the remote layer, same shape as the chat/session
// stores: the dependency runs db → remote, never the reverse.
var _ subscription.Store = (*SubscriptionStore)(nil)
