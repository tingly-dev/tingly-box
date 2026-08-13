package peer

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Inbox is a peer's inbound stream hub: it persists updates through the
// Store, wakes long-pollers, and raises the once-per-offline-episode notice
// when an update is queued while no poller is connected (spec §6 — a
// periodic tool is usually offline; silence would read as broken, a notice
// per message would be spam).
type Inbox struct {
	store Store

	mu sync.Mutex
	// waiters holds the wake channels of in-flight Poll calls, per peer.
	// Presence of a waiter == "the tool is connected right now".
	waiters map[string][]chan struct{}
	// noticeSent marks that the offline notice went out for the current
	// offline episode; cleared when a poller connects.
	noticeSent map[string]bool

	// notify delivers the in-chat offline notice. Set by the wiring
	// (SetOfflineNotifier); nil = no notice (standalone/test setups).
	notify func(p Peer, queued Update)
}

// NewInbox builds an inbox over the store.
func NewInbox(store Store) *Inbox {
	return &Inbox{
		store:      store,
		waiters:    make(map[string][]chan struct{}),
		noticeSent: make(map[string]bool),
	}
}

// SetOfflineNotifier wires the in-chat notice callback. The callback runs on
// its own goroutine — it sends a chat message and must not block Enqueue.
func (in *Inbox) SetOfflineNotifier(fn func(p Peer, queued Update)) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.notify = fn
}

// Enqueue persists one inbound update for the peer and wakes any waiting
// poller. When nobody is waiting it raises the offline notice once per
// offline episode.
func (in *Inbox) Enqueue(p Peer, u Update) error {
	u.PeerUUID = p.UUID
	u.BotUUID = p.BotUUID
	if u.Type == "" {
		u.Type = UpdateTypeMessage
	}
	dropped, err := in.store.AppendUpdate(&u)
	if err != nil {
		return err
	}
	if dropped > 0 {
		logrus.WithFields(logrus.Fields{
			"peer":    p.UUID,
			"dropped": dropped,
		}).Warn("peer inbox over capacity; dropped oldest unconfirmed updates")
	}

	in.mu.Lock()
	ws := in.waiters[p.UUID]
	var notify func(Peer, Update)
	if len(ws) > 0 {
		// Someone is connected: wake them all (they re-read from the store,
		// so extra wakes are harmless).
		for _, w := range ws {
			close(w)
		}
		in.waiters[p.UUID] = nil
	} else if !in.noticeSent[p.UUID] && in.notify != nil {
		in.noticeSent[p.UUID] = true
		notify = in.notify
	}
	in.mu.Unlock()

	if notify != nil {
		go notify(p, u)
	}
	return nil
}

// Poll implements the getUpdates verb. A positive offset first confirms
// every update with id < offset (advancing the cursor and pruning — the
// Telegram idiom: pass last_update_id+1 on the next poll). It then returns
// the peer's unconfirmed updates, oldest first, waiting up to timeout when
// there are none. Offset 0 re-reads unconfirmed updates, so a tool that
// crashed mid-batch sees the batch again. A connected poller ends the
// offline episode.
func (in *Inbox) Poll(ctx context.Context, peerUUID string, offset int64, timeout time.Duration, limit int) ([]Update, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset > 0 {
		if err := in.store.AckUpdates(peerUUID, offset-1); err != nil {
			return nil, err
		}
	}
	deadline := time.Now().Add(timeout)

	// The poller showing up ends the offline episode, whether or not
	// updates are pending.
	in.mu.Lock()
	in.noticeSent[peerUUID] = false
	in.mu.Unlock()

	for {
		p, err := in.store.Get(peerUUID)
		if err != nil {
			return nil, err
		}
		updates, err := in.store.UpdatesAfter(peerUUID, p.AckedUpdateID, limit)
		if err != nil {
			return nil, err
		}
		if len(updates) > 0 {
			return updates, nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return []Update{}, nil
		}

		wake := make(chan struct{})
		in.mu.Lock()
		in.waiters[peerUUID] = append(in.waiters[peerUUID], wake)
		in.mu.Unlock()

		timer := time.NewTimer(remaining)
		select {
		case <-wake:
			timer.Stop()
		case <-timer.C:
			in.removeWaiter(peerUUID, wake)
			return []Update{}, nil
		case <-ctx.Done():
			timer.Stop()
			in.removeWaiter(peerUUID, wake)
			return nil, ctx.Err()
		}
	}
}

// HasWaiter reports whether a poller is currently connected — used by
// /peers to show the tool's live state.
func (in *Inbox) HasWaiter(peerUUID string) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return len(in.waiters[peerUUID]) > 0
}

func (in *Inbox) removeWaiter(peerUUID string, wake chan struct{}) {
	in.mu.Lock()
	defer in.mu.Unlock()
	ws := in.waiters[peerUUID]
	for i, w := range ws {
		if w == wake {
			in.waiters[peerUUID] = append(ws[:i], ws[i+1:]...)
			return
		}
	}
}

// RecentSends tracks the platform message ids a peer recently sent per chat,
// so a human replying to one of those messages routes to the sender
// (addressing tier 2). In-memory and bounded: losing it on restart only
// disables reply-to until the tool sends again — mention and sticky still
// work (spec §6).
type RecentSends struct {
	mu    sync.Mutex
	byKey map[string]string // chatID + "\x00" + messageID → peer uuid
	order []string          // FIFO eviction
	cap   int
}

// NewRecentSends builds a tracker holding up to cap entries (256 when <=0).
func NewRecentSends(cap int) *RecentSends {
	if cap <= 0 {
		cap = 256
	}
	return &RecentSends{byKey: make(map[string]string), cap: cap}
}

// Track records one sent message.
func (r *RecentSends) Track(chatID, messageID, peerUUID string) {
	if messageID == "" {
		return
	}
	key := chatID + "\x00" + messageID
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byKey[key]; !exists {
		r.order = append(r.order, key)
		for len(r.order) > r.cap {
			delete(r.byKey, r.order[0])
			r.order = r.order[1:]
		}
	}
	r.byKey[key] = peerUUID
}

// Lookup resolves a replied-to message to the peer that sent it ("" when
// unknown).
func (r *RecentSends) Lookup(chatID, messageID string) string {
	if messageID == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byKey[chatID+"\x00"+messageID]
}
