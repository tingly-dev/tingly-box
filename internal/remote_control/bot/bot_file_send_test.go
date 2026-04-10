package bot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	imbot "github.com/tingly-dev/tingly-box/imbot/core"
)

// stubBot is a minimal Bot implementation for testing SendFile.
// Only SendMessage is exercised; all other methods are no-ops.
type stubBot struct {
	sentOpts   *imbot.SendMessageOptions
	sentTarget string
	sendErr    error
}

func (b *stubBot) UUID() string                          { return "stub" }
func (b *stubBot) Connect(ctx context.Context) error    { return nil }
func (b *stubBot) Disconnect(ctx context.Context) error { return nil }
func (b *stubBot) IsConnected() bool                    { return true }
func (b *stubBot) SendMessage(ctx context.Context, target string, opts *imbot.SendMessageOptions) (*imbot.SendResult, error) {
	b.sentTarget = target
	b.sentOpts = opts
	return &imbot.SendResult{MessageID: "1"}, b.sendErr
}
func (b *stubBot) SendText(ctx context.Context, target string, text string) (*imbot.SendResult, error) {
	return &imbot.SendResult{}, nil
}
func (b *stubBot) SendMedia(ctx context.Context, target string, media []imbot.MediaAttachment) (*imbot.SendResult, error) {
	return &imbot.SendResult{}, nil
}
func (b *stubBot) React(ctx context.Context, messageID string, emoji string) error      { return nil }
func (b *stubBot) EditMessage(ctx context.Context, messageID string, text string) error { return nil }
func (b *stubBot) DeleteMessage(ctx context.Context, messageID string) error            { return nil }
func (b *stubBot) ChunkText(text string) []string                                       { return []string{text} }
func (b *stubBot) ValidateTextLength(text string) error                                 { return nil }
func (b *stubBot) GetMessageLimit() int                                                 { return 4096 }
func (b *stubBot) Status() *imbot.BotStatus                                             { return &imbot.BotStatus{} }
func (b *stubBot) PlatformInfo() *imbot.PlatformInfo                                   { return &imbot.PlatformInfo{} }
func (b *stubBot) OnMessage(handler func(imbot.Message))                                {}
func (b *stubBot) OnError(handler func(error))                                          {}
func (b *stubBot) OnConnected(handler func())                                           {}
func (b *stubBot) OnDisconnected(handler func())                                        {}
func (b *stubBot) OnReady(handler func())                                               {}
func (b *stubBot) Close() error                                                         { return nil }

func makeTestHandlerCtx(bot imbot.Bot) HandlerContext {
	return HandlerContext{
		Bot:      bot,
		ChatID:   "chat-123",
		Platform: "mock",
		Message:  imbot.Message{},
	}
}

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// ============================================================================
// BotHandler.SendFile tests
// ============================================================================

func TestBotHandlerSendFile_CallsSendMedia(t *testing.T) {
	handler := &BotHandler{}
	bot := &stubBot{}
	hCtx := makeTestHandlerCtx(bot)

	dir := t.TempDir()
	filePath := writeTempFile(t, dir, "result.txt", "analysis complete")

	err := handler.SendFile(context.Background(), hCtx, filePath, "Here is the result")
	require.NoError(t, err)

	assert.Equal(t, "chat-123", bot.sentTarget)
	require.NotNil(t, bot.sentOpts)
	assert.Equal(t, "Here is the result", bot.sentOpts.Text)
	require.Len(t, bot.sentOpts.Media, 1)
	assert.Equal(t, "result.txt", bot.sentOpts.Media[0].Filename)
	assert.Equal(t, "document", bot.sentOpts.Media[0].Type)
}

func TestBotHandlerSendFile_ImageTypeDetected(t *testing.T) {
	handler := &BotHandler{}
	bot := &stubBot{}
	hCtx := makeTestHandlerCtx(bot)

	dir := t.TempDir()
	filePath := writeTempFile(t, dir, "chart.png", "PNG_DATA")

	err := handler.SendFile(context.Background(), hCtx, filePath, "")
	require.NoError(t, err)

	require.Len(t, bot.sentOpts.Media, 1)
	assert.Equal(t, "image", bot.sentOpts.Media[0].Type)
	assert.Equal(t, "chart.png", bot.sentOpts.Media[0].Filename)
}

func TestBotHandlerSendFile_FileNotFound(t *testing.T) {
	handler := &BotHandler{}
	bot := &stubBot{}
	hCtx := makeTestHandlerCtx(bot)

	err := handler.SendFile(context.Background(), hCtx, "/nonexistent/path/file.txt", "")
	assert.Error(t, err)
	assert.Nil(t, bot.sentOpts)
}

func TestBotHandlerSendFile_EmptyCaption(t *testing.T) {
	handler := &BotHandler{}
	bot := &stubBot{}
	hCtx := makeTestHandlerCtx(bot)

	dir := t.TempDir()
	filePath := writeTempFile(t, dir, "data.csv", "col1,col2\n1,2")

	err := handler.SendFile(context.Background(), hCtx, filePath, "")
	require.NoError(t, err)

	require.NotNil(t, bot.sentOpts)
	assert.Empty(t, bot.sentOpts.Text)
}

func TestBotHandlerSendFile_FileSizePopulated(t *testing.T) {
	handler := &BotHandler{}
	bot := &stubBot{}
	hCtx := makeTestHandlerCtx(bot)

	dir := t.TempDir()
	content := "hello world"
	filePath := writeTempFile(t, dir, "hello.txt", content)

	err := handler.SendFile(context.Background(), hCtx, filePath, "")
	require.NoError(t, err)

	require.Len(t, bot.sentOpts.Media, 1)
	assert.Equal(t, int64(len(content)), bot.sentOpts.Media[0].Size)
}
