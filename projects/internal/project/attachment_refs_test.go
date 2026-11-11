package project

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestAttachmentRefsEnforceProjectAndUserIsolation(t *testing.T) {
	f := newFixture(t, nil)
	c := &Coordinator{store: f.store, layout: f.threads.layout}
	a, err := c.layout.SaveAttachment(f.project.ID, "one.txt", strings.NewReader("one"), 0)
	require.NoError(t, err)
	event := &db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Payload: AttachmentsPayload([]Attachment{a})}
	require.NoError(t, f.store.AddThreadEvent(event))
	refs := []AttachmentRef{{MessageID: event.ID, Index: 0}, {MessageID: event.ID, Index: 0}}
	got, err := c.resolveAttachmentRefs(f.project, refs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, a.Path, got[0].Path)
	_, err = c.resolveAttachmentRefs(f.project, []AttachmentRef{{MessageID: event.ID, Index: 1}})
	require.Error(t, err)
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	_, err = c.resolveAttachmentRefs(other, refs)
	require.Error(t, err)
	relay := &db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Source: db.SourceThread, Payload: event.Payload}
	require.NoError(t, f.store.AddThreadEvent(relay))
	_, err = c.resolveAttachmentRefs(f.project, []AttachmentRef{{MessageID: relay.ID, Index: 0}})
	require.Error(t, err)
	path, err := c.layout.LibraryPath(f.project.ID, a.Path)
	require.NoError(t, err)
	require.NoError(t, os.Remove(path))
	outside := t.TempDir() + "/outside"
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0600))
	require.NoError(t, os.Symlink(outside, path))
	_, err = c.resolveAttachmentRefs(f.project, refs)
	require.Error(t, err, "a swapped symlink cannot be delegated")
	require.NoError(t, os.Remove(path))
	_, err = c.resolveAttachmentRefs(f.project, refs)
	require.Error(t, err)
}

func TestAttachmentOnlyHistoryExposesReferences(t *testing.T) {
	f := newFixture(t, nil)
	c := &Coordinator{store: f.store, layout: f.threads.layout}
	e := &db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Payload: AttachmentsPayload([]Attachment{{Name: "only.txt", Path: "uploads/000000000000-only.txt"}})}
	require.NoError(t, f.store.AddThreadEvent(e))
	got, err := c.readConversation(f.project, readArgs{ID: e.ID})
	require.NoError(t, err)
	require.Contains(t, got, "only.txt")
	require.Contains(t, got, "index=0")
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
