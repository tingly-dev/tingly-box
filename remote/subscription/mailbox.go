package subscription

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Mailbox is the inbound delivery hub: it persists events through the Store,
// wakes long-pollers, and raises the once-per-offline-episode notice when a
// message is queued while no poller is connected (spec §6 — a periodic tool
// is usually offline; silence would read as broken, a notice per message
// would be spam).
type Mailbox struct {
	store Store

	mu sync.Mutex
	// waiters holds the wake channels of in-flight Poll calls, per
	// subscription. Presence of a waiter == "the tool is connected right now".
	waiters map[string][]chan struct{}
	// noticeSent marks that the offline notice went out for the current
	// offline episode; cleared when a poller connects.
	noticeSent map[string]bool

	// notify delivers the in-chat offline notice. Set by the wiring
	// (SetOfflineNotifier); nil = no notice (standalone/test setups).
	notify func(sub Subscription, queued Event)
}

// NewMailbox builds a mailbox over the store.
func NewMailbox(store Store) *Mailbox {
	return &Mailbox{
		store:      store,
		waiters:    make(map[string][]chan struct{}),
		noticeSent: make(map[string]bool),
	}
}

// SetOfflineNotifier wires the in-chat notice callback. The callback runs on
// its own goroutine — it sends a chat message and must not block Enqueue.
func (m *Mailbox) SetOfflineNotifier(fn func(sub Subscription, queued Event)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notify = fn
}

// Enqueue persists one inbound event for the subscription and wakes any
// waiting poller. When nobody is waiting it raises the offline notice once
// per offline episode.
func (m *Mailbox) Enqueue(sub Subscription, ev Event) error {
	ev.SubscriptionUUID = sub.UUID
	ev.BotUUID = sub.BotUUID
	dropped, err := m.store.AppendEvent(&ev)
	if err != nil {
		return err
	}
	if dropped > 0 {
		logrus.WithFields(logrus.Fields{
			"subscription": sub.UUID,
			"dropped":      dropped,
		}).Warn("subscription mailbox over capacity; dropped oldest unacked events")
	}

	m.mu.Lock()
	ws := m.waiters[sub.UUID]
	var notify func(Subscription, Event)
	if len(ws) > 0 {
		// Someone is connected: wake them all (they re-read from the store,
		// so extra wakes are harmless).
		for _, w := range ws {
			close(w)
		}
		m.waiters[sub.UUID] = nil
	} else if !m.noticeSent[sub.UUID] && m.notify != nil {
		m.noticeSent[sub.UUID] = true
		notify = m.notify
	}
	m.mu.Unlock()

	if notify != nil {
		go notify(sub, ev)
	}
	return nil
}

// Poll returns the subscription's undelivered events (id > its acked
// cursor), oldest first, waiting up to timeout when the mailbox is empty.
// A connected poller ends the offline episode.
func (m *Mailbox) Poll(ctx context.Context, subscriptionUUID string, timeout time.Duration, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	deadline := time.Now().Add(timeout)

	// The poller showing up ends the offline episode, whether or not events
	// are pending.
	m.mu.Lock()
	m.noticeSent[subscriptionUUID] = false
	m.mu.Unlock()

	for {
		sub, err := m.store.Get(subscriptionUUID)
		if err != nil {
			return nil, err
		}
		events, err := m.store.EventsAfter(subscriptionUUID, sub.AckedEventID, limit)
		if err != nil {
			return nil, err
		}
		if len(events) > 0 {
			return events, nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return []Event{}, nil
		}

		wake := make(chan struct{})
		m.mu.Lock()
		m.waiters[subscriptionUUID] = append(m.waiters[subscriptionUUID], wake)
		m.mu.Unlock()

		timer := time.NewTimer(remaining)
		select {
		case <-wake:
			timer.Stop()
		case <-timer.C:
			m.removeWaiter(subscriptionUUID, wake)
			return []Event{}, nil
		case <-ctx.Done():
			timer.Stop()
			m.removeWaiter(subscriptionUUID, wake)
			return []Event{}, ctx.Err()
		}
	}
}

// Ack advances the subscription's cursor (never backwards) and prunes acked
// events.
func (m *Mailbox) Ack(subscriptionUUID string, upTo int64) error {
	return m.store.AckEvents(subscriptionUUID, upTo)
}

// HasWaiter reports whether a poller is currently connected — used by /subs
// to show the tool's live state.
func (m *Mailbox) HasWaiter(subscriptionUUID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.waiters[subscriptionUUID]) > 0
}

func (m *Mailbox) removeWaiter(subscriptionUUID string, wake chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.waiters[subscriptionUUID]
	for i, w := range ws {
		if w == wake {
			m.waiters[subscriptionUUID] = append(ws[:i], ws[i+1:]...)
			return
		}
	}
}

// RecentSends tracks the platform message ids a subscription recently sent
// per chat, so a human replying to one of those messages routes to the
// sender (addressing tier 2). In-memory and bounded: losing it on restart
// only disables reply-to until the tool sends again — mention and sticky
// still work (spec §6).
type RecentSends struct {
	mu    sync.Mutex
	byKey map[string]string // chatID + "\x00" + messageID → subscription uuid
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
func (r *RecentSends) Track(chatID, messageID, subscriptionUUID string) {
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
	r.byKey[key] = subscriptionUUID
}

// Lookup resolves a replied-to message to the subscription that sent it
// ("" when unknown).
func (r *RecentSends) Lookup(chatID, messageID string) string {
	if messageID == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byKey[chatID+"\x00"+messageID]
}
