package server

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

type treeResponse struct {
	Dir     string                 `json:"dir"`
	Path    string                 `json:"path"`
	Entries []project.LibraryEntry `json:"entries"`
}

func TestLibraryTreeListsOneLevelAndRefusesEscapes(t *testing.T) {
	root := t.TempDir()
	_, c := newServerAt(t, nil, root)
	var p db.Project
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "tree"}, &p))
	lib := project.Layout{Root: root}.LibraryDir(p.ID)
	for _, f := range []string{"a.md", "outputs/run/report.md", ".tmp/t/x"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(lib, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lib, f), []byte("x"), 0o644))
	}

	var top treeResponse
	require.Equal(t, 200, c.do("GET", "/projects/"+p.ID+"/library/tree", nil, &top))
	assert.Equal(t, lib, top.Dir)
	require.Len(t, top.Entries, 2)
	assert.Equal(t, "outputs", top.Entries[0].Name)
	assert.Equal(t, "dir", top.Entries[0].Kind)
	assert.Equal(t, "a.md", top.Entries[1].Name)

	var sub treeResponse
	require.Equal(t, 200, c.do("GET", "/projects/"+p.ID+"/library/tree?path="+url.QueryEscape("outputs/run"), nil, &sub))
	assert.Equal(t, "outputs/run", sub.Path)
	require.Len(t, sub.Entries, 1)
	assert.Equal(t, "outputs/run/report.md", sub.Entries[0].Path)

	for _, bad := range []string{"..", "outputs/../..", ".tmp", ".tmp/t"} {
		assert.Equal(t, 400, c.do("GET", "/projects/"+p.ID+"/library/tree?path="+url.QueryEscape(bad), nil, nil), bad)
	}
	// A leading slash is not absolute: it stays inside the library.
	assert.Equal(t, 404, c.do("GET", "/projects/"+p.ID+"/library/tree?path=%2Fetc", nil, nil))
	assert.Equal(t, 404, c.do("GET", "/projects/"+p.ID+"/library/tree?path=nope", nil, nil))
	assert.Equal(t, 400, c.do("GET", "/projects/"+p.ID+"/library/tree?path=a.md", nil, nil))
	assert.Equal(t, 404, c.do("GET", "/projects/missing/library/tree", nil, nil))
}

func TestLibraryFileDownloadAndDeleteApplyTheSameChecksAsTheTree(t *testing.T) {
	root := t.TempDir()
	_, c := newServerAt(t, nil, root)
	var p db.Project
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "files"}, &p))
	lib := project.Layout{Root: root}.LibraryDir(p.ID)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644))
	for _, f := range []string{"a.md", "outputs/run/report.md", ".tmp/t/x"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(lib, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lib, f), []byte("x"), 0o644))
	}
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "linkdir")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(lib, "leak.txt")))

	file := func(rel string) string { return "/projects/" + p.ID + "/library/file?path=" + url.QueryEscape(rel) }
	assert.Equal(t, 200, c.do("GET", file("a.md"), nil, nil))
	assert.Equal(t, 200, c.do("GET", file("outputs/run/report.md"), nil, nil))
	for _, bad := range []string{"..", "../x", "outputs/../../x", ".tmp/t/x", "linkdir/secret", "leak.txt"} {
		assert.Equal(t, 400, c.do("GET", file(bad), nil, nil), "GET "+bad)
		assert.Equal(t, 400, c.do("DELETE", file(bad), nil, nil), "DELETE "+bad)
	}
	assert.Equal(t, 404, c.do("GET", file("nope"), nil, nil))
	assert.Equal(t, 404, c.do("GET", file("outputs"), nil, nil))
	assert.Equal(t, 404, c.do("DELETE", file("outputs"), nil, nil))

	// Nothing refused was touched.
	assert.FileExists(t, filepath.Join(lib, ".tmp/t/x"))
	assert.FileExists(t, filepath.Join(outside, "secret"))
	assert.DirExists(t, filepath.Join(lib, "outputs"))

	assert.Equal(t, 200, c.do("DELETE", file("a.md"), nil, nil))
	assert.NoFileExists(t, filepath.Join(lib, "a.md"))
}
