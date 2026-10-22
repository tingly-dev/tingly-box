package db

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ---- IM channel bindings ----

// bindingSelect reads bindings with the project of their thread.
const bindingSelect = "channel_bindings.*, COALESCE(threads.project_id, '') AS project_id"

func (s *Store) bindings() *gorm.DB {
	return s.db.Table("channel_bindings").Select(bindingSelect).
		Joins("LEFT JOIN threads ON threads.id = channel_bindings.thread_id")
}

// BindingFilter narrows ListChannelBindings; both empty lists all.
type BindingFilter struct {
	// ProjectID lists the chats bound to any thread of the project.
	ProjectID string
	// ThreadID lists the chats bound to exactly this thread.
	ThreadID string
}

// ListChannelBindings returns bindings, newest first. A chat that has not
// reached a thread yet (only a Folder) is not listed.
func (s *Store) ListChannelBindings(f BindingFilter) ([]ChannelBinding, error) {
	out := []ChannelBinding{}
	q := s.bindings().Where("channel_bindings.thread_id <> ''").
		Order("channel_bindings.created_at DESC, channel_bindings.rowid DESC")
	if f.ProjectID != "" {
		q = q.Where("threads.project_id = ?", f.ProjectID)
	}
	if f.ThreadID != "" {
		q = q.Where("channel_bindings.thread_id = ?", f.ThreadID)
	}
	err := q.Scan(&out).Error
	return out, err
}

// ListChannelBindingsOf returns the bindings of any of the given threads.
func (s *Store) ListChannelBindingsOf(threadIDs []string) ([]ChannelBinding, error) {
	out := []ChannelBinding{}
	if len(threadIDs) == 0 {
		return out, nil
	}
	err := s.bindings().Where("channel_bindings.thread_id IN ?", threadIDs).
		Order("channel_bindings.created_at, channel_bindings.rowid").Scan(&out).Error
	return out, err
}

// GetChannelBinding loads the binding of an IM chat.
func (s *Store) GetChannelBinding(platform, chatID string) (*ChannelBinding, error) {
	var b ChannelBinding
	res := s.bindings().Where("channel_bindings.platform = ? AND channel_bindings.chat_id = ?", platform, chatID).Limit(1).Scan(&b)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &b, nil
}

// UpsertChannelBinding binds b.Platform/b.ChatID to the thread b.ThreadID. A
// chat that is already bound moves to the new thread (its id and created_at
// stay, and a known title is kept when b has none). b is filled in with the
// stored row.
func (s *Store) UpsertChannelBinding(b *ChannelBinding) error {
	b.Platform = strings.TrimSpace(b.Platform)
	b.ChatID = strings.TrimSpace(b.ChatID)
	now := time.Now()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var cur ChannelBinding
		err := tx.First(&cur, "platform = ? AND chat_id = ?", b.Platform, b.ChatID).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			b.ID, b.CreatedAt, b.UpdatedAt = newID(), now, now
			return tx.Create(b).Error
		case err != nil:
			return err
		}
		cur.ThreadID, cur.NewThread, cur.Folder = b.ThreadID, b.NewThread, b.Folder
		if t := strings.TrimSpace(b.ChatTitle); t != "" {
			cur.ChatTitle = t
		}
		cur.UpdatedAt = now
		if err := tx.Save(&cur).Error; err != nil {
			return err
		}
		*b = cur
		return nil
	})
	if err != nil {
		return err
	}
	if t, err := s.GetThread(b.ThreadID); err == nil {
		b.ProjectID = t.ProjectID
	}
	return nil
}

// DeleteChannelBinding unbinds a chat.
func (s *Store) DeleteChannelBinding(id string) error {
	res := s.db.Where("id = ?", id).Delete(&ChannelBinding{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProjectChannelBindings unbinds every chat bound to a thread of the
// project and returns how many there were.
func (s *Store) DeleteProjectChannelBindings(projectID string) (int64, error) {
	res := s.db.Where("thread_id IN (?)", s.db.Model(&Thread{}).Select("id").Where("project_id = ?", projectID)).
		Delete(&ChannelBinding{})
	return res.RowsAffected, res.Error
}

// SetChannelBindingFolder makes a chat's next message start a new thread in
// folder (see ChannelBinding.Folder); a chat with no binding gets one that
// has only the folder.
func (s *Store) SetChannelBindingFolder(platform, chatID, title, folder, profile string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var cur ChannelBinding
		err := tx.First(&cur, "platform = ? AND chat_id = ?", platform, chatID).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(&ChannelBinding{ID: newID(), Platform: platform, ChatID: chatID,
				ChatTitle: strings.TrimSpace(title), Folder: folder, Profile: profile, CreatedAt: now, UpdatedAt: now}).Error
		case err != nil:
			return err
		}
		return tx.Model(&cur).Updates(map[string]any{"folder": folder, "profile": profile, "new_thread": false, "updated_at": now}).Error
	})
}

// SetChannelBindingNewThread sets whether a chat's next message starts a new
// thread (see ChannelBinding.NewThread).
func (s *Store) SetChannelBindingNewThread(id string, on bool) error {
	return s.db.Model(&ChannelBinding{}).Where("id = ?", id).
		Updates(map[string]any{"new_thread": on, "updated_at": time.Now()}).Error
}

// SetChannelBindingVerbose sets whether a chat hears each step of its
// thread's turns (see ChannelBinding.Verbose).
func (s *Store) SetChannelBindingVerbose(id string, on bool) error {
	return s.db.Model(&ChannelBinding{}).Where("id = ?", id).
		Updates(map[string]any{"verbose": on, "updated_at": time.Now()}).Error
}

// MoveChannelBindings moves every chat bound to one thread to another, and
// reports how many moved.
func (s *Store) MoveChannelBindings(fromThreadID, toThreadID string) (int64, error) {
	res := s.db.Model(&ChannelBinding{}).Where("thread_id = ?", fromThreadID).
		Updates(map[string]any{"thread_id": toThreadID, "new_thread": false, "updated_at": time.Now()})
	return res.RowsAffected, res.Error
}
