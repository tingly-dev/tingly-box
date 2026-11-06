package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func TestLibraryFileRefusesEncodedAndOddPaths(t *testing.T) {
	root := t.TempDir()
	_, c := newServerAt(t, nil, root)
	var p db.Project
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "odd"}, &p))
	lib := project.Layout{Root: root}.LibraryDir(p.ID)
	require.NoError(t, os.MkdirAll(lib, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "a.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(lib), "outside.md"), []byte("OUT"), 0o644))

	get := func(rawQuery string) int {
		code, body := c.fetch("/projects/" + p.ID + "/library/file?path=" + rawQuery)
		assert.NotContains(t, body, "OUT", rawQuery)
		return code
	}
	// Decoded once by the server: ../outside.md in various spellings.
	for _, q := range []string{"..%2Foutside.md", "%2e%2e/outside.md", "%2e%2e%2foutside.md", "%2E%2E%2Foutside.md", "..%5Coutside.md", "a.md%00", "%00", "%2Fetc%2Fpasswd%00", ".%2Foutside.md", "..%252foutside.md"} {
		assert.Contains(t, []int{400, 404}, get(q), q)
	}
	// Refused as invalid, specifically.
	for _, q := range []string{"..%2Foutside.md", "%2e%2e%2foutside.md", "a.md%00", ".%2Fa.md"} {
		assert.Equal(t, 400, get(q), q)
	}
	// Absolute paths stay inside the library, so there is nothing there.
	assert.Equal(t, 404, get("%2Fetc%2Fpasswd"))
	// A.MD is a.md only where the file system ignores case (macOS by default).
	want := http.StatusNotFound
	if _, err := os.Stat(filepath.Join(lib, "A.MD")); err == nil {
		want = http.StatusOK
	}
	assert.Equal(t, want, get("A.MD"))
}

func TestLibraryDownloadServesTheOpenedFileAsAnAttachment(t *testing.T) {
	root := t.TempDir()
	_, c := newServerAt(t, nil, root)
	var p db.Project
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "dl"}, &p))
	lib := project.Layout{Root: root}.LibraryDir(p.ID)
	require.NoError(t, os.MkdirAll(lib, 0o755))
	for name, body := range map[string]string{"page.html": "<script>alert(1)</script>", "index.html": "<h1>i</h1>", "报告.txt": "0123456789"} {
		require.NoError(t, os.WriteFile(filepath.Join(lib, name), []byte(body), 0o644))
	}

	do := func(name string, hdr map[string]string) *http.Response {
		req, err := http.NewRequest("GET", c.base+"/projects/"+p.ID+"/library/file?path="+url.QueryEscape(name), nil)
		require.NoError(t, err)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	resp := do("page.html", nil)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;"), resp.Header.Get("Content-Disposition"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))

	// A file named index.html is a file, not a redirect to its folder.
	resp = do("index.html", nil)
	assert.Equal(t, 200, resp.StatusCode)
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "<h1>i</h1>", string(b))

	resp = do("报告.txt", map[string]string{"Range": "bytes=2-4"})
	assert.Equal(t, 206, resp.StatusCode)
	b, _ = io.ReadAll(resp.Body)
	assert.Equal(t, "234", string(b))
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	require.NoError(t, err)
	assert.Equal(t, "报告.txt", params["filename"])
}

func TestUploadsNeverFollowLinksOutOfTheLibrary(t *testing.T) {
	root := t.TempDir()
	_, c := newServerAt(t, nil, root)
	var p db.Project
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "up"}, &p))
	lib := project.Layout{Root: root}.LibraryDir(p.ID)
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(lib, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "victim"), []byte("orig"), 0o644))
	// A thread left links in the library.
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "uploads")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "victim"), filepath.Join(lib, "victim")))

	code, _ := c.upload("/projects/"+p.ID+"/library", "victim", "pwned")
	assert.Equal(t, 400, code)
	code, _ = c.upload("/projects/"+p.ID+"/library", ".env", "x")
	assert.Equal(t, 400, code)
	code, _ = c.upload("/projects/"+p.ID+"/attachments", "shot.png", "pwned")
	assert.GreaterOrEqual(t, code, 400)

	entries, _ := os.ReadDir(outside)
	assert.Len(t, entries, 1, "nothing was created outside the library")
	b, _ := os.ReadFile(filepath.Join(outside, "victim"))
	assert.Equal(t, "orig", string(b))
}

