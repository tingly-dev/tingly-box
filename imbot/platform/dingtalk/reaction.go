package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Robot emotion reply: https://open.dingtalk.com/document/development/robot-reply-emotion
// The endpoint needs the conversation and robot code alongside the message id,
// which core.Bot.React does not carry, so inbound messages are remembered here.
const (
	emotionReplyPath = "/v1.0/robot/emotion/reply"
	maxMsgRefs       = 1024
)

// emotionBackground is the bubble background used for text emotions.
const emotionBackground = "im_bg_1"

// emotionsByName indexes defaultEmotions by Chinese and (lower-cased) English name.
var emotionsByName = func() map[string]emotion {
	m := make(map[string]emotion, 2*len(defaultEmotions))
	for _, e := range defaultEmotions {
		m[e.zh] = e
		m[strings.ToLower(e.en)] = e
	}
	return m
}()

func lookupEmotion(name string) (emotion, bool) {
	if e, ok := emotionsByName[name]; ok {
		return e, true
	}
	e, ok := emotionsByName[strings.ToLower(name)]
	return e, ok
}

type msgRef struct{ conversationID, robotCode string }

// msgRefs is a bounded FIFO of recent inbound messages.
type msgRefs struct {
	mu    sync.Mutex
	m     map[string]msgRef
	order []string
}

func newMsgRefs() *msgRefs { return &msgRefs{m: make(map[string]msgRef)} }

func (r *msgRefs) put(id string, ref msgRef) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[id]; !ok {
		if len(r.order) >= maxMsgRefs {
			delete(r.m, r.order[0])
			r.order = r.order[1:]
		}
		r.order = append(r.order, id)
	}
	r.m[id] = ref
}

func (r *msgRefs) get(id string) (msgRef, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref, ok := r.m[id]
	return ref, ok
}

// React replies to an inbound message with a text emotion. messageID is the
// DingTalk msgId; emoji is a default emotion name, Chinese or English (see core dingtalkReactions).
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	emo, ok := lookupEmotion(emoji)
	if !ok {
		return core.NewBotError(core.ErrPlatformError, fmt.Sprintf("dingtalk: unsupported emotion %q", emoji), false)
	}
	ref, ok := b.msgs.get(messageID)
	if !ok {
		return core.NewBotError(core.ErrPlatformError, "dingtalk react: unknown message (conversation not recorded)", false)
	}
	token, err := b.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("dingtalk react: get token: %w", err)
	}

	body, _ := json.Marshal(map[string]any{
		"robotCode":          ref.robotCode,
		"openMsgId":          messageID,
		"openConversationId": ref.conversationID,
		"emotionType":        2,
		"emotionName":        emo.zh,
		"textEmotion": map[string]string{
			"emotionId":    emo.id,
			"emotionName":  emo.zh,
			"text":         emo.zh,
			"backgroundId": emotionBackground,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiBase+emotionReplyPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dingtalk react: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk react: http: %w", scrubURLError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dingtalk react: status %d: %s", resp.StatusCode, string(data))
	}
	return nil
}
