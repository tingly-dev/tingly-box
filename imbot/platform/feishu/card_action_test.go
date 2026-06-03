package feishu

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// captureMessage registers a handler and returns a function that waits for the
// next emitted message (handlers run on goroutines, so synchronization is needed).
func captureMessage(bot *Bot) func(t *testing.T) (core.Message, bool) {
	ch := make(chan core.Message, 1)
	bot.OnMessage(func(m core.Message) { ch <- m })
	return func(t *testing.T) (core.Message, bool) {
		t.Helper()
		select {
		case m := <-ch:
			return m, true
		case <-time.After(time.Second):
			return core.Message{}, false
		}
	}
}

func TestBuildLarkContent_Image(t *testing.T) {
	bot := newTestBot(t)
	content := bot.buildLarkContent("image", map[string]interface{}{"image_key": "img_v2_x"}, "", "om_1")

	mc, ok := content.(*core.MediaContent)
	if !ok {
		t.Fatalf("expected *core.MediaContent, got %T", content)
	}
	if len(mc.Media) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(mc.Media))
	}
	att := mc.Media[0]
	if att.Type != "image" || att.URL != "feishu://img_v2_x" || att.MimeType != "image/png" {
		t.Errorf("unexpected image attachment: %+v", att)
	}
	if att.Raw["feishu_res_type"] != "image" || att.Raw["feishu_file_key"] != "img_v2_x" || att.Raw["feishu_message_id"] != "om_1" {
		t.Errorf("unexpected raw: %+v", att.Raw)
	}
}

func TestBuildLarkContent_File(t *testing.T) {
	bot := newTestBot(t)
	content := bot.buildLarkContent("file", map[string]interface{}{"file_key": "file_x", "file_name": "report.pdf"}, "", "om_2")

	mc, ok := content.(*core.MediaContent)
	if !ok {
		t.Fatalf("expected *core.MediaContent, got %T", content)
	}
	att := mc.Media[0]
	if att.Type != "document" || att.Filename != "report.pdf" || att.MimeType != "application/pdf" {
		t.Errorf("unexpected file attachment: %+v", att)
	}
	if att.Raw["feishu_res_type"] != "file" {
		t.Errorf("expected res_type=file, got %v", att.Raw["feishu_res_type"])
	}
}

func TestBuildLarkContent_Text(t *testing.T) {
	bot := newTestBot(t)
	content := bot.buildLarkContent("text", map[string]interface{}{"text": "hello"}, `{"text":"hello"}`, "om_3")
	tc, ok := content.(*core.TextContent)
	if !ok {
		t.Fatalf("expected *core.TextContent, got %T", content)
	}
	if tc.Text != "hello" {
		t.Errorf("expected text=hello, got %q", tc.Text)
	}
}

