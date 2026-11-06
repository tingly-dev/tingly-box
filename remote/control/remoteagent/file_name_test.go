package remoteagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeFileName(t *testing.T) {
	long := strings.Repeat("长", 100) + ".docx"
	cases := map[string]string{
		"report.pdf":          "report.pdf",
		"季度 报告 (final).pdf":   "季度 报告 (final).pdf",
		"../../etc/passwd":    "passwd",
		`C:\Users\me\a b.txt`: "a b.txt",
		"a\x00b\x1f\n.txt":    "ab.txt",
		"  .hidden  ":         "hidden",
		"..":                  "",
		"/":                   "",
		"":                    "",
		"a/b/":                "",
		"bad\xff\xfeutf8.txt": "badutf8.txt",
	}
	for in, want := range cases {
		assert.Equal(t, want, SanitizeFileName(in), "%q", in)
	}
	got := SanitizeFileName(long)
	assert.LessOrEqual(t, len(got), maxFileNameBytes)
	assert.True(t, strings.HasSuffix(got, ".docx"), "the extension survives the length cap: %q", got)
}

func TestChooseFileName(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 5, 7, 0, time.UTC)
	png := append([]byte("\x89PNG\r\n\x1a\n"), 0, 0)
	assert.Equal(t, "image-20261106-090507.png", chooseFileName("", "", "image/png", png, now))
	assert.Equal(t, "image-20261106-090507.png", chooseFileName("", "", "application/octet-stream", png, now), "bytes win over a vague declared type")
	assert.Equal(t, "file-20261106-090507.pdf", chooseFileName("", "", "application/pdf", []byte("%PDF-1.7"), now))
	assert.Equal(t, "a.pdf", chooseFileName("a.pdf", `attachment; filename="b.pdf"`, "application/pdf", nil, now), "the message's name beats the header")
	assert.Equal(t, "名字.pdf", chooseFileName("", "attachment; filename*=UTF-8''%E5%90%8D%E5%AD%97.pdf", "application/pdf", nil, now))
	assert.Equal(t, "notes.txt", chooseFileName("notes", "", "text/plain", []byte("hello"), now))
	assert.Equal(t, "data.v2", chooseFileName("data.v2", "", "text/csv", []byte("a,b"), now), "an existing extension is left alone")
}

func TestDownloadFile_SameNameDoesNotOverwrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := NewFileStore()
	a, err := s.DownloadFile(context.Background(), dir, srv.URL+"/one", "text/plain", "notes.txt")
	assert.NoError(t, err)
	b, err := s.DownloadFile(context.Background(), dir, srv.URL+"/two", "text/plain", "notes.txt")
	assert.NoError(t, err)
	assert.Equal(t, "notes.txt", a.Filename)
	assert.Equal(t, "notes (2).txt", b.Filename)
	got, _ := os.ReadFile(a.Path)
	assert.Equal(t, "/one", string(got))
	entries, _ := os.ReadDir(filepath.Join(dir, ".agent"))
	assert.Len(t, entries, 2, "no scratch file is left behind")
}
