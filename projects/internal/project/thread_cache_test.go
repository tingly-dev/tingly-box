package project

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSharedCachesStayOutOfTheLibrary walks the library surface that matters
// for a thread-written dot directory: the flat list, the tree, a direct open
// and the zip download. Unlike the scratch subtree (.tmp), the cache is never
// listed or served; it is only accounted for by CacheBytes.
func TestSharedCachesStayOutOfTheLibrary(t *testing.T) {
	l, _ := seedLibrary(t) // seeds .cache/go-mod/cache.bin beside .tmp/t1/scratch

	files, err := l.ListLibrary("p")
	require.NoError(t, err)
	for _, f := range files {
		assert.False(t, strings.HasPrefix(f.Path, ".cache"), "the library must not list the cache: %s", f.Path)
	}

	root, err := l.ListLibraryDir("p", "")
	require.NoError(t, err)
	assert.NotContains(t, names(root), ".cache")
	_, err = l.ListLibraryDir("p", ".cache")
	assert.ErrorIs(t, err, ErrInvalidLibraryPath, "the cache cannot be opened even when named directly")

	_, _, err = l.OpenLibraryFile("p", ".cache/go-mod/cache.bin")
	assert.ErrorIs(t, err, ErrInvalidLibraryPath)

	for name := range archiveOf(t, l, "") {
		assert.False(t, strings.HasPrefix(name, ".cache"), "the root zip must not carry the cache: %s", name)
	}

	assert.Greater(t, l.CacheBytes("p"), int64(0))
	assert.Equal(t, int64(0), l.CacheBytes("no-such-project"))
}
