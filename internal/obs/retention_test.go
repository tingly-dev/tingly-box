package obs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path string, size int, mod time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o644))
	require.NoError(t, os.Chtimes(path, mod, mod))
}

func TestPruneRecordDir_MaxAge(t *testing.T) {
	base := t.TempDir()
	oldDay := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	newDay := time.Now().Format("2006-01-02")
	oldPath := filepath.Join(base, "scenario", "sessions", oldDay, "sess.jsonl.gz")
	newPath := filepath.Join(base, "scenario", "sessions", newDay, "sess.jsonl.gz")
	writeFile(t, oldPath, 10, time.Now().AddDate(0, 0, -30))
	writeFile(t, newPath, 10, time.Now())

	removed, err := PruneRecordDir(base, RetentionPolicy{MaxAge: 7 * 24 * time.Hour, MaxBytes: -1})
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	_, statErr := os.Stat(oldPath)
	assert.True(t, os.IsNotExist(statErr), "old session file must be removed")
	_, statErr = os.Stat(newPath)
	assert.NoError(t, statErr, "recent session file must survive")
	_, statErr = os.Stat(filepath.Dir(oldPath))
	assert.True(t, os.IsNotExist(statErr), "empty date directory is removed")
}

func TestPruneRecordDir_MaxBytes(t *testing.T) {
	base := t.TempDir()
	day := time.Now().Format("2006-01-02")
	oldest := filepath.Join(base, "s", "sessions", day, "a.jsonl.gz")
	middle := filepath.Join(base, "s", "sessions", day, "b.jsonl.gz")
	newest := filepath.Join(base, "s", "sessions", day, "c.jsonl.gz")
	writeFile(t, oldest, 400, time.Now().Add(-3*time.Hour))
	writeFile(t, middle, 400, time.Now().Add(-2*time.Hour))
	writeFile(t, newest, 400, time.Now().Add(-time.Hour))

	// Budget fits ~1.5 files: the two oldest go.
	removed, err := PruneRecordDir(base, RetentionPolicy{MaxAge: -1, MaxBytes: 600})
	require.NoError(t, err)
	assert.Equal(t, 2, removed)
	_, err = os.Stat(oldest)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(middle)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(newest)
	assert.NoError(t, err, "newest file survives size pruning")
}

func TestPruneRecordDir_MissingDir(t *testing.T) {
	removed, err := PruneRecordDir(filepath.Join(t.TempDir(), "nope"), DefaultRetention())
	require.NoError(t, err)
	assert.Zero(t, removed)
}

func TestPruneRecordDir_LeavesNonLogFiles(t *testing.T) {
	base := t.TempDir()
	day := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	blob := filepath.Join(base, "blobs", "ab", "cd", "deadbeef.json")
	writeFile(t, blob, 10, time.Now().AddDate(0, 0, -30))
	log := filepath.Join(base, "s", "sessions", day, "sess.jsonl.gz")
	writeFile(t, log, 10, time.Now().AddDate(0, 0, -30))

	_, err := PruneRecordDir(base, RetentionPolicy{MaxAge: 7 * 24 * time.Hour, MaxBytes: -1})
	require.NoError(t, err)
	_, err = os.Stat(blob)
	assert.NoError(t, err, "content-addressed blobs are not touched by session retention")
	_, err = os.Stat(log)
	assert.True(t, os.IsNotExist(err))
}
