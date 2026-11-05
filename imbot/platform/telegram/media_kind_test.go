package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

func TestSendMediaNeverSendsNonPhotoFormatsAsPhoto(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return (&url.URL{Scheme: "file", Path: p}).String()
	}
	cases := []struct {
		name   string
		url    string
		typ    string
		method string
	}{
		{"png photo", write("a.png", "\x89PNG\r\n\x1a\nx"), "image", "/sendPhoto"},
		{"jpeg photo", write("a.jpg", "\xff\xd8\xffx"), "image", "/sendPhoto"},
		{"svg tagged image goes as document", write("a.svg", "<svg/>"), "image", "/sendDocument"},
		{"svgz tagged image goes as document", write("a.svgz", "x"), "image", "/sendDocument"},
		{"gif tagged image goes as document", write("a.gif", "GIF89a\x01\x00\x01\x00"), "image", "/sendDocument"},
		{"extensionless file tagged image goes as document", write("noext", "x"), "image", "/sendDocument"},
		{"document stays document", write("a.pdf", "%PDF"), "document", "/sendDocument"},
		{"remote image URL keeps photo", "https://example.com/pic", "image", "/sendPhoto"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var path string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path = r.URL.Path
				fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":42,"type":"private"}}}`)
			}))
			defer server.Close()
			api, err := tgbot.New("test", tgbot.WithServerURL(server.URL), tgbot.WithSkipGetMe())
			require.NoError(t, err)
			b := &Bot{BaseBot: core.NewBaseBot(&core.Config{}), api: api, ctx: context.Background()}
			_, err = b.sendMedia(context.Background(), 42, &core.SendMessageOptions{Media: []core.MediaAttachment{{Type: c.typ, URL: c.url}}})
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(path, c.method), "got %s, want %s", path, c.method)
		})
	}
}
