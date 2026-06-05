package feishu

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

// These are wire-level tests: they point the Lark SDK HTTP client at a local
// httptest server and assert the exact outgoing request (path, receive_id_type,
// msg_type, body), without any real Feishu credentials or manual interaction.
// They catch the regressions that have actually bitten this code: a wrong
// receive_id_type (open_id cross app), a card that renders without buttons, and
// media sent to the wrong endpoint.

type recordedReq struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

type fakeFeishu struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []recordedReq
}

// newFakeFeishu starts a server that answers the tenant-token endpoint and
// records every other request, returning canned success responses keyed by path.
func newFakeFeishu(t *testing.T) *fakeFeishu {
	t.Helper()
	f := &fakeFeishu{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "tenant_access_token") {
			io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-abc","expire":7200}`)
			return
		}

		f.mu.Lock()
		f.reqs = append(f.reqs, recordedReq{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Body:   string(body),
		})
		f.mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/im/v1/images"):
			io.WriteString(w, `{"code":0,"msg":"ok","data":{"image_key":"img_FAKE"}}`)
		case strings.HasSuffix(r.URL.Path, "/im/v1/files"):
			io.WriteString(w, `{"code":0,"msg":"ok","data":{"file_key":"file_FAKE"}}`)
		case strings.HasSuffix(r.URL.Path, "/im/v1/messages"):
			io.WriteString(w, `{"code":0,"msg":"ok","data":{"message_id":"om_FAKE"}}`)
		case strings.Contains(r.URL.Path, "/reactions"):
			io.WriteString(w, `{"code":0,"msg":"ok","data":{"reaction_id":"r1"}}`)
		default:
			io.WriteString(w, `{"code":0,"msg":"ok"}`)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// requests returns the recorded non-token requests in order.
func (f *fakeFeishu) requests() []recordedReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedReq, len(f.reqs))
	copy(out, f.reqs)
	return out
}

// lastRequest returns the most recent recorded request.
func (f *fakeFeishu) lastRequest(t *testing.T) recordedReq {
	t.Helper()
	reqs := f.requests()
	if len(reqs) == 0 {
		t.Fatal("no request recorded")
	}
	return reqs[len(reqs)-1]
}

// newBot returns a ready Feishu bot whose SDK client targets the fake server.
func (f *fakeFeishu) newBot(t *testing.T) *Bot {
	t.Helper()
	cfg := &core.Config{
		Platform: core.PlatformFeishu,
		Auth: core.AuthConfig{
			Type:         "oauth",
			ClientID:     "cli_x",
			ClientSecret: "sec_x",
		},
	}
	bot, err := NewBot(cfg, DomainFeishu)
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	bot.client = lark.NewClient("cli_x", "sec_x",
		lark.WithOpenBaseUrl(f.srv.URL),
		lark.WithHttpClient(f.srv.Client()),
	)
	bot.MarkReady()
	return bot
}

func mustContain(t *testing.T, haystack, needle, what string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: expected to contain %q, got: %s", what, needle, haystack)
	}
}

