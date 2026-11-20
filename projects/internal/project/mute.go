package project

import (
	"errors"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ErrMutePast is for a mute that would end before it starts.
var ErrMutePast = errors.New("muted_until must be in the future")

// MuteChannel mutes a bound chat until until (nil: until unmuted), or unmutes
// it. The binding stays as it is; only what is sent to the chat stops (see
// db.ChannelBinding.Muted). A change of state leaves one note in the thread
// the chat talks to.
func (s *Service) MuteChannel(id string, muted bool, until *time.Time) (*db.ChannelBinding, error) {
	now := time.Now()
	if muted && until != nil && !until.After(now) {
		return nil, ErrMutePast
	}
	before, err := s.Store.GetChannelBindingByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.Store.SetChannelBindingMute(id, muted, until); err != nil {
		return nil, err
	}
	b, err := s.Store.GetChannelBindingByID(id)
	if err != nil {
		return nil, err
	}
	s.Relay.resetMuteNotice(b.Platform, b.ChatID)
	if before.Muted != b.Muted || !sameTime(before.MutedUntil, b.MutedUntil) {
		s.noteMute(b)
	}
	return b, nil
}

// noteMute records in the chat's thread that the chat was muted or unmuted,
// once per change, so the transcript explains why the chat went quiet.
func (s *Service) noteMute(b *db.ChannelBinding) {
	if b.ThreadID == "" || s.Threads == nil {
		return
	}
	t, err := s.Store.GetThread(b.ThreadID)
	if err != nil {
		return
	}
	s.Threads.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Source: db.SourceSystem, Content: muteNote(b)})
}

func muteNote(b *db.ChannelBinding) string {
	chat := chatName(b)
	if !b.Muted {
		return "IM " + chat + " unmuted: it hears replies and updates again."
	}
	return "IM " + chat + " muted " + mutedWhile(b) + ": nothing is sent to it; its messages still arrive."
}

// mutedWhile says how long a mute lasts: "until Nov 21 18:00" (server time)
// or "until unmuted".
func mutedWhile(b *db.ChannelBinding) string {
	if b.MutedUntil == nil {
		return "until unmuted"
	}
	return "until " + b.MutedUntil.Local().Format("Jan 2 15:04")
}

func chatName(b *db.ChannelBinding) string {
	if t := strings.TrimSpace(b.ChatTitle); t != "" {
		return t
	}
	return b.ChatID
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// mutedReplyNotice is what a muted chat is told the first time it writes
// during a mute: its message went through, but the reply will not come here.
func mutedReplyNotice(b *db.ChannelBinding) string {
	return "This chat is muted " + mutedWhile(b) + ". Your message reached Projects; replies stay on the web until you unmute it on the Bind page."
}
