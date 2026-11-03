package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

func TestMediaCaptionMarkdownEntities(t *testing.T) {
	for _, kind := range []string{"image", "document"} {
		t.Run(kind, func(t *testing.T) {
			var caption string
			var entities []models.MessageEntity
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseMultipartForm(1<<20))
				caption = r.FormValue("caption")
				require.NoError(t, json.Unmarshal([]byte(r.FormValue("caption_entities")), &entities))
				if kind == "image" {
					require.True(t, strings.HasSuffix(r.URL.Path, "/sendPhoto"))
				} else {
					require.True(t, strings.HasSuffix(r.URL.Path, "/sendDocument"))
				}
				fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":42,"type":"private"}}}`)
			}))
			defer server.Close()
			api, err := tgbot.New("test", tgbot.WithServerURL(server.URL), tgbot.WithSkipGetMe())
			require.NoError(t, err)
			b := &Bot{BaseBot: core.NewBaseBot(&core.Config{}), api: api, ctx: context.Background()}
			_, err = b.sendMedia(context.Background(), 42, &core.SendMessageOptions{Text: "**Hello** world", ParseMode: core.ParseModeMarkdown, Media: []core.MediaAttachment{{Type: kind, URL: "https://example.com/file"}}})
			require.NoError(t, err)
			require.Equal(t, "Hello world", caption)
			require.Len(t, entities, 1)
			require.Equal(t, models.MessageEntityTypeBold, entities[0].Type)
		})
	}
}
