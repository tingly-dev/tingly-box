package project

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestAttachmentOnlyHistoryShowsPersistentPath(t *testing.T) {
	f := newFixture(t, nil)
	c := &Coordinator{store: f.store, layout: f.threads.layout}
	a, err := c.layout.SaveAttachment(f.project.ID, "only.txt", strings.NewReader("actual"), 0)
	require.NoError(t, err)
	e := &db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Payload: AttachmentsPayload([]Attachment{a})}
	require.NoError(t, f.store.AddThreadEvent(e))
	path, err := c.layout.LibraryPath(f.project.ID, a.Path)
	require.NoError(t, err)
	got, err := c.readConversation(f.project, readArgs{ID: e.ID})
	require.NoError(t, err)
	require.Contains(t, got, path)
	require.NoError(t, os.Remove(path))
	got, err = c.readConversation(f.project, readArgs{ID: e.ID})
	require.NoError(t, err)
	require.Contains(t, got, "only.txt (removed)")
}

func TestFolderLegacyAttachmentPromptIsThreadScoped(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "start"})
	require.NoError(t, err)
	p, err := f.store.GetProject(s.ProjectID)
	require.NoError(t, err)
	a, err := f.threads.layout.SaveAttachment(SessionOwner(s.ID), "legacy.txt", strings.NewReader("old"), 0)
	require.NoError(t, err)
	path, err := f.threads.layout.LibraryPath(SessionOwner(s.ID), a.Path)
	require.NoError(t, err)
	prompt := f.threads.threadPrompt(queued{text: "read", atts: []Attachment{a}}, p, s)
	require.Contains(t, prompt, path)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "old", string(b))
	other := *s
	other.ID = "other"
	require.NotContains(t, f.threads.threadPrompt(queued{atts: []Attachment{a}}, p, &other), path)
}