func TestLibraryArchiveStreamsAFolderAsAZip(t *testing.T) {
	root := t.TempDir()
	_, c := newServerAt(t, nil, root)
	var p db.Project
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "zip"}, &p))
	lib := project.Layout{Root: root}.LibraryDir(p.ID)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("SECRET"), 0o644))
	for f, body := range map[string]string{"outputs/run 1/report.md": "# r", "outputs/run 1/data/rows.csv": "a,b", "outputs/run 1/.hidden": "no", "outputs/other.txt": "o", ".tmp/t/x": "no", "a.md": "a"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(lib, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lib, f), []byte(body), 0o644))
	}
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(lib, "outputs/run 1/leak.txt")))

	archive := func(rel string) (*http.Response, []byte) {
		resp, err := http.Get(c.base + "/projects/" + p.ID + "/library/archive?path=" + url.QueryEscape(rel))
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp, body
	}

	resp, body := archive("outputs/run 1")
	require.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/zip", resp.Header.Get("Content-Type"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	disp, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	require.NoError(t, err)
	assert.Equal(t, "attachment", disp)
	assert.Equal(t, "run 1.zip", params["filename"])
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	got := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[f.Name] = string(b)
	}
	assert.Equal(t, map[string]string{"report.md": "# r", "data/rows.csv": "a,b"}, got)

	// The root downloads too, when it fits.
	resp, body = archive("")
	require.Equal(t, 200, resp.StatusCode)
	_, params, _ = mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	assert.Equal(t, "library.zip", params["filename"])
	zr, err = zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	assert.Len(t, zr.File, 4)
	assert.NotContains(t, string(body), "SECRET")

	// Refused before anything is streamed.
	for _, bad := range []string{"..", "outputs/../..", ".tmp", ".tmp/t", "a.md"} {
		resp, body := archive(bad)
		assert.Equal(t, 400, resp.StatusCode, bad)
		assert.Contains(t, string(body), `"error"`, bad)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"), bad)
	}
	// Nothing to download is said, not sent as an empty zip: a folder of only
	// empty folders, hidden entries and links.
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "hollow/inner"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "hollow/.git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "hollow/.git/x"), []byte("x"), 0o644))
	resp, body = archive("hollow")
	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, string(body), "no files to download")

	resp, _ = archive("nope")
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = archive("/etc")
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, 404, c.do("GET", "/projects/missing/library/archive", nil, nil))
}

func TestLibraryArchiveOverTheLimitsIs413WithTheReason(t *testing.T) {
	root := t.TempDir()
	_, c := newServerAt(t, nil, root)
	var p db.Project
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "big"}, &p))
	lib := project.Layout{Root: root}.LibraryDir(p.ID)
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "many"), 0o755))
	for i := 0; i <= project.MaxArchiveFiles; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(lib, "many", fmt.Sprintf("f%03d", i)), []byte("x"), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "fat"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "fat/f.bin"), make([]byte, project.MaxArchiveBytes+1), 0o644))

	var e struct {
		Error string `json:"error"`
	}
	require.Equal(t, http.StatusRequestEntityTooLarge, c.do("GET", "/projects/"+p.ID+"/library/archive?path=many", nil, &e))
	assert.Contains(t, e.Error, "100 files")
	require.Equal(t, http.StatusRequestEntityTooLarge, c.do("GET", "/projects/"+p.ID+"/library/archive?path=fat", nil, &e))
	assert.Contains(t, e.Error, "10 MB")
}
