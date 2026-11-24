package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReactSendsEmotionReply(t *testing.T) {
	var got map[string]any
	var path, tok string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, tok = r.URL.Path, r.Header.Get("x-acs-dingtalk-access-token")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	b := &Bot{msgs: newMsgRefs(), apiBase: srv.URL, cachedToken: "T", tokenExpiry: farFuture()}
	b.msgs.put("m1", msgRef{conversationID: "cid", robotCode: "rc"})

	if err := b.React(context.Background(), "m1", "RogerThat"); err != nil {
		t.Fatal(err)
	}
	if path != emotionReplyPath || tok != "T" {
		t.Fatalf("path=%s tok=%s", path, tok)
	}
	if got["robotCode"] != "rc" || got["openMsgId"] != "m1" || got["openConversationId"] != "cid" || got["emotionType"] != float64(2) || got["emotionName"] != "收到" {
		t.Fatalf("body=%v", got)
	}
}

func TestReactErrors(t *testing.T) {
	b := &Bot{msgs: newMsgRefs()}
	if err := b.React(context.Background(), "m1", "RogerThat"); err == nil {
		t.Fatal("expected unknown-message error")
	}
	if err := b.React(context.Background(), "m1", "NoSuchEmotion"); err == nil {
		t.Fatal("expected unsupported emotion error")
	}
}
func farFuture() time.Time { return time.Now().Add(time.Hour) }

func TestLookupEmotion(t *testing.T) {
	for _, n := range []string{"RogerThat", "收到", "ok", "Wrong", "Love", "Cry", "Like"} {
		if _, ok := lookupEmotion(n); !ok {
			t.Errorf("%s not found", n)
		}
	}
	if e, _ := lookupEmotion("RogerThat"); e.id != "emotion_167" {
		t.Errorf("id=%s", e.id)
	}
	if len(defaultEmotions) != 199 {
		t.Errorf("len=%d", len(defaultEmotions))
	}
}
