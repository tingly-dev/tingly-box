package project

import (
	"context"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestLocalTranscript(t *testing.T) {
	data := []byte(`{"type":"user","timestamp":"2026-01-01T00:00:00Z","message":{"content":"hello"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"answer"},{"type":"tool_use","id":"call-1","name":"Read","input":{"file_path":"a"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"call-1","content":"file contents"}]}}
{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"hidden"}]}}
`)
	events, title, err := localTranscript(data)
	require.NoError(t, err)
	require.Equal(t, "hello", title)
	require.Len(t, events, 4)
	require.Equal(t, []string{db.EvUser, db.EvAssistant, db.EvToolUse, db.EvToolResult}, []string{events[0].Kind, events[1].Kind, events[2].Kind, events[3].Kind})
	require.Equal(t, "call-1", events[3].RequestID)
}

func TestLoadLocalSessionResumesAndReusesThread(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	folder := t.TempDir()
	id := "7a0f2838-e090-4ef1-a358-ddbdaef7042b"
	historyDir := filepath.Join(home, ".claude", "projects", "-"+strings.ReplaceAll(strings.TrimPrefix(folder, "/"), "/", "-"))
	require.NoError(t, os.MkdirAll(historyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(historyDir, id+".jsonl"), []byte(`{"type":"user","sessionId":"7a0f2838-e090-4ef1-a358-ddbdaef7042b","timestamp":"2026-01-01T00:00:00Z","message":{"content":"hello"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"answer"}]}}
`), 0644))
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("continued", 1, 1); return nil })
	p, err := f.store.OpenFolder(folder, "")
	require.NoError(t, err)
	sessions, err := f.threads.LocalSessions(context.Background(), p.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	thread, err := f.threads.LoadLocalSession(context.Background(), p.ID, id)
	require.NoError(t, err)
	events, err := f.store.ListThreadEvents(thread.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	again, err := f.threads.LoadLocalSession(context.Background(), p.ID, id)
	require.NoError(t, err)
	require.Equal(t, thread.ID, again.ID)
	_, err = f.threads.Send(context.Background(), thread.ID, "continue", Source{}, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(f.agent.Launches()) > 0 }, 5*time.Second, 10*time.Millisecond)
	require.True(t, f.agent.Launches()[0].Resume)
	waitStatus(t, f.store, thread.ID, db.ThreadIdle)
}
