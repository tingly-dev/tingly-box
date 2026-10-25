package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

func (c client) upload(path, name, body string) (int, []project.Attachment) {
	c.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("files", name)
	require.NoError(c.t, err)
	_, _ = fw.Write([]byte(body))
	require.NoError(c.t, w.Close())
	resp, err := http.Post(c.base+path, w.FormDataContentType(), &buf)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	var out project.AttachmentsResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Attachments
}

func (c client) fetch(path string) (int, string) {
	c.t.Helper()
	resp, err := http.Get(c.base + path)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Two threads of one folder see the same uploads, and a thread from before
// uploads were shared per folder still opens its own old files.
func TestFolderThreadsShareUploadsOverHTTP(t *testing.T) {
	a := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil }}
	root := t.TempDir()
	_, c := newServerAt(t, &project.Agents{Service: a.Service()}, root)
	var f db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &f))

	// Uploaded before any thread exists, used by the first message.
	code, atts := c.upload("/projects/"+f.ID+"/attachments", "log.txt", "boom")
	require.Equal(t, 201, code)
	require.Len(t, atts, 1)
	var s1, s2 db.Thread
	require.Equal(t, 201, c.do("POST", "/projects/"+f.ID+"/threads", map[string]any{"content": "look", "attachments": []string{atts[0].Path}}, &s1))
	require.Equal(t, 201, c.do("POST", "/projects/"+f.ID+"/threads", map[string]any{"content": "me too"}, &s2))

	// Both threads open it.
	for _, id := range []string{s1.ID, s2.ID} {
		code, body := c.fetch("/threads/" + id + "/attachments/file?path=" + url.QueryEscape(atts[0].Path))
		assert.Equal(t, 200, code)
		assert.Equal(t, "boom", body)
	}

	// Uploaded through the second thread, the first one sees it too.
	code, more := c.upload("/threads/"+s2.ID+"/attachments", "b.txt", "bee")
	require.Equal(t, 201, code)
	code, body := c.fetch("/threads/" + s1.ID + "/attachments/file?path=" + url.QueryEscape(more[0].Path))
	assert.Equal(t, 200, code)
	assert.Equal(t, "bee", body)

	// A file only in a thread's legacy area is still served to that thread,
	// and not to another one.
	legacy, err := project.Layout{Root: root}.SaveAttachment(project.SessionOwner(s1.ID), "old.txt", bytes.NewReader([]byte("old")), 0)
	require.NoError(t, err)
	code, body = c.fetch("/threads/" + s1.ID + "/attachments/file?path=" + url.QueryEscape(legacy.Path))
	assert.Equal(t, 200, code)
	assert.Equal(t, "old", body)
	code, _ = c.fetch("/threads/" + s2.ID + "/attachments/file?path=" + url.QueryEscape(legacy.Path))
	assert.NotEqual(t, 200, code)
	_, err = os.Stat(filepath.Join(root, f.ID, "library", "uploads"))
	assert.NoError(t, err, "uploads live in the folder project's library")
}
