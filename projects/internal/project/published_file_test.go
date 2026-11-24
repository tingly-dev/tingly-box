package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestPublishFilePersistsAnAccessibleConversationEvent(t *testing.T) {
	s, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	root := t.TempDir()
	layout := Layout{Root: t.TempDir()}
	p, err := s.OpenFolder(root, "")
	require.NoError(t, err)
	thread := &db.Thread{ProjectID: p.ID, Cwd: root, Title: "Test"}
	require.NoError(t, s.CreateThread(thread))
	name := filepath.Join(root, "diagram.txt")
	require.NoError(t, os.WriteFile(name, []byte("diagram contents"), 0o600))
	svc := &Service{Store: s, Layout: layout, Hub: NewHub()}
	ev, err := svc.PublishFile(context.Background(), thread.ID, "diagram.txt", "Result")
	require.NoError(t, err)
	assert.Equal(t, db.EvFile, ev.Kind)
	assert.Equal(t, "Result", ev.Content)
	atts := ParseAttachments(ev.Payload)
	require.Len(t, atts, 1)
	stored, err := layout.AttachmentPath(p, thread, atts[0].Path)
	require.NoError(t, err)
	got, err := os.ReadFile(stored)
	require.NoError(t, err)
	assert.Equal(t, "diagram contents", string(got))
	events, err := s.ListThreadEvents(thread.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, ev.ID, events[0].ID)
}

func TestPublishFileRejectsPathsOutsideTheThread(t *testing.T) {
	s, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	root := t.TempDir()
	p, err := s.OpenFolder(root, "")
	require.NoError(t, err)
	thread := &db.Thread{ProjectID: p.ID, Cwd: root, Title: "Test"}
	require.NoError(t, s.CreateThread(thread))
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	svc := &Service{Store: s, Layout: Layout{Root: t.TempDir()}, Hub: NewHub()}
	_, err = svc.PublishFile(context.Background(), thread.ID, outside, "")
	require.ErrorContains(t, err, "outside")
	link := filepath.Join(root, "link.txt")
	if os.Symlink(outside, link) == nil {
		_, err = svc.PublishFile(context.Background(), thread.ID, link, "")
		require.ErrorContains(t, err, "outside")
	}
}
