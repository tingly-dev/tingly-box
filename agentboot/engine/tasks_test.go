package engine

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// liveStore remembers how many background tasks the engine reported at the
// moment each entry was appended, and the task events themselves.
type liveStore struct {
	*memStore
	e      *Engine
	mu     sync.Mutex
	seen   []int
	events []TaskEvent
}

func (s *liveStore) Append(id string, entry Entry) {
	s.memStore.Append(id, entry)
	if entry.Kind != KindTask {
		return
	}
	var ev TaskEvent
	_ = json.Unmarshal(entry.Payload, &ev)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	s.seen = append(s.seen, len(s.e.BackgroundTasks(id)))
}

func tasksChangedMessage(tasks ...map[string]any) *claude.SystemMessage {
	list := make([]any, 0, len(tasks))
	for _, t := range tasks {
		list = append(list, t)
	}
	return &claude.SystemMessage{SubType: claude.SystemSubtypeBackgroundTasksChanged, Raw: map[string]interface{}{"tasks": list}}
}

// The store appends a task event knowing the live set it describes: a page
// that reads the running tasks when the event arrives must see them.
func TestRecord_LiveSetIsCurrentWhenTheEventIsAppended(t *testing.T) {
	store := &liveStore{memStore: newMemStore()}
	e := New(Config{Store: store})
	store.e = e
	conv := newConverter()

	e.record("s1", conv, tasksChangedMessage(
		map[string]any{"task_id": "b1", "task_type": "local_bash", "description": "pnpm test"},
		map[string]any{"task_id": "a1", "task_type": "local_agent", "description": "review"},
	))
	e.record("s1", conv, tasksChangedMessage())

	if len(store.seen) != 2 || store.seen[0] != 2 || store.seen[1] != 0 {
		t.Fatalf("running tasks seen at append = %v, want [2 0]", store.seen)
	}
}

// Tasks that die with their process leave no event of their own; the
// transcript must still say the set is now empty, or pages show them running
// until the next reload.
func TestClearTasks_SaysTheSetIsEmptyWhenTasksDiedWithTheProcess(t *testing.T) {
	store := &liveStore{memStore: newMemStore()}
	e := New(Config{Store: store})
	store.e = e
	e.record("s1", newConverter(), tasksChangedMessage(map[string]any{"task_id": "b1", "task_type": "local_bash", "description": "sleep 600"}))
	store.events, store.seen = nil, nil

	e.clearTasks("s1")

	if len(store.events) != 1 || store.events[0].Event != "background_tasks_changed" || len(store.events[0].Tasks) != 0 {
		t.Fatalf("events after clearTasks = %+v, want one empty background_tasks_changed", store.events)
	}
	if store.seen[0] != 0 {
		t.Fatalf("live set still has %d tasks when the event is appended", store.seen[0])
	}
	if got := e.BackgroundTasks("s1"); len(got) != 0 {
		t.Fatalf("tasks = %+v, want none", got)
	}

	// Nothing was running: nothing to say.
	store.events = nil
	e.clearTasks("s1")
	if len(store.events) != 0 {
		t.Fatalf("clearTasks of an empty set appended %+v", store.events)
	}
}
