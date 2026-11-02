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
			_, err := bot.SendMessage(context.Background(), "ou_test", &core.SendMessageOptions{Text: "**caption**\n\n| A | B |\n| --- | --- |\n| 1 | 2 |", ParseMode: core.ParseModeMarkdown, Media: []core.MediaAttachment{{Type: tc.kind, MimeType: tc.mime, Filename: tc.name, URL: (&url.URL{Scheme: "file", Path: path}).String()}}})
			require.NoError(t, err)
			require.Equal(t, "payload", uploaded)
			require.Len(t, messages, 2)
			require.Equal(t, tc.msgType, messages[0].MsgType)
			require.Equal(t, "ou_test", messages[0].ReceiveId)
			var content map[string]string
			require.NoError(t, json.Unmarshal([]byte(messages[0].Content), &content))
			require.Equal(t, "key-123", content[tc.msgType+"_key"])
			require.Equal(t, "post", messages[1].MsgType)
			var post struct {
				ZhCN struct {
					Content [][]struct {
						Tag  string `json:"tag"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"zh_cn"`
			}
			require.NoError(t, json.Unmarshal([]byte(messages[1].Content), &post))
			require.Len(t, post.ZhCN.Content, 1)
			require.Len(t, post.ZhCN.Content[0], 1)
			require.Equal(t, "md", post.ZhCN.Content[0][0].Tag)
			require.Equal(t, "**caption**\n\n| A | B |\n| --- | --- |\n| 1 | 2 |", post.ZhCN.Content[0][0].Text)
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

func TestSendTextMarkdownAndPlain(t *testing.T) {
	for _, tc := range []struct {
		domain Domain
		mode   core.ParseMode
		want   string
	}{
		{DomainFeishu, core.ParseModeMarkdown, "post"},
		{DomainFeishu, "", "text"},
		{DomainLark, core.ParseModeMarkdown, "interactive"},
	} {
		t.Run(string(tc.domain)+"_"+tc.want, func(t *testing.T) {
			var msg struct {
				MsgType string `json:"msg_type"`
				Content string `json:"content"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
					fmt.Fprint(w, `{"code":0,"tenant_access_token":"test-token","expire":7200}`)
					return
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&msg))
				fmt.Fprint(w, `{"code":0,"data":{"message_id":"om_123"}}`)
			}))
			defer server.Close()
			client, _ := buildIMService("id", "secret", server.URL)
			base := core.NewBaseBot(&core.Config{})
			base.SetStatus(core.BotStatus{Ready: true})
			b := &Bot{BaseBot: base, client: client, domain: tc.domain}
			_, err := b.SendMessage(context.Background(), "ou_test", &core.SendMessageOptions{Text: "**hi**", ParseMode: tc.mode})
			require.NoError(t, err)
			require.Equal(t, tc.want, msg.MsgType)
			require.Contains(t, msg.Content, "**hi**")
		})
	}
}
