package dingtalk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

type fakeDingTalk struct {
	mu       sync.Mutex
	uploads  []string // "<type>:<filename>:<size>"
	sends    []fakeSend
	uploadFn func(w http.ResponseWriter)
	sendFn   func(w http.ResponseWriter)
}

type fakeSend struct {
	Path  string
	Token string
	Body  map[string]any
}

func (f *fakeDingTalk) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/gettoken", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":0,"access_token":"tok","expires_in":7200}`))
	})
	mux.HandleFunc("/media/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		file, hdr, err := r.FormFile("media")
		if err != nil {
			t.Errorf("form file media: %v", err)
			return
		}
		data, _ := io.ReadAll(file)
		f.mu.Lock()
		f.uploads = append(f.uploads, r.URL.Query().Get("type")+":"+hdr.Filename+":"+string(data))
		f.mu.Unlock()
		if r.URL.Query().Get("access_token") != "tok" {
			t.Errorf("upload without token")
		}
		if f.uploadFn != nil {
			f.uploadFn(w)
			return
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","media_id":"@mid1","type":"file"}`))
	})
	send := func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.sends = append(f.sends, fakeSend{r.URL.Path, r.Header.Get("x-acs-dingtalk-access-token"), body})
		f.mu.Unlock()
		if f.sendFn != nil {
			f.sendFn(w)
			return
		}
		_, _ = w.Write([]byte(`{"processQueryKey":"pqk"}`))
	}
	mux.HandleFunc("/v1.0/robot/groupMessages/send", send)
	mux.HandleFunc("/v1.0/robot/oToMessages/batchSend", send)
	return mux
}

