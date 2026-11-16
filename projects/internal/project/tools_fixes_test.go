package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n"), res.IsError
}

// A tool that edits the project must not lock the rest of its own turn out,
// while a change from anywhere else still must.
func TestEditingTheProjectDoesNotBlockTheRestOfTheTurn(t *testing.T) {
	f := newToolsFixture(t)
	cs, _ := f.client(t, f.p.MainThreadID)

	out, isErr := callTool(t, cs, "update_instructions", map[string]any{"instructions": "Be brief."})
	require.False(t, isErr, out)
	out, isErr = callTool(t, cs, "read_conversation", map[string]any{})
	assert.False(t, isErr, "the tool's own change is part of the turn now: %s", out)
	out, isErr = callTool(t, cs, "update_instructions", map[string]any{"instructions": "Be briefer."})
	assert.False(t, isErr, out, "and a second edit in the same turn works")

	other, err := f.store.GetProject(f.p.ID)
	require.NoError(t, err)
	other.Goal = "changed by the user"
	require.NoError(t, f.store.SaveProject(other))
	_, isErr = callTool(t, cs, "read_conversation", map[string]any{})
	assert.True(t, isErr, "a change made elsewhere still stops the turn")
}

func TestAFailedEditKeepsTheTurnsView(t *testing.T) {
	f := newToolsFixture(t)
	cs, turn := f.client(t, f.p.MainThreadID)
	_, isErr := callTool(t, cs, "update_instructions", map[string]any{"instructions": strings.Repeat("x", MaxInstructions+1)})
	require.True(t, isErr)
	assert.Empty(t, turn.snapshot.Instructions, "a refused edit changes nothing the turn must adopt")
	_, isErr = callTool(t, cs, "read_conversation", map[string]any{})
	assert.False(t, isErr)
}

func TestListSharedFilesIsSortedAndCappedInThatOrder(t *testing.T) {
	f := newToolsFixture(t)
	dir := f.svc.Layout.LibraryDir(f.p.ID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for i := 0; i < 250; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d.txt", (i*7)%250)), nil, 0o644))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "a-dir"), 0o755))
	cs, _ := f.client(t, f.p.MainThreadID)
	out, isErr := callTool(t, cs, "list_shared_files", map[string]any{"path": "library"})
	require.False(t, isErr, out)
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 201)
	assert.Equal(t, "a-dir/", lines[0])
	assert.Equal(t, "f000.txt", lines[1])
	assert.Equal(t, "f198.txt", lines[199], "the first 200 in name order, not in the file system's order")
	assert.Equal(t, "… more entries omitted", lines[200])
}

func TestMemoryFileNameIsStable(t *testing.T) {
	assert.Equal(t, "release-date.md", MemoryFileName("Release date"))
	assert.Equal(t, "release-date.md", MemoryFileName(" release_date.md "))
	cn := MemoryFileName("发布日期")
	assert.Equal(t, cn, MemoryFileName("发布日期"), "the same topic is the same file")
	assert.NotEqual(t, cn, MemoryFileName("发布时间"), "different topics are different files")
	assert.Regexp(t, `^note-[0-9a-f]{8}\.md$`, cn)
	assert.True(t, memoryNameRe.MatchString(cn))
}

func TestRememberUpdatesAChineseTopicAndRefusesABlankOne(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	require.NoError(t, l.Ensure("p"))
	a, err := l.Remember("p", "发布日期", "周五。", "周五发布")
	require.NoError(t, err)
	b, err := l.Remember("p", "发布日期", "周一。", "周一发布")
	require.NoError(t, err)
	assert.Equal(t, a, b)
	files, err := l.ListMemory("p")
	require.NoError(t, err)
	assert.Len(t, files, 2, "the index and one note, not two notes")
	idx, err := l.ReadMemory("p", MemoryIndex)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(idx, "发布日期"), "one index line")
	_, err = l.Remember("p", "   ", "x", "y")
	assert.Error(t, err)
}
