package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// A local file goes up under its real name, also when the caller gave none.
func TestSendMediaUploadsUnderTheRealFileName(t *testing.T) {
	for _, tc := range []struct{ name, filename, want string }{
		{"given", "报告 v1.pdf", "报告 v1.pdf"},
		{"from path", "", "notes.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var uploaded string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseMultipartForm(1<<20))
				for _, hdrs := range r.MultipartForm.File {
					uploaded = hdrs[0].Filename
				}
				fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":42,"type":"private"}}}`)
			}))
			defer server.Close()
			api, err := tgbot.New("test", tgbot.WithServerURL(server.URL), tgbot.WithSkipGetMe())
			require.NoError(t, err)
			b := &Bot{BaseBot: core.NewBaseBot(&core.Config{}), api: api, ctx: context.Background()}
			p := filepath.Join(t.TempDir(), "notes.txt")
			require.NoError(t, os.WriteFile(p, []byte("hi"), 0o600))
			_, err = b.sendMedia(context.Background(), 42, &core.SendMessageOptions{Media: []core.MediaAttachment{{Type: "document", URL: "file://" + p, Filename: tc.filename}}})
			require.NoError(t, err)
			require.Equal(t, tc.want, uploaded)
		})
	}
}
