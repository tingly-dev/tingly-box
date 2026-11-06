package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// dingTalkInbound boots a bot against a fake DingTalk whose messageFiles/download
// answers with a link into the same server, and returns what the bot emits for
// one callback.
func dingTalkInbound(t *testing.T, data *chatbot.BotCallbackDataModel) (core.Message, []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var downloads []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/gettoken", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":0,"access_token":"tok","expires_in":7200}`))
	})
	var srv *httptest.Server
	mux.HandleFunc("/v1.0/robot/messageFiles/download", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("x-acs-dingtalk-access-token") != "tok" {
			t.Errorf("download without token")
		}
		mu.Lock()
		downloads = append(downloads, body)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"downloadUrl": srv.URL + "/oss/" + body["downloadCode"].(string)})
	})
	srv = httptest.NewServer(mux)
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
	bot.adapter = NewAdapter(bot.Config())
	emitted := make(chan core.Message, 1)
	bot.OnMessage(func(m core.Message) { emitted <- m })
	if _, err := bot.onChatBotMessage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-emitted:
		mu.Lock()
		defer mu.Unlock()
		return got, downloads
	case <-time.After(3 * time.Second):
		t.Fatal("no message emitted")
		return core.Message{}, nil
	}
}

func TestInbound_FileKeepsItsName(t *testing.T) {
	msg, downloads := dingTalkInbound(t, &chatbot.BotCallbackDataModel{
		ConversationId: "cid", ConversationType: "1", SenderStaffId: "staffA", RobotCode: "robot-key",
		Msgtype: "file",
		Content: map[string]any{"fileName": "季度报告 final.pdf", "fileId": "f1", "spaceId": "s1", "downloadCode": "dc-file"},
	})
	mc, ok := msg.Content.(*core.MediaContent)
	if !ok {
		t.Fatalf("content = %T, want media", msg.Content)
	}
	m := mc.Media[0]
	if m.Filename != "季度报告 final.pdf" {
		t.Errorf("Filename = %q", m.Filename)
	}
	if m.MimeType != "application/pdf" || m.Type != "document" {
		t.Errorf("type = %q / %q", m.Type, m.MimeType)
	}
	if m.URL == "" || len(downloads) != 1 || downloads[0]["downloadCode"] != "dc-file" || downloads[0]["robotCode"] != "robot-key" {
		t.Errorf("url = %q, downloads = %v", m.URL, downloads)
	}
}

func TestInbound_PictureHasNoNameForTheStoreToInvent(t *testing.T) {
	msg, _ := dingTalkInbound(t, &chatbot.BotCallbackDataModel{
		ConversationId: "cid", ConversationType: "1", RobotCode: "robot-key", Msgtype: "picture",
		Content: map[string]any{"downloadCode": "dc-pic", "pictureDownloadCode": "dc-pic"},
	})
	mc, ok := msg.Content.(*core.MediaContent)
	if !ok {
		t.Fatalf("content = %T, want media", msg.Content)
	}
	m := mc.Media[0]
	if m.Type != "image" || m.Filename != "" || m.URL == "" {
		t.Errorf("media = %+v", m)
	}
}

func TestInbound_RichTextKeepsTextAndPictures(t *testing.T) {
	msg, _ := dingTalkInbound(t, &chatbot.BotCallbackDataModel{
		ConversationId: "cid", ConversationType: "2", RobotCode: "robot-key", Msgtype: "richText",
		Content: map[string]any{"richText": []any{
			map[string]any{"text": "see this "},
			map[string]any{"type": "picture", "downloadCode": "dc-1"},
			map[string]any{"text": "and that"},
		}},
	})
	mc, ok := msg.Content.(*core.MediaContent)
	if !ok {
		t.Fatalf("content = %T, want media", msg.Content)
	}
	if mc.Caption != "see this and that" || len(mc.Media) != 1 || mc.Media[0].Type != "image" {
		t.Errorf("caption = %q, media = %+v", mc.Caption, mc.Media)
	}
}

func TestInbound_FileWithoutDownloadCodeStillArrives(t *testing.T) {
	msg, _ := dingTalkInbound(t, &chatbot.BotCallbackDataModel{
		ConversationId: "cid", ConversationType: "1", RobotCode: "robot-key", Msgtype: "file",
		Content: map[string]any{"fileName": "a.txt"},
	})
	if mc, ok := msg.Content.(*core.MediaContent); !ok || mc.Media[0].Filename != "a.txt" {
		t.Errorf("content = %#v", msg.Content)
	}
}

func TestInbound_VideoAndAudioAreNotMedia(t *testing.T) {
	for _, typ := range []string{"video", "audio"} {
		msg, downloads := dingTalkInbound(t, &chatbot.BotCallbackDataModel{
			ConversationId: "cid", ConversationType: "1", RobotCode: "robot-key", Msgtype: typ,
			Content: map[string]any{"downloadCode": "dc", "recognition": "hello"},
		})
		if _, ok := msg.Content.(*core.MediaContent); ok || len(downloads) != 0 {
			t.Errorf("%s: content = %T, downloads = %v", typ, msg.Content, downloads)
		}
	}
}
