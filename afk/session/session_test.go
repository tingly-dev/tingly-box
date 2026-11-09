package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userMsg(text string) anthropic.BetaMessageParam {
	return anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(text))
}

// The SDK ships no assistant-message constructor (assistant turns normally come
// from Message.ToParam()), so tests build one directly.
func assistantMsg(text string) anthropic.BetaMessageParam {
	return anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(text)},
	}
}

func TestOpen_MissingFileIsEmptySession(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "none.jsonl"))
	require.NoError(t, err, "a chat's first turn has no history to read")
	assert.Equal(t, 0, s.Len())
	assert.Empty(t, s.Messages())
}

func TestAppend_RoundTripsThroughReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")

	s, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Append(userMsg("hello"), assistantMsg("hi there")))

	reopened, err := Open(path)
	require.NoError(t, err)
	require.Equal(t, 2, reopened.Len())

	msgs := reopened.Messages()
	assert.Equal(t, anthropic.BetaMessageParamRoleUser, msgs[0].Role)
	assert.Equal(t, "hello", msgs[0].Content[0].OfText.Text)
	assert.Equal(t, anthropic.BetaMessageParamRoleAssistant, msgs[1].Role)
	assert.Equal(t, "hi there", msgs[1].Content[0].OfText.Text)
}

// The format is one entry per line — that is what makes an append incapable of
// corrupting the entries before it.
func TestAppend_WritesOneLinePerEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	s, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Append(userMsg("a"), assistantMsg("b"), userMsg("c")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 3)
	for _, line := range lines {
		var e Entry
		require.NoError(t, json.Unmarshal([]byte(line), &e), "each line must stand alone as JSON")
		assert.Equal(t, EntryMessage, e.Type)
	}
}

// Appending must not rewrite what is already on disk: the earlier bytes stay
// byte-identical, which is the durability property the old store lacked.
func TestAppend_DoesNotRewriteEarlierBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	s, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Append(userMsg("first")))

	before, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, s.Append(assistantMsg("second")))
	after, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, string(before), string(after[:len(before)]),
		"the existing prefix must be untouched by a later append")
	assert.Greater(t, len(after), len(before))
}

// A nil *Session is what a disabled store yields, and it reaches the harness
// inside an afk.Log interface where it is not == nil. Append absorbs it so no
// consumer needs its own typed-nil guard.
func TestAppend_NilSessionIsNoop(t *testing.T) {
	var s *Session
	assert.NoError(t, s.Append(userMsg("dropped")))
}

// One bad line costs one message, not the conversation. The whole-file store
// treated any parse failure as "no history at all".
func TestOpen_SkipsCorruptLineAndKeepsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	s, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Append(userMsg("before")))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString("{not valid json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	s2, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s2.Append(assistantMsg("after")))

	reopened, err := Open(path)
	require.NoError(t, err)
	msgs := reopened.Messages()
	require.Len(t, msgs, 2, "the readable entries on both sides of the bad line survive")
	assert.Equal(t, "before", msgs[0].Content[0].OfText.Text)
	assert.Equal(t, "after", msgs[1].Content[0].OfText.Text)
}

// Turns carrying tool output blow past bufio.Scanner's 64KB default line size;
// without a larger buffer the scan stops and the rest of the file vanishes.
func TestOpen_HandlesVeryLongLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	s, err := Open(path)
	require.NoError(t, err)

	huge := strings.Repeat("x", 300*1024)
	require.NoError(t, s.Append(userMsg(huge), assistantMsg("after the big one")))

	reopened, err := Open(path)
	require.NoError(t, err)
	msgs := reopened.Messages()
	require.Len(t, msgs, 2, "a large turn must not truncate the scan")
	assert.Equal(t, huge, msgs[0].Content[0].OfText.Text)
	assert.Equal(t, "after the big one", msgs[1].Content[0].OfText.Text)
}

func TestImportLegacy_SeedsFromWholeFileArray(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "chat-smartguide.json")
	jsonlPath := filepath.Join(dir, "chat-smartguide.jsonl")

	old := []anthropic.BetaMessageParam{userMsg("old question"), assistantMsg("old answer")}
	data, err := json.Marshal(old)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(legacy, data, 0o600))

	s, err := Open(jsonlPath)
	require.NoError(t, err)
	require.NoError(t, s.ImportLegacy(legacy))

	assert.Equal(t, 2, s.Len(), "upgrading must not drop an existing conversation")
	reopened, err := Open(jsonlPath)
	require.NoError(t, err)
	assert.Equal(t, 2, reopened.Len())
	assert.FileExists(t, legacy, "the old file is left as a backup, not deleted")
}

func TestImportLegacy_DoesNotReimportOnceLogged(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "chat-smartguide.json")
	jsonlPath := filepath.Join(dir, "chat-smartguide.jsonl")

	old := []anthropic.BetaMessageParam{userMsg("old")}
	data, err := json.Marshal(old)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(legacy, data, 0o600))

	s, err := Open(jsonlPath)
	require.NoError(t, err)
	require.NoError(t, s.ImportLegacy(legacy))
	require.NoError(t, s.Append(userMsg("new")))

	// A second open must not replay the legacy file on top of the log.
	s2, err := Open(jsonlPath)
	require.NoError(t, err)
	require.NoError(t, s2.ImportLegacy(legacy))
	assert.Equal(t, 2, s2.Len(), "legacy import is once, not every open")
}

func TestImportLegacy_MissingFileIsFine(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "s.jsonl"))
	require.NoError(t, err)
	require.NoError(t, s.ImportLegacy(filepath.Join(dir, "nope.json")))
	assert.Equal(t, 0, s.Len())
}