func TestWire_SendText_PlainAndReceiveIdType(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	if _, err := bot.SendMessage(context.Background(), "ou_user1", &core.SendMessageOptions{Text: "hello"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	req := f.lastRequest(t)
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if !strings.HasSuffix(req.Path, "/im/v1/messages") {
		t.Errorf("path = %s, want .../im/v1/messages", req.Path)
	}
	// ou_ prefix must resolve to open_id, not the bug-era default.
	if got := req.Query.Get("receive_id_type"); got != "open_id" {
		t.Errorf("receive_id_type = %q, want open_id", got)
	}
	mustContain(t, req.Body, `"msg_type":"text"`, "body")
	mustContain(t, req.Body, "hello", "body")
}

func TestWire_SendInteractiveCard_ChatIdAndButtons(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	kb := interaction.NewKeyboardBuilder().
		AddRow(interaction.CallbackButton("Approve", "perm:allow:req1")).
		Build()

	_, err := bot.SendMessage(context.Background(), "oc_chat1", &core.SendMessageOptions{
		Text:     "decide:",
		Metadata: map[string]interface{}{"replyMarkup": kb},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	req := f.lastRequest(t)
	// oc_ prefix is a chat id; sending it as open_id is what caused "open_id cross app".
	if got := req.Query.Get("receive_id_type"); got != "chat_id" {
		t.Errorf("receive_id_type = %q, want chat_id", got)
	}
	mustContain(t, req.Body, `"msg_type":"interactive"`, "body")
	// The keyboard must actually render as a card button, not vanish.
	mustContain(t, req.Body, "button", "card body")
	mustContain(t, req.Body, "Approve", "card body")
}

func TestWire_SendCardJSON_Verbatim(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	cardJSON := `{"config":{"wide_screen_mode":true},"elements":[{"tag":"div","text":{"tag":"lark_md","content":"hi"}}]}`
	_, err := bot.SendMessage(context.Background(), "oc_chat1", &core.SendMessageOptions{
		Text:     "ignored",
		Metadata: map[string]interface{}{"card_json": cardJSON},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	req := f.lastRequest(t)
	mustContain(t, req.Body, `"msg_type":"interactive"`, "body")
	mustContain(t, req.Body, "wide_screen_mode", "card body")
}

func TestWire_SendMedia_Image(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	path := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := bot.SendMessage(context.Background(), "oc_chat1", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "image", URL: path}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	reqs := f.requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (upload + create), got %d", len(reqs))
	}
	if !strings.HasSuffix(reqs[0].Path, "/im/v1/images") {
		t.Errorf("first request path = %s, want .../im/v1/images", reqs[0].Path)
	}
	if !strings.HasSuffix(reqs[1].Path, "/im/v1/messages") {
		t.Errorf("second request path = %s, want .../im/v1/messages", reqs[1].Path)
	}
	mustContain(t, reqs[1].Body, `"msg_type":"image"`, "create body")
	mustContain(t, reqs[1].Body, "img_FAKE", "create body") // uploaded key plumbed through
}

func TestWire_SendMedia_File(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := bot.SendMessage(context.Background(), "oc_chat1", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "document", URL: path, Filename: "report.pdf"}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	reqs := f.requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (upload + create), got %d", len(reqs))
	}
	if !strings.HasSuffix(reqs[0].Path, "/im/v1/files") {
		t.Errorf("first request path = %s, want .../im/v1/files", reqs[0].Path)
	}
	mustContain(t, reqs[1].Body, `"msg_type":"file"`, "create body")
	mustContain(t, reqs[1].Body, "file_FAKE", "create body")
}

func TestWire_EditMessage_Patch(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	if err := bot.EditMessage(context.Background(), "om_123", "done"); err != nil {
		t.Fatalf("EditMessage: %v", err)
	}

	req := f.lastRequest(t)
	if req.Method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", req.Method)
	}
	if !strings.HasSuffix(req.Path, "/im/v1/messages/om_123") {
		t.Errorf("path = %s, want .../messages/om_123", req.Path)
	}
	mustContain(t, req.Body, "done", "patch body")
}

func TestWire_DeleteMessage(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	if err := bot.DeleteMessage(context.Background(), "om_123"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	req := f.lastRequest(t)
	if req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.Method)
	}
	if !strings.HasSuffix(req.Path, "/im/v1/messages/om_123") {
		t.Errorf("path = %s, want .../messages/om_123", req.Path)
	}
}

func TestWire_React(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	if err := bot.React(context.Background(), "om_123", "THUMBSUP"); err != nil {
		t.Fatalf("React: %v", err)
	}

	req := f.lastRequest(t)
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if !strings.HasSuffix(req.Path, "/im/v1/messages/om_123/reactions") {
		t.Errorf("path = %s, want .../messages/om_123/reactions", req.Path)
	}
	mustContain(t, req.Body, "THUMBSUP", "reaction body")
}

// --- L3: inbound event replay ---

func sptr(s string) *string { return &s }

func TestConvertLarkMessage_ImageInbound(t *testing.T) {
	f := newFakeFeishu(t)
	bot := f.newBot(t)

	ev := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{UserId: sptr("u1"), OpenId: sptr("ou_1")},
			},
			Message: &larkim.EventMessage{
				MessageId:   sptr("om_in"),
				ChatId:      sptr("oc_in"),
				ChatType:    sptr("p2p"),
				MessageType: sptr("image"),
				Content:     sptr(`{"image_key":"img_in"}`),
			},
		},
	}

	msg := bot.convertLarkMessageToCore(ev)

	// Reply target must be the chat id (oc_), matching the card-callback target.
	if msg.GetReplyTarget() != "oc_in" {
		t.Errorf("reply target = %q, want oc_in", msg.GetReplyTarget())
	}
	if !msg.IsMediaContent() {
		t.Fatalf("expected media content, got %T", msg.Content)
	}
	media := msg.GetMedia()
	if len(media) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(media))
	}
	if media[0].URL != "feishu://img_in" {
		t.Errorf("attachment URL = %q, want feishu://img_in", media[0].URL)
	}
	if media[0].Raw["feishu_res_type"] != "image" || media[0].Raw["feishu_message_id"] != "om_in" {
		t.Errorf("unexpected attachment raw: %+v", media[0].Raw)
	}
}
