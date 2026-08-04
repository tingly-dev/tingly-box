package subscription

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemStore is an in-memory Store used by tests and standalone (host-less)
// setups. The production store is SQLite-backed (internal/data/db).
type MemStore struct {
	mu     sync.Mutex
	subs   map[string]Subscription
	events map[string][]Event // subscription uuid → events, ascending id
	nextID int64
}

// NewMemStore builds an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{subs: make(map[string]Subscription), events: make(map[string][]Event)}
}

func (s *MemStore) Create(sub *Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.subs {
		if existing.Name == sub.Name {
			return ErrNameTaken
		}
	}
	if sub.UUID == "" {
		sub.UUID = uuid.NewString()
	}
	now := time.Now()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now
	s.subs[sub.UUID] = *sub
	return nil
}

func (s *MemStore) Get(uuid string) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[uuid]
	if !ok {
		return Subscription{}, ErrNotFound
	}
	return sub, nil
}

func (s *MemStore) GetByToken(tokenHash string) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.subs {
		if sub.TokenHash != "" && sub.TokenHash == tokenHash {
			return sub, nil
		}
	}
	return Subscription{}, ErrNotFound
}

func (s *MemStore) List() ([]Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, sub)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemStore) ListByBot(botUUID string) ([]Subscription, error) {
	all, _ := s.List()
	out := make([]Subscription, 0, len(all))
	for _, sub := range all {
		if sub.BotUUID == botUUID {
			out = append(out, sub)
		}
	}
	return out, nil
}

func (s *MemStore) HasEnabledForBot(botUUID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.subs {
		if sub.BotUUID == botUUID && sub.Enabled {
			return true
		}
	}
	return false
}

func (s *MemStore) Update(sub *Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub.UUID]; !ok {
		return ErrNotFound
	}
	for id, existing := range s.subs {
		if id != sub.UUID && existing.Name == sub.Name {
			return ErrNameTaken
		}
	}
	sub.UpdatedAt = time.Now()
	s.subs[sub.UUID] = *sub
	return nil
}

func (s *MemStore) Delete(uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, uuid)
	delete(s.events, uuid)
	return nil
}

func (s *MemStore) AppendEvent(ev *Event) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	ev.ID = s.nextID
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	list := append(s.events[ev.SubscriptionUUID], *ev)
	dropped := 0
	for len(list) > MaxQueuedEvents {
		list = list[1:]
		dropped++
	}
	s.events[ev.SubscriptionUUID] = list
	return dropped, nil
}

func (s *MemStore) EventsAfter(subscriptionUUID string, afterID int64, limit int) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Event{}
	for _, ev := range s.events[subscriptionUUID] {
		if ev.ID > afterID {
			out = append(out, ev)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *MemStore) AckEvents(subscriptionUUID string, upTo int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[subscriptionUUID]
	if !ok {
		return ErrNotFound
	}
	if upTo > sub.AckedEventID {
		sub.AckedEventID = upTo
		s.subs[subscriptionUUID] = sub
	}
	kept := s.events[subscriptionUUID][:0]
	for _, ev := range s.events[subscriptionUUID] {
		if ev.ID > sub.AckedEventID {
			kept = append(kept, ev)
		}
	}
	s.events[subscriptionUUID] = kept
	return nil
}

var _ Store = (*MemStore)(nil)
