package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

func TestSendMediaUploadsByCategoryAndSendsCaption(t *testing.T) {
	for _, tc := range []struct{ name, mime, kind, endpoint, msgType, uploadField string }{
		{"image.png", "image/png", "image", "/open-apis/im/v1/images", "image", "image_type"},
		{"report.pdf", "application/pdf", "document", "/open-apis/im/v1/files", "file", "file_type"},
		{"vector.svg", "image/svg+xml", "image", "/open-apis/im/v1/files", "file", "file_type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type message struct {
				MsgType   string `json:"msg_type"`
				Content   string `json:"content"`
				ReceiveId string `json:"receive_id"`
			}
			var messages []message
			var uploaded string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/open-apis/auth/v3/tenant_access_token/internal":
					fmt.Fprint(w, `{"code":0,"tenant_access_token":"test-token","expire":7200}`)
				case tc.endpoint:
					require.NoError(t, r.ParseMultipartForm(1<<20))
					typ := "message"
					part := "image"
					key := "image_key"
					if tc.msgType == "file" {
						typ = "stream"
						part = "file"
						key = "file_key"
						require.Equal(t, tc.name, r.FormValue("file_name"))
					}
					require.Equal(t, typ, r.FormValue(tc.uploadField))
					f, _, err := r.FormFile(part)
					require.NoError(t, err)
					defer f.Close()
					data, err := io.ReadAll(f)
					require.NoError(t, err)
					uploaded = string(data)
					fmt.Fprintf(w, `{"code":0,"data":{"%s":"key-123"}}`, key)
				case "/open-apis/im/v1/messages":
					require.Equal(t, "open_id", r.URL.Query().Get("receive_id_type"))
					var msg message
					require.NoError(t, json.NewDecoder(r.Body).Decode(&msg))
					messages = append(messages, msg)
					fmt.Fprint(w, `{"code":0,"data":{"message_id":"om_123"}}`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, _ := buildIMService("id", "secret", server.URL)
			base := core.NewBaseBot(&core.Config{})
			base.SetStatus(core.BotStatus{Ready: true})
			bot := &Bot{BaseBot: base, client: client, domain: DomainFeishu}
			path := filepath.Join(t.TempDir(), tc.name)
			require.NoError(t, os.WriteFile(path, []byte("payload"), 0600))
			_, err := bot.SendMessage(context.Background(), "ou_test", &core.SendMessageOptions{Text: "caption", Media: []core.MediaAttachment{{Type: tc.kind, MimeType: tc.mime, Filename: tc.name, URL: (&url.URL{Scheme: "file", Path: path}).String()}}})
			require.NoError(t, err)
			require.Equal(t, "payload", uploaded)
			require.Len(t, messages, 2)
			require.Equal(t, tc.msgType, messages[0].MsgType)
			require.Equal(t, "ou_test", messages[0].ReceiveId)
			var content map[string]string
			require.NoError(t, json.Unmarshal([]byte(messages[0].Content), &content))
			require.Equal(t, "key-123", content[tc.msgType+"_key"])
			require.Equal(t, "text", messages[1].MsgType)
			require.Contains(t, messages[1].Content, "caption")
		})
	}
}

func TestSendMediaReportsUploadFailureWithoutSendingText(t *testing.T) {
	var sent bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			fmt.Fprint(w, `{"code":0,"tenant_access_token":"test-token","expire":7200}`)
		case "/open-apis/im/v1/images":
			fmt.Fprint(w, `{"code":99991672,"msg":"missing permission"}`)
		default:
			sent = true
			fmt.Fprint(w, `{"code":0}`)
		}
	}))
	defer server.Close()
	client, _ := buildIMService("id", "secret", server.URL)
	base := core.NewBaseBot(&core.Config{})
	base.SetStatus(core.BotStatus{Ready: true})
	bot := &Bot{BaseBot: base, client: client, domain: DomainFeishu}
	path := filepath.Join(t.TempDir(), "image.png")
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0600))
	_, err := bot.SendMessage(context.Background(), "ou_test", &core.SendMessageOptions{Text: "caption", Media: []core.MediaAttachment{{Type: "image", MimeType: "image/png", URL: (&url.URL{Scheme: "file", Path: path}).String()}}})
	require.ErrorContains(t, err, "missing permission")
	require.False(t, sent)
}
