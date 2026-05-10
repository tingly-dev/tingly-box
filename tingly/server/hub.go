package server

import (
	"sync"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/wire"
)

// hub is the in-memory routing table for live WebSocket connections.
//
// Bots are looked up by botID; multiple bot connections for the same UUID
// (e.g. failover) are supported and all receive forwarded frames.
//
// Chat connections are keyed by (botID, chatID). Multiple chat clients can
// be open on the same chat (e.g. user has the chat open on phone + desktop).
type hub struct {
	mu sync.RWMutex

	bots  map[string]map[*conn]struct{}            // botID → set of bot connections
	chats map[string]map[string]map[*conn]struct{} // botID → chatID → set of chat connections
}

func newHub() *hub {
	return &hub{
		bots:  make(map[string]map[*conn]struct{}),
		chats: make(map[string]map[string]map[*conn]struct{}),
	}
}

func (h *hub) addBot(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.bots[c.botID]
	if set == nil {
		set = make(map[*conn]struct{})
		h.bots[c.botID] = set
	}
	set[c] = struct{}{}
}

func (h *hub) removeBot(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.bots[c.botID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.bots, c.botID)
		}
	}
}

func (h *hub) addChat(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	byBot := h.chats[c.botID]
	if byBot == nil {
		byBot = make(map[string]map[*conn]struct{})
		h.chats[c.botID] = byBot
	}
	set := byBot[c.chatID]
	if set == nil {
		set = make(map[*conn]struct{})
		byBot[c.chatID] = set
	}
	set[c] = struct{}{}
}

func (h *hub) removeChat(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	byBot := h.chats[c.botID]
	if byBot == nil {
		return
	}
	if set := byBot[c.chatID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(byBot, c.chatID)
		}
	}
	if len(byBot) == 0 {
		delete(h.chats, c.botID)
	}
}

// botConns snapshots the live bot connections for a given bot ID.
func (h *hub) botConns(botID string) []*conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set := h.bots[botID]
	if len(set) == 0 {
		return nil
	}
	out := make([]*conn, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	return out
}

// chatConns snapshots the live chat connections for (botID, chatID).
func (h *hub) chatConns(botID, chatID string) []*conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	byBot := h.chats[botID]
	if byBot == nil {
		return nil
	}
	set := byBot[chatID]
	if len(set) == 0 {
		return nil
	}
	out := make([]*conn, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	return out
}

// fanout sends f to every conn in conns. A failed write closes the conn so
// the read loop notices and unregisters it. Errors are intentionally
// swallowed — the read side is the source of truth for connection health.
func fanout(conns []*conn, f wire.Frame) {
	for _, c := range conns {
		_ = c.write(f)
	}
}
