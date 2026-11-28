package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// eventsPayload mirrors the HTTP body of GET /threads/:id/events, so the sizes
// logged here are what the browser receives (Gin writes the same JSON).
type eventsPayload struct {
	Events []ThreadEvent `json:"events"`
	More   bool          `json:"more"`
}

func payloadBytes(t *testing.T, events []ThreadEvent, more bool) int {
	t.Helper()
	out, err := json.Marshal(eventsPayload{Events: events, More: more})
	require.NoError(t, err)
	return len(out)
}

const (
	measureProgressTools = "TodoWrite,TaskCreate,TaskUpdate,TaskList,TaskGet"
	measureBigResult     = "a tool result whose output is the bulk of a transcript"
	measureTodos         = `{"todos":[{"content":"read the code","status":"completed"},{"content":"change the cache","status":"in_progress"}]}`
)

// seedTranscript writes n conversation events with a realistic mix: mostly
// large tool output, plus assistant text, user turns and the checklist tool.
func seedTranscript(t *testing.T, s *Store, threadID string, n int) {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	big := strings.Repeat(measureBigResult, 100) // ~4.5KB, a typical tool result
	for i := 0; i < n; i++ {
		e := ThreadEvent{ThreadID: threadID, CreatedAt: base.Add(time.Duration(i) * time.Minute)}
		switch i % 20 {
		case 0:
			e.Kind, e.Content = EvUser, "please change the transcript loading"
		case 1:
			e.Kind, e.Content = EvAssistant, strings.Repeat("reasoning about the cache ", 20)
		case 5:
			e.Kind, e.Content, e.RequestID, e.Payload = EvToolUse, "TodoWrite", fmt.Sprintf("t%d", i), measureTodos
		case 6:
			e.Kind, e.Content, e.RequestID = EvToolResult, "ok", fmt.Sprintf("t%d", i-1)
		case 10:
			e.Kind, e.Content, e.RequestID, e.Payload = EvToolUse, "Bash", fmt.Sprintf("b%d", i), `{"command":"go test ./..."}`
		default:
			e.Kind, e.Content, e.RequestID = EvToolResult, big, fmt.Sprintf("b%d", i-1)
		}
		require.NoError(t, s.AddThreadEvent(&e))
	}
	// Distinct write times in creation order, so `since` is measurable.
	require.NoError(t, s.db.Exec("UPDATE thread_events SET updated_at = created_at WHERE thread_id = ?", threadID).Error)
}

func TestTranscriptLoadMeasurement(t *testing.T) {
	for _, n := range []int{200, 1000} {
		t.Run(fmt.Sprintf("%d events", n), func(t *testing.T) {
			s := newStore(t)
			_, main := newProject(t, s, "P")
			seedTranscript(t, s, main.ID, n)

			full, _, err := s.PageThreadEvents(main.ID, EventPage{})
			require.NoError(t, err)
			page, more, err := s.PageThreadEvents(main.ID, EventPage{Limit: 200})
			require.NoError(t, err)
			progress, _, err := s.PageThreadEvents(main.ID, EventPage{Tools: strings.Split(measureProgressTools, ",")})
			require.NoError(t, err)

			// Three new events and one old one changed since the page was read.
			cursor := page[len(page)-1].UpdatedAt
			var changedID string
			require.NoError(t, s.db.Model(&ThreadEvent{}).Where("thread_id = ?", main.ID).Order("created_at").Limit(1).Pluck("id", &changedID).Error)
			require.NoError(t, s.db.Exec("UPDATE thread_events SET updated_at = ? WHERE id = ?", time.Now().UTC(), changedID).Error)
			for i := 0; i < 3; i++ {
				require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: main.ID, Kind: EvAssistant, Content: "a new reply"}))
			}
			since, _, err := s.PageThreadEvents(main.ID, EventPage{Since: cursor})
			require.NoError(t, err)

			t.Logf("events=%d more=%v", n, more)
			t.Logf("  full transcript (old checklist read) : %8d B  (%d events)", payloadBytes(t, full, false), len(full))
			t.Logf("  first page, limit=200              : %8d B  (%d events)", payloadBytes(t, page, more), len(page))
			t.Logf("  checklist, tools=...               : %8d B  (%d events)", payloadBytes(t, progress, false), len(progress))
			t.Logf("  catch-up, since=cursor             : %8d B  (%d events)", payloadBytes(t, since, false), len(since))

			// The checklist read is a small slice of the transcript, and the
			// catch-up is far smaller than re-reading the page.
			require.Less(t, payloadBytes(t, progress, false), payloadBytes(t, page, more)/4)
			require.Less(t, payloadBytes(t, since, false), payloadBytes(t, page, more)/10)
		})
	}
}
