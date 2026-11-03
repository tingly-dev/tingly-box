package weixin

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

	"github.com/tingly-dev/tingly-box/imbot/core"
)

type fakeIlink struct {
	mu       sync.Mutex
	upload   map[string]any   // getuploadurl request
	cdnBytes int              // ciphertext size received
	sends    []map[string]any // sendmessage requests
}

func (f *fakeIlink) server(t *testing.T) *httptest.Server {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/ilink/bot/getuploadurl", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.upload = req
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0, "upload_full_url": srv.URL + "/cdn"})
	})
	mux.HandleFunc("/cdn", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.cdnBytes = len(b)
		f.mu.Unlock()
		w.Header().Set("x-encrypted-param", "dl-param")
	})
	mux.HandleFunc("/ilink/bot/sendmessage", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.sends = append(f.sends, req)
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"ret":0}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newMediaTestBot(t *testing.T, baseURL string) *Bot {
	t.Helper()
	bot, err := NewBot(&core.Config{
		Platform: core.PlatformWeixin,
		Auth:     core.AuthConfig{Type: "qr", Token: "bot-token", AccountID: "bot1"},
		Options:  map[string]interface{}{"baseUrl": baseURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	bot.MarkReady()
	return bot
}

func messageItems(t *testing.T, req map[string]any) []map[string]any {
	t.Helper()
	msg, _ := req["msg"].(map[string]any)
	raw, _ := msg["item_list"].([]any)
	var out []map[string]any
	for _, it := range raw {
		out = append(out, it.(map[string]any))
	}
	return out
}

func TestSendMessage_FileWithCaption(t *testing.T) {
	f := &fakeIlink{}
	srv := f.server(t)
	bot := newMediaTestBot(t, srv.URL)

	p := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(p, []byte("hello pdf"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := bot.SendMessage(context.Background(), "user@im.wechat", &core.SendMessageOptions{
		Text:     "here you go",
		Metadata: map[string]interface{}{"context_token": "ctx-1"},
		Media:    []core.MediaAttachment{{Type: "document", URL: "file://" + p, Filename: "report.pdf"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if f.cdnBytes == 0 {
		t.Error("nothing was uploaded to the CDN")
	}
	if len(f.sends) != 2 {
		t.Fatalf("want file message + caption message, got %d sends", len(f.sends))
	}
	fileItems := messageItems(t, f.sends[0])
	if len(fileItems) != 1 || fileItems[0]["file_item"] == nil {
		t.Fatalf("first message is not a file item: %v", f.sends[0])
	}
	fi := fileItems[0]["file_item"].(map[string]any)
	if fi["file_name"] != "report.pdf" {
		t.Errorf("file_item = %v", fi)
	}
	textItems := messageItems(t, f.sends[1])
	if len(textItems) != 1 || textItems[0]["text_item"] == nil {
		t.Fatalf("second message is not text: %v", f.sends[1])
	}
	for i, s := range f.sends {
		msg := s["msg"].(map[string]any)
		if msg["context_token"] != "ctx-1" {
			t.Errorf("send %d context_token = %v", i, msg["context_token"])
		}
	}
}

func TestSendMessage_ImageUsesImageItem(t *testing.T) {
	f := &fakeIlink{}
	srv := f.server(t)
	bot := newMediaTestBot(t, srv.URL)

	p := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(p, []byte("PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.SendMessage(context.Background(), "u", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "image", URL: "file://" + p, Filename: "pic.png"}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(f.sends) != 1 {
		t.Fatalf("sends = %d", len(f.sends))
	}
	if items := messageItems(t, f.sends[0]); len(items) != 1 || items[0]["image_item"] == nil {
		t.Errorf("not an image item: %v", f.sends[0])
	}
}

func TestSendMessage_MediaErrorsAreSurfaced(t *testing.T) {
	f := &fakeIlink{}
	srv := f.server(t)
	bot := newMediaTestBot(t, srv.URL)

	_, err := bot.SendMessage(context.Background(), "u", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "document", URL: "file:///no/such/file.pdf"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Fatalf("missing local file must fail clearly, got %v", err)
	}
	if len(f.sends) != 0 {
		t.Error("nothing should have been sent")
	}
}
