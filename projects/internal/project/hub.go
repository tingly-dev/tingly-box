package project

import (
	"encoding/json"
	"sync"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// AllProjects subscribes to every project's events (the sidebar's stream); it
// carries thread, project and coordinator events, but neither streamed
// coordinator text nor thread transcript entries (tool results and payloads,
// which only the project's own stream needs).
const AllProjects = "*"

// Event types pushed to a project's SSE stream.
const (
	EventMessageDelta = "message_delta" // streamed coordinator text
	EventThread       = "thread"        // a thread changed
	EventThreadEvent  = "thread_event"  // a thread transcript entry was added or changed (the project conversation is its main thread's)
	EventProject      = "project"       // project settings or state changed
	EventCoordinator  = "coordinator"   // the coordinator started or finished working ({"busy": bool})
)

// Event is one SSE payload.
type Event struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id"`
	// Data is the created/changed object (message, thread, thread event,
	// project) or, for message_delta, {"id", "text"}.
	Data any `json:"data"`
	// Threads is, on a thread event, its project's thread counts after the
	// change, so a project list can update its row without re-fetching.
	Threads *db.ThreadCounts `json:"threads,omitempty"`
}

// Hub fans project events out to subscribers (open project pages).
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan []byte]struct{}
}

// NewHub builds an empty hub.
func NewHub() *Hub { return &Hub{subs: map[string]map[chan []byte]struct{}{}} }

// Subscribe returns a channel of encoded events for projectID and a cancel
// func that must be called when the subscriber goes away.
func (h *Hub) Subscribe(projectID string) (<-chan []byte, func()) {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	if h.subs[projectID] == nil {
		h.subs[projectID] = map[chan []byte]struct{}{}
	}
	h.subs[projectID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs[projectID], ch)
		h.mu.Unlock()
	}
}

// Publish sends an event to every subscriber of its project. A subscriber
// that cannot keep up drops events; the page re-fetches on reconnect.
func (h *Hub) Publish(ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	send := func(subs map[chan []byte]struct{}) {
		for ch := range subs {
			select {
			case ch <- b:
			default:
			}
		}
	}
	send(h.subs[ev.ProjectID])
	if ev.Type != EventMessageDelta && ev.Type != EventThreadEvent {
		send(h.subs[AllProjects])
	}
}