func TestMimeFromFileName(t *testing.T) {
	// Only assert extensions in Go's builtin MIME table (system mime.types may vary).
	cases := map[string]string{
		"report.pdf":  "application/pdf",
		"photo.png":   "image/png",
		"noext":       "application/octet-stream",
		"archive.xyz": "application/octet-stream",
	}
	for name, want := range cases {
		if got := mimeFromFileName(name); got != want {
			t.Errorf("mimeFromFileName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFeishuFileType(t *testing.T) {
	cases := map[string]string{
		"report.pdf":  "pdf",
		"a.docx":      "doc",
		"b.xlsx":      "xls",
		"c.pptx":      "ppt",
		"clip.mp4":    "mp4",
		"voice.opus":  "opus",
		"archive.zip": "stream",
		"noext":       "stream",
	}
	for name, want := range cases {
		if got := feishuFileType(name); got != want {
			t.Errorf("feishuFileType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestOpenMediaReader_LocalFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/hello.txt"
	if err := os.WriteFile(path, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	r, closeFn, err := openMediaReader(context.Background(), "file://"+path)
	if err != nil {
		t.Fatalf("openMediaReader: %v", err)
	}
	defer closeFn()
	got, _ := io.ReadAll(r)
	if string(got) != "hi" {
		t.Errorf("expected %q, got %q", "hi", string(got))
	}
}

func TestGetReceiveIdType(t *testing.T) {
	cases := map[string]string{
		"ou_abc123":        "open_id",
		"on_abc123":        "union_id",
		"oc_abc123":        "chat_id",
		"user@example.com": "email",
		"g1234567":         "user_id",
		"":                 "user_id",
	}
	for in, want := range cases {
		if got := getReceiveIdType(in); got != want {
			t.Errorf("getReceiveIdType(%q) = %q, want %q", in, got, want)
		}
	}
}

func newTestBot(t *testing.T) *Bot {
	t.Helper()
	cfg := &core.Config{
		Platform: core.PlatformFeishu,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     "cli_test",
			ClientSecret: "secret_test",
		},
	}
	bot, err := NewBot(cfg, DomainFeishu)
	if err != nil {
		t.Fatalf("NewBot failed: %v", err)
	}
	return bot
}

func TestHandleCardActionTrigger_EmitsCallbackMessage(t *testing.T) {
	bot := newTestBot(t)
	wait := captureMessage(bot)

	userID := "user-123"
	event := &callback.CardActionTriggerEvent{
		Event: &callback.CardActionTriggerRequest{
			Token:    "tok-1",
			Operator: &callback.Operator{UserID: &userID},
			Action: &callback.CallBackAction{
				Value: map[string]interface{}{"callback": "perm:allow:req1"},
			},
			Context: &callback.Context{
				OpenMessageID: "om_msg1",
				OpenChatID:    "oc_chat1",
			},
		},
	}

	resp, err := bot.handleCardActionTrigger(context.Background(), event)
	if err != nil {
		t.Fatalf("handleCardActionTrigger returned error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response (ack), got %+v", resp)
	}
	got, ok := wait(t)
	if !ok {
		t.Fatal("expected a message to be emitted")
	}

	if isCb, _ := got.Metadata["is_callback"].(bool); !isCb {
		t.Errorf("expected is_callback=true, got %v", got.Metadata["is_callback"])
	}
	if cd, _ := got.Metadata["callback_data"].(string); cd != "perm:allow:req1" {
		t.Errorf("expected callback_data=perm:allow:req1, got %q", cd)
	}
	if mid, _ := got.Metadata["message_id"].(string); mid != "om_msg1" {
		t.Errorf("expected message_id=om_msg1, got %q", mid)
	}
	if got.GetReplyTarget() != "oc_chat1" {
		t.Errorf("expected reply target=oc_chat1 (chat id), got %q", got.GetReplyTarget())
	}
	if got.Sender.ID != userID {
		t.Errorf("expected sender=%q, got %q", userID, got.Sender.ID)
	}
}

func TestHandleCardActionTrigger_NoCallbackData(t *testing.T) {
	bot := newTestBot(t)
	wait := captureMessage(bot)

	event := &callback.CardActionTriggerEvent{
		Event: &callback.CardActionTriggerRequest{
			Action:  &callback.CallBackAction{Value: map[string]interface{}{}},
			Context: &callback.Context{OpenChatID: "oc_chat1"},
		},
	}

	resp, err := bot.handleCardActionTrigger(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
	if _, ok := wait(t); ok {
		t.Error("expected no message to be emitted when callback data is absent")
	}
}

func TestHandleCardActionTrigger_FallsBackToChatID(t *testing.T) {
	bot := newTestBot(t)
	wait := captureMessage(bot)

	event := &callback.CardActionTriggerEvent{
		Event: &callback.CardActionTriggerRequest{
			Operator: &callback.Operator{OpenID: "ou_open1"},
			Action:   &callback.CallBackAction{Value: map[string]interface{}{"callback": "action:bind"}},
			Context:  &callback.Context{OpenChatID: "oc_chat1", OpenMessageID: "om_msg1"},
		},
	}

	if _, err := bot.handleCardActionTrigger(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := wait(t)
	if !ok {
		t.Fatal("expected a message to be emitted")
	}
	if got.GetReplyTarget() != "oc_chat1" {
		t.Errorf("expected reply target to fall back to OpenChatID, got %q", got.GetReplyTarget())
	}
	if got.Sender.ID != "ou_open1" {
		t.Errorf("expected sender to be operator open_id, got %q", got.Sender.ID)
	}
}
