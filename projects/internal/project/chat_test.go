package project

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// pngBytes is a 1x1 PNG.
func pngBytes() []byte {
	b, _ := hex.DecodeString("89504e470d0a1a0a0000000d4948445200000001000000010806000000" +
		"1f15c4890000000d49444154789c6360000002000100ffff030000060005" +
		"57bfabd40000000049454e44ae426082")
	return b
}

func TestChatLaunchIsReadOnly(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Hello.", 3, 2)
		return nil
	})
	ch, err := f.threads.StartChat(context.Background(), ChatThreadSpec{Text: "hi"})
	require.NoError(t, err)
	got := waitStatus(t, f.store, ch.ID, db.ThreadIdle)

	chatProject, err := f.store.GetProject(ch.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, db.ProjectChat, chatProject.Type)
	assert.Equal(t, f.threads.layout.ChatDir(ch.ID), ch.Cwd)
	assert.Equal(t, "default", got.PermissionMode)
	assert.Equal(t, 1, got.Turns)

	spec := f.agent.Launches()[0]
	assert.Equal(t, []string{"Read", "Glob", "Grep"}, spec.Tools)
	assert.Equal(t, []string{"Read", "Glob", "Grep"}, spec.AllowedTools)
	assert.True(t, spec.Restricted)
	assert.True(t, spec.SafeMode)
	assert.True(t, spec.StrictMcpConfig)
	assert.Equal(t, ChatSystemPrompt, spec.CustomSystemPrompt)
	assert.Empty(t, spec.AppendSystemPrompt, "no project prompt")
	assert.Empty(t, spec.SettingsPath, "no settings")
	assert.Empty(t, spec.MCPConfigPath, "no MCP")
	assert.Empty(t, spec.AdditionalMCPConfigPaths)
	assert.Equal(t, "default", spec.PermissionMode)
	assert.Contains(t, spec.Env, "CLAUDE_CONFIG_DIR="+f.threads.layout.ChatConfigDir(ch.ID))
	assert.Contains(t, spec.Env, "TMPDIR="+f.threads.layout.ChatTmpDir(ch.ID))
	assert.Contains(t, spec.Env, "ANTHROPIC_BASE_URL=http://gw")

	select {
	case r := <-f.reports:
		t.Fatalf("a chat has no coordinator to report to: %+v", r)
	default:
	}
	events, _ := f.store.ListThreadEvents(ch.ID)
	for _, e := range events {
		assert.NotEqual(t, db.EvReport, e.Kind)
	}
}

func TestChatImageIsAVisionBlockAndTextIsAReadPath(t *testing.T) {
	type seen struct {
		prompt  string
		content []string
	}
	got := make(chan seen, 1)
	f := newFixture(t, func(turn *enginetest.Turn) error {
		var kinds []string
		for _, b := range turn.Opts.Content {
			kinds = append(kinds, b.Type+":"+b.MediaType)
		}
		got <- seen{prompt: turn.Prompt, content: kinds}
		return nil
	})
	ch, err := f.threads.StartChat(context.Background(), ChatThreadSpec{
		Text: "what is this?",
		Attachments: []ChatUpload{
			{Name: "pic.png", Data: pngBytes()},
			{Name: "notes.txt", Data: []byte("NOTES-CONTENT")},
		},
	})
	require.NoError(t, err)
	waitStatus(t, f.store, ch.ID, db.ThreadIdle)
	s := <-got
	assert.Contains(t, s.prompt, "attachments/", "text attachment path is in the prompt")
	assert.NotContains(t, s.prompt, "NOTES-CONTENT", "text content stays on disk for Read")
	require.Len(t, s.content, 1, "one image block")
	assert.Equal(t, "image:image/png", s.content[0])
}

func TestChatRefusesBinaryAndOversizedUploads(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })

	_, err := f.threads.StartChat(context.Background(), ChatThreadSpec{
		Text:        "read this",
		Attachments: []ChatUpload{{Name: "doc.pdf", Data: []byte("%PDF-1.7\n...")}},
	})
	require.ErrorIs(t, err, ErrChatUnsupportedType)

	big := append(pngBytes(), make([]byte, ChatMaxImageBytes)...)
	_, err = f.threads.StartChat(context.Background(), ChatThreadSpec{
		Text:        "look",
		Attachments: []ChatUpload{{Name: "big.png", Data: big}},
	})
	var limit *ChatLimitError
	require.ErrorAs(t, err, &limit)
	assert.Equal(t, "image", limit.Kind)
}

func TestChatAttachmentPathsCannotEscape(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	ch, err := f.threads.StartChat(context.Background(), ChatThreadSpec{
		Text:        "hi",
		Attachments: []ChatUpload{{Name: "a.txt", Data: []byte("hello")}},
	})
	require.NoError(t, err)
	waitStatus(t, f.store, ch.ID, db.ThreadIdle)

	atts, err := f.threads.SaveChatAttachments(ch.ID, []ChatUpload{{Name: "b.txt", Data: []byte("more")}})
	require.NoError(t, err)
	require.Len(t, atts, 1)
	resolved, err := f.threads.layout.ResolveChatAttachments(ch.ID, []string{atts[0].Path})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(resolved[0].Name, "b"))

	for _, bad := range []string{"../.claude/settings.json", "/etc/passwd", "attachments/../../x"} {
		_, err := f.threads.layout.ResolveChatAttachments(ch.ID, []string{bad})
		assert.Error(t, err, bad)
	}
	// A symlink inside attachments/ is refused even if it points inside.
	link := filepath.Join(f.threads.layout.ChatAttachmentsDir(ch.ID), "0000000000000000-link.txt")
	require.NoError(t, os.Symlink("/etc/passwd", link))
	_, err = f.threads.layout.ResolveChatAttachments(ch.ID, []string{"attachments/" + filepath.Base(link)})
	assert.Error(t, err)
}

func TestChatDeleteRemovesItsFolder(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("bye", 1, 1); return nil })
	ch, err := f.threads.StartChat(context.Background(), ChatThreadSpec{Text: "hi"})
	require.NoError(t, err)
	waitStatus(t, f.store, ch.ID, db.ThreadIdle)

	dir := f.threads.layout.ChatDir(ch.ID)
	require.DirExists(t, dir)
	require.NoError(t, f.threads.DeleteChat(ch.ID))
	assert.NoDirExists(t, dir)
	_, err = f.store.GetThread(ch.ID)
	require.Error(t, err)
}

func TestChatExportIncludesAttachments(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("exported", 1, 1); return nil })
	ch, err := f.threads.StartChat(context.Background(), ChatThreadSpec{
		Text:        "hi",
		Attachments: []ChatUpload{{Name: "notes.txt", Data: []byte("export me")}},
	})
	require.NoError(t, err)
	waitStatus(t, f.store, ch.ID, db.ThreadIdle)

	var buf bytes.Buffer
	require.NoError(t, f.threads.ExportChat(ch.ID, &buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	names := map[string]string{}
	for _, zf := range zr.File {
		f, err := zf.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(f)
		f.Close()
		require.NoError(t, err)
		names[zf.Name] = string(b)
	}
	assert.Contains(t, names, "chat.json")
	found := false
	for name, body := range names {
		if strings.HasPrefix(name, "attachments/") {
			found = true
			assert.Equal(t, "export me", body)
		}
	}
	assert.True(t, found, "the export carries the attachment: %v", names)
	assert.Contains(t, names["chat.json"], "tingly-chat")

	err = f.threads.ExportChat(f.project.ID, &bytes.Buffer{})
	assert.Error(t, err, "a project is not a chat")
}
