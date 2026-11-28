package project

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestAttachmentRoundTrip(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	require.NoError(t, l.Ensure("p1"))

	a, err := l.SaveAttachment("p1", "../../etc/shot.png", strings.NewReader("png"), 0)
	require.NoError(t, err)
	assert.Equal(t, "shot.png", a.Name)
	assert.Equal(t, "image/png", a.Type)
	assert.Regexp(t, `^uploads/[0-9a-f]{12}-shot\.png$`, a.Path, "one flat file: id, then the name")

	got, err := l.ResolveAttachments("p1", []string{a.Path})
	require.NoError(t, err)
	assert.Equal(t, []Attachment{a}, got)

	note := AttachmentNote(l.LibraryDir("p1"), got)
	assert.Contains(t, note, filepath.Join(l.LibraryDir("p1"), filepath.FromSlash(a.Path)))
	assert.Equal(t, got, ParseAttachments(AttachmentsPayload(got)))

	// Removing the file turns the note into "removed", not an error.
	require.NoError(t, os.Remove(filepath.Join(l.LibraryDir("p1"), a.Path)))
	assert.Contains(t, AttachmentNote(l.LibraryDir("p1"), got), "shot.png (removed)")
}

func TestAttachmentRefusals(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	require.NoError(t, l.Ensure("p1"))
	require.NoError(t, os.WriteFile(filepath.Join(l.LibraryDir("p1"), "secret.txt"), []byte("x"), 0o644))

	_, err := l.ResolveAttachments("p1", []string{"secret.txt"})
	assert.Error(t, err, "only uploads/ can be attached")
	_, err = l.ResolveAttachments("p1", []string{"uploads/../secret.txt"})
	assert.Error(t, err)
	_, err = l.ResolveAttachments("p1", []string{"uploads/000000000000-x.txt"})
	assert.Error(t, err)
	_, err = l.ResolveAttachments("p1", []string{"uploads/sub/000000000000-x.txt"})
	assert.Error(t, err, "uploads are flat")

	_, err = l.SaveAttachment("p1", "big.bin", strings.NewReader("12345"), 4)
	assert.Error(t, err)
	entries, _ := os.ReadDir(filepath.Join(l.LibraryDir("p1"), UploadsDir))
	assert.Empty(t, entries, "a refused upload leaves nothing behind")
}

func TestRenderHistoryListsAttachments(t *testing.T) {
	dir := t.TempDir()
	msgs := renderHistory([]db.ThreadEvent{{
		Kind: db.EvUser, Content: "look",
		Payload: AttachmentsPayload([]Attachment{{Name: "a.txt", Path: "uploads/aaaaaaaaaaaa-a.txt", Type: "text/plain", Size: 1}}),
	}}, dir)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content, "a.txt (removed)")
}

func TestLongUploadNameIsCut(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	a, err := l.SaveAttachment("p1", strings.Repeat("长", 200)+".txt", strings.NewReader("x"), 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(filepath.Base(a.Path)), 255)
	assert.True(t, strings.HasSuffix(a.Name, ".txt"))
	_, err = l.ResolveAttachments("p1", []string{a.Path})
	assert.NoError(t, err)
}

func TestDownloadNameDropsTheUploadIDOnly(t *testing.T) {
	for rel, want := range map[string]string{
		"uploads/0123456789ab-report final.pdf": "report final.pdf",
		"uploads/0123456789ab-a-b.txt":          "a-b.txt",
		"uploads/notes.md":                      "notes.md",
		"uploads/sub/0123456789ab-x.txt":        "0123456789ab-x.txt",
		"0123456789ab-x.txt":                    "0123456789ab-x.txt",
		"docs/readme.md":                        "readme.md",
	} {
		assert.Equal(t, want, DownloadName(rel), rel)
	}
}

// The size check is inclusive at the limit and refuses exactly one byte over
// (TooLargeError, which the HTTP layer maps to 413), so the web client's
// precheck — which uses the same boundary — agrees with the server.
func TestSaveAttachmentLimitBoundary(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	require.NoError(t, l.Ensure("p1"))

	a, err := l.SaveAttachment("p1", "edge.bin", strings.NewReader(strings.Repeat("x", 8)), 8)
	require.NoError(t, err, "a file exactly at the limit is kept")
	assert.EqualValues(t, 8, a.Size)

	_, err = l.SaveAttachment("p1", "over.bin", strings.NewReader(strings.Repeat("x", 9)), 8)
	var tooLarge *TooLargeError
	require.ErrorAs(t, err, &tooLarge)
	assert.Equal(t, "over.bin", tooLarge.Name)
	assert.EqualValues(t, 8, tooLarge.Limit)
}

// The web client refuses an oversized file before uploading it, using a copy
// of MaxUploadBytes (it cannot read Go constants at runtime). Keep the copy
// equal: a drift means the UI accepts a file the server then rejects with 413,
// which is exactly what the precheck exists to prevent.
func TestWebClientUploadLimitMatchesTheServer(t *testing.T) {
	path := filepath.Join("..", "..", "..", "frontend", "packages", "projects", "src", "utils", "uploadLimit.ts")
	src, err := os.ReadFile(path)
	require.NoError(t, err, "the web client's upload limit file moved; update this test's path")

	m := regexp.MustCompile(`MAX_UPLOAD_BYTES\s*=\s*([0-9]+)\s*<<\s*([0-9]+)`).FindSubmatch(src)
	require.NotNil(t, m, "uploadLimit.ts must define MAX_UPLOAD_BYTES as a << b")

	shift, err := strconv.Atoi(string(m[2]))
	require.NoError(t, err)
	base, err := strconv.Atoi(string(m[1]))
	require.NoError(t, err)
	assert.Equal(t, MaxUploadBytes, int64(base)<<shift, "the web client's MAX_UPLOAD_BYTES must match the server's MaxUploadBytes")
}
