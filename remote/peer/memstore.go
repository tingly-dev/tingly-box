package peer

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemStore is an in-memory Store used by tests and standalone (host-less)
// setups. The production store is SQLite-backed (internal/db).
type MemStore struct {
	mu      sync.Mutex
	peers   map[string]Peer
	updates map[string][]Update // peer uuid → updates, ascending id
	nextID  int64
}

// NewMemStore builds an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{peers: make(map[string]Peer), updates: make(map[string][]Update)}
}

func (s *MemStore) Create(p *Peer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.peers {
		if existing.Name == p.Name {
			return ErrNameTaken
		}
	}
	if p.UUID == "" {
		p.UUID = uuid.NewString()
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	s.peers[p.UUID] = *p
	return nil
}

func (s *MemStore) Get(uuid string) (Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[uuid]
	if !ok {
		return Peer{}, ErrNotFound
	}
	return p, nil
}

func (s *MemStore) GetByToken(tokenHash string) (Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.peers {
		if p.TokenHash != "" && p.TokenHash == tokenHash {
			return p, nil
		}
	}
	return Peer{}, ErrNotFound
}

func (s *MemStore) List() ([]Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Peer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemStore) ListByBot(botUUID string) ([]Peer, error) {
	all, _ := s.List()
	out := make([]Peer, 0, len(all))
	for _, p := range all {
		if p.BotUUID == botUUID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *MemStore) HasEnabledForBot(botUUID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.peers {
		if p.BotUUID == botUUID && p.Enabled {
			return true
		}
	}
	return false
}

func (s *MemStore) Save(p *Peer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.peers[p.UUID]; !ok {
		return ErrNotFound
	}
	for id, existing := range s.peers {
		if id != p.UUID && existing.Name == p.Name {
			return ErrNameTaken
		}
	}
	p.UpdatedAt = time.Now()
	s.peers[p.UUID] = *p
	return nil
}

func (s *MemStore) Delete(uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, uuid)
	delete(s.updates, uuid)
	return nil
}

func (s *MemStore) AppendUpdate(u *Update) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	u.ID = s.nextID
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	list := append(s.updates[u.PeerUUID], *u)
	dropped := 0
	for len(list) > MaxQueuedUpdates {
		list = list[1:]
		dropped++
	}
	s.updates[u.PeerUUID] = list
	return dropped, nil
}

func (s *MemStore) UpdatesAfter(peerUUID string, afterID int64, limit int) ([]Update, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Update{}
	for _, u := range s.updates[peerUUID] {
		if u.ID > afterID {
			out = append(out, u)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (s *MemStore) GetUpdate(peerUUID string, id int64) (Update, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.updates[peerUUID] {
		if u.ID == id {
			return u, nil
		}
	}
	return Update{}, ErrNotFound
}

func (s *MemStore) AckUpdates(peerUUID string, upTo int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[peerUUID]
	if !ok {
		return ErrNotFound
	}
	if upTo > p.AckedUpdateID {
		p.AckedUpdateID = upTo
		s.peers[peerUUID] = p
	}
	kept := s.updates[peerUUID][:0]
	for _, u := range s.updates[peerUUID] {
		if u.ID > p.AckedUpdateID {
			kept = append(kept, u)
		}
	}
	s.updates[peerUUID] = kept
	return nil
}

var _ Store = (*MemStore)(nil)
