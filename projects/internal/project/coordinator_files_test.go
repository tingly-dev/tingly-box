package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorSharedFilesStayWithinProject(t *testing.T) {
	layout := Layout{Root: t.TempDir()}
	require.NoError(t, layout.Ensure("one"))
	require.NoError(t, layout.Ensure("two"))
	file := filepath.Join(layout.LibraryDir("one"), "input.txt")
	require.NoError(t, os.WriteFile(file, []byte("shared"), 0o600))
	secret := filepath.Join(layout.LibraryDir("two"), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("other project"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(layout.LibraryDir("one"), "escape")))
	c := &Coordinator{layout: layout}
	content, err := c.sharedFile("one", file, false)
	require.NoError(t, err)
	require.Equal(t, "shared", content[0].(*mcp.TextContent).Text)
	_, err = c.sharedFile("one", "library/escape", false)
	require.Error(t, err)
	_, err = c.sharedFile("one", "library/../memory/../../two/library/secret.txt", false)
	require.Error(t, err)
	_, err = c.sharedFile("one", secret, false)
	require.Error(t, err)
	content, err = c.sharedFile("one", "memory/MEMORY.md", false)
	require.NoError(t, err)
	require.Contains(t, content[0].(*mcp.TextContent).Text, "Project memory")
	content, err = c.sharedFile("one", "library", true)
	require.NoError(t, err)
	require.Contains(t, content[0].(*mcp.TextContent).Text, "input.txt")
	// The shared tool cache is not project content: not listed, not readable.
	require.NoError(t, os.MkdirAll(filepath.Join(layout.CacheDir("one"), "go-mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(layout.CacheDir("one"), "go-mod", "cache.bin"), []byte("cache"), 0o644))
	content, err = c.sharedFile("one", "library", true)
	require.NoError(t, err)
	require.NotContains(t, content[0].(*mcp.TextContent).Text, cacheDirName)
	_, err = c.sharedFile("one", "library/.cache/go-mod/cache.bin", false)
	require.ErrorContains(t, err, "not project content")
}
