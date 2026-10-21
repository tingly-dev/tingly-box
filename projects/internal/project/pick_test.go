package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestConnectBindsAChatToAProjectOrAFolder(t *testing.T) {
	f, svc, _ := channelFixture(t)
	empty, err := f.store.OpenFolder(t.TempDir(), "")
	require.NoError(t, err)
	old := folderChat(t, f, svc, FolderThreadSpec{})

	targets, err := svc.ChannelTargets()
	require.NoError(t, err)
	var ids []string
	for _, tg := range targets {
		ids = append(ids, tg.ProjectID)
	}
	assert.Contains(t, ids, f.project.ID)
	assert.Contains(t, ids, old.ProjectID)
	assert.NotContains(t, ids, empty.ID, "a folder without a session has no thread to bind to")

	reply, err := svc.ConnectChannel("telegram", "c", f.project.ID)
	require.NoError(t, err)
	assert.Contains(t, reply, "project conversation of Demo")
	b, err := f.store.GetChannelBinding("telegram", "c")
	require.NoError(t, err)
	assert.Equal(t, f.project.MainThreadID, b.ThreadID)

	// A folder: the chat moves there, and its next message starts a new
	// session rather than continuing the latest one.
	reply, err = svc.ConnectChannel("telegram", "c", old.ProjectID)
	require.NoError(t, err)
	assert.Contains(t, reply, "next message starts a new session")
	b, err = f.store.GetChannelBinding("telegram", "c")
	require.NoError(t, err)
	assert.Equal(t, old.ID, b.ThreadID)
	assert.True(t, b.NewThread)

	p, _ := f.store.GetProject(f.project.ID)
	require.NoError(t, svc.SetArchived(p, true))
	_, err = svc.ConnectChannel("telegram", "c", f.project.ID)
	assert.ErrorIs(t, err, ErrArchived)
	_, err = svc.ConnectChannel("telegram", "c", "nope")
	assert.ErrorIs(t, err, db.ErrNotFound)
}
