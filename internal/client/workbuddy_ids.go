package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Process-local salt keeps header IDs opaque: neither prompts, session IDs,
// account IDs nor client IPs are sent as correlation identifiers.
var workbuddyIDSalt = uuid.NewString()

func workbuddyID(kind, key string) string {
	digest := sha256.Sum256([]byte(workbuddyIDSalt + "|" + kind + "|" + key))
	return hex.EncodeToString(digest[:16])
}

func workbuddyRequestIDs(h http.Header) {
	msg := h.Get("X-Conversation-Message-ID")
	if msg == "" {
		msg = h.Get("X-Request-ID")
	}
	if msg == "" {
		msg = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	if h.Get("X-Conversation-Message-ID") == "" {
		h.Set("X-Conversation-Message-ID", msg)
	}
	if h.Get("X-Request-ID") == "" {
		h.Set("X-Request-ID", msg)
	}
}

func (t *workbuddyRoundTripper) setChatIDs(h http.Header, body []byte) {
	workbuddyRequestIDs(h)
	conversation := h.Get("X-Conversation-ID")
	if conversation == "" && t.sessionScope != "" {
		conversation = workbuddyID("conversation", t.sessionScope)
		h.Set("X-Conversation-ID", conversation)
	}
	if h.Get("X-Conversation-Request-ID") != "" {
		return
	}
	// Without a real session (or the caller's conversation ID), do not merge
	// unrelated chats by their shared IP or a common first message like "hi".
	turn := workbuddyTurnKey(body)
	if conversation != "" && turn != "" {
		h.Set("X-Conversation-Request-ID", workbuddyID("turn", conversation+"|"+turn))
	} else {
		h.Set("X-Conversation-Request-ID", strings.ReplaceAll(uuid.NewString(), "-", ""))
	}
}

// A user turn stays the same as tool results/assistant steps are appended.
// Count user contributions as well as their content so a repeated "continue"
// starts a new turn. Explicit upstream IDs win when a client supplies them.
func workbuddyTurnKey(body []byte) string {
	_, _, messages := workbuddyChatMessages(body)
	count, last := 0, ""
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		var text string
		if json.Unmarshal(m.Content, &text) != nil {
			var parts []map[string]json.RawMessage
			if json.Unmarshal(m.Content, &parts) != nil {
				continue
			}
			kept := make([]map[string]json.RawMessage, 0, len(parts))
			for _, p := range parts {
				if string(p["type"]) != `"tool_result"` {
					kept = append(kept, p)
				}
			}
			if len(kept) == 0 {
				continue
			}
			raw, _ := json.Marshal(kept)
			text = string(raw)
		}
		if text != "" {
			count++
			last = text
		}
	}
	if count == 0 {
		return ""
	}
	return strconv.Itoa(count) + "|" + last
}