func newTestBot(t *testing.T, f *fakeDingTalk) *Bot {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	bot, err := NewDingTalkBot(&core.Config{
		Platform: core.PlatformDingTalk,
		Auth:     core.AuthConfig{Type: "oauth", ClientID: "robot-key", ClientSecret: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bot.oapiBase, bot.apiBase = srv.URL, srv.URL
	bot.MarkReady()
	return bot
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSendMessage_ImageToGroup(t *testing.T) {
	f := &fakeDingTalk{}
	bot := newTestBot(t, f)
	bot.convMap["cidGroup"] = conversation{direct: false, staffID: "staff1"}
	p := writeFile(t, "pic.png", "PNGDATA")

	res, err := bot.SendMessage(context.Background(), "cidGroup", &core.SendMessageOptions{
		Text:  "look",
		Media: []core.MediaAttachment{{Type: "image", URL: "file://" + p, Filename: "pic.png", MimeType: "image/png"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "pqk" {
		t.Errorf("MessageID = %q", res.MessageID)
	}
	if len(f.uploads) != 1 || f.uploads[0] != "image:pic.png:PNGDATA" {
		t.Fatalf("uploads = %v", f.uploads)
	}
	if len(f.sends) != 2 {
		t.Fatalf("want image + caption sends, got %d", len(f.sends))
	}
	img := f.sends[0]
	if img.Path != "/v1.0/robot/groupMessages/send" || img.Token != "tok" {
		t.Errorf("send = %+v", img)
	}
	if img.Body["msgKey"] != "sampleImageMsg" || img.Body["openConversationId"] != "cidGroup" || img.Body["robotCode"] != "robot-key" {
		t.Errorf("body = %v", img.Body)
	}
	if img.Body["msgParam"] != `{"photoURL":"@mid1"}` {
		t.Errorf("msgParam = %v", img.Body["msgParam"])
	}
	txt := f.sends[1]
	if txt.Body["msgKey"] != "sampleText" || txt.Body["msgParam"] != `{"content":"look"}` {
		t.Errorf("caption body = %v", txt.Body)
	}
}

func TestSendMessage_FileToDirect(t *testing.T) {
	f := &fakeDingTalk{}
	bot := newTestBot(t, f)
	bot.convMap["cidDM"] = conversation{direct: true, staffID: "staff42"}
	p := writeFile(t, "report.PDF", "PDF")

	if _, err := bot.SendMessage(context.Background(), "cidDM", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "document", URL: "file://" + p, Filename: "report.PDF"}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(f.uploads) != 1 || !strings.HasPrefix(f.uploads[0], "file:report.PDF:") {
		t.Fatalf("uploads = %v", f.uploads)
	}
	if len(f.sends) != 1 {
		t.Fatalf("sends = %d", len(f.sends))
	}
	s := f.sends[0]
	if s.Path != "/v1.0/robot/oToMessages/batchSend" {
		t.Errorf("path = %s", s.Path)
	}
	users, _ := s.Body["userIds"].([]any)
	if len(users) != 1 || users[0] != "staff42" {
		t.Errorf("userIds = %v", s.Body["userIds"])
	}
	if s.Body["msgKey"] != "sampleFile" || s.Body["msgParam"] != `{"fileName":"report.PDF","fileType":"pdf","mediaId":"@mid1"}` {
		t.Errorf("body = %v", s.Body)
	}
}

func TestSendMessage_UnsupportedImageFormatGoesAsFile(t *testing.T) {
	f := &fakeDingTalk{}
	bot := newTestBot(t, f)
	bot.convMap["c"] = conversation{}
	p := writeFile(t, "pic.webp", "W")
	if _, err := bot.SendMessage(context.Background(), "c", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "image", URL: "file://" + p, Filename: "pic.webp"}},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.uploads[0], "file:pic.webp:") || f.sends[0].Body["msgKey"] != "sampleFile" {
		t.Errorf("uploads=%v sends=%v", f.uploads, f.sends)
	}
}

func TestSendMessage_ErrorsAreSurfaced(t *testing.T) {
	ctx := context.Background()
	p := writeFile(t, "a.txt", "x")
	media := []core.MediaAttachment{{Type: "document", URL: "file://" + p, Filename: "a.txt"}}

	t.Run("unknown conversation", func(t *testing.T) {
		bot := newTestBot(t, &fakeDingTalk{})
		if _, err := bot.SendMessage(ctx, "nope", &core.SendMessageOptions{Media: media}); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("upload errcode", func(t *testing.T) {
		f := &fakeDingTalk{uploadFn: func(w http.ResponseWriter) {
			_, _ = w.Write([]byte(`{"errcode":40005,"errmsg":"invalid file type"}`))
		}}
		bot := newTestBot(t, f)
		bot.convMap["c"] = conversation{}
		_, err := bot.SendMessage(ctx, "c", &core.SendMessageOptions{Media: media})
		if err == nil || !strings.Contains(err.Error(), "invalid file type") {
			t.Fatalf("err = %v", err)
		}
		if len(f.sends) != 0 {
			t.Error("must not send after failed upload")
		}
	})
	t.Run("send http error", func(t *testing.T) {
		f := &fakeDingTalk{sendFn: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":"Forbidden.AccessDenied.AccessTokenPermissionDenied"}`))
		}}
		bot := newTestBot(t, f)
		bot.convMap["c"] = conversation{}
		_, err := bot.SendMessage(ctx, "c", &core.SendMessageOptions{Media: media})
		if err == nil || !strings.Contains(err.Error(), "AccessTokenPermissionDenied") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("invalid staff", func(t *testing.T) {
		f := &fakeDingTalk{sendFn: func(w http.ResponseWriter) {
			_, _ = w.Write([]byte(`{"processQueryKey":"k","invalidStaffIdList":["s"]}`))
		}}
		bot := newTestBot(t, f)
		bot.convMap["c"] = conversation{direct: true, staffID: "s"}
		if _, err := bot.SendMessage(ctx, "c", &core.SendMessageOptions{Media: media}); err == nil {
			t.Fatal("want error for invalid staff")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		bot := newTestBot(t, &fakeDingTalk{})
		bot.convMap["c"] = conversation{}
		_, err := bot.SendMessage(ctx, "c", &core.SendMessageOptions{Media: []core.MediaAttachment{{Type: "document", URL: "file:///no/such/file"}}})
		if err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("too large", func(t *testing.T) {
		bot := newTestBot(t, &fakeDingTalk{})
		bot.convMap["c"] = conversation{}
		big := filepath.Join(t.TempDir(), "big.bin")
		f, _ := os.Create(big)
		_ = f.Truncate(maxMediaBytes + 1)
		_ = f.Close()
		if _, err := bot.SendMessage(ctx, "c", &core.SendMessageOptions{Media: []core.MediaAttachment{{Type: "document", URL: "file://" + big}}}); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestOnChatBotMessage_RecordsConversationForOpenAPI(t *testing.T) {
	f := &fakeDingTalk{}
	bot := newTestBot(t, f)
	bot.adapter = NewAdapter(bot.Config())

	dm := &chatbot.BotCallbackDataModel{ConversationId: "cidDM", ConversationType: "1", SenderStaffId: "staffA", SessionWebhook: "https://hook/dm", Msgtype: "text", Text: chatbot.BotCallbackDataTextModel{Content: "hi"}}
	grp := &chatbot.BotCallbackDataModel{ConversationId: "cidGrp", ConversationType: "2", SenderStaffId: "staffB", SessionWebhook: "https://hook/grp", Msgtype: "text", Text: chatbot.BotCallbackDataTextModel{Content: "hi"}}
	for _, d := range []*chatbot.BotCallbackDataModel{dm, grp} {
		if _, err := bot.onChatBotMessage(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}

	p := writeFile(t, "a.txt", "x")
	media := []core.MediaAttachment{{Type: "document", URL: "file://" + p, Filename: "a.txt"}}
	for _, id := range []string{"cidDM", "cidGrp"} {
		if _, err := bot.SendMessage(context.Background(), id, &core.SendMessageOptions{Media: media}); err != nil {
			t.Fatal(err)
		}
	}
	if f.sends[0].Path != "/v1.0/robot/oToMessages/batchSend" || f.sends[1].Path != "/v1.0/robot/groupMessages/send" {
		t.Errorf("routing = %s, %s", f.sends[0].Path, f.sends[1].Path)
	}
	if users, _ := f.sends[0].Body["userIds"].([]any); len(users) != 1 || users[0] != "staffA" {
		t.Errorf("userIds = %v", f.sends[0].Body["userIds"])
	}
}

func TestSendMessage_CaptionFailureAfterAttachmentIsReported(t *testing.T) {
	calls := 0
	f := &fakeDingTalk{}
	f.sendFn = func(w http.ResponseWriter) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
			return
		}
		_, _ = w.Write([]byte(`{"processQueryKey":"k"}`))
	}
	bot := newTestBot(t, f)
	bot.convMap["c"] = conversation{}
	p := writeFile(t, "a.txt", "x")
	_, err := bot.SendMessage(context.Background(), "c", &core.SendMessageOptions{
		Text:  "cap",
		Media: []core.MediaAttachment{{Type: "document", URL: "file://" + p}},
	})
	if err == nil || !strings.Contains(err.Error(), "attachment sent but description failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestErrorsDoNotLeakCredentials(t *testing.T) {
	bot, err := NewDingTalkBot(&core.Config{
		Platform: core.PlatformDingTalk,
		Auth:     core.AuthConfig{Type: "oauth", ClientID: "robot-key", ClientSecret: "TOPSECRET"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bot.MarkReady()
	bot.oapiBase, bot.apiBase = "http://127.0.0.1:1", "http://127.0.0.1:1"
	bot.convMap["c"] = conversation{}
	p := writeFile(t, "a.txt", "x")
	_, err = bot.SendMessage(context.Background(), "c", &core.SendMessageOptions{Media: []core.MediaAttachment{{Type: "document", URL: "file://" + p}}})
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "TOPSECRET") || strings.Contains(err.Error(), "appsecret") {
		t.Fatalf("error leaks the app secret: %v", err)
	}
}
