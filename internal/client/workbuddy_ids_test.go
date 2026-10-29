package client

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestWorkBuddyConversationAndTurnIDs(t *testing.T) {
	p := &typ.Provider{UUID: "account-one", OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerWorkBuddy}}
	session := typ.SessionID{Source: typ.SessionSourceHeader, Value: "private-session", IPBackup: "192.0.2.10"}
	rt := newWorkBuddyRoundTripper(http.DefaultTransport, p, session)
	headers := func(body string) http.Header { h := http.Header{}; rt.setChatIDs(h, []byte(body)); return h }
	first := `{"messages":[{"role":"user","content":"continue"}]}`
	h1 := headers(first)
	h2 := headers(`{"messages":[{"role":"user","content":"continue"},{"role":"assistant","tool_calls":[{"id":"one"}]},{"role":"tool","tool_call_id":"one","content":"output"}]}`)
	h3 := headers(`{"messages":[{"role":"user","content":"continue"},{"role":"assistant","content":"done"},{"role":"user","content":"continue"}]}`)
	require.Equal(t, h1.Get("X-Conversation-ID"), h2.Get("X-Conversation-ID"))
	require.Equal(t, h1.Get("X-Conversation-Request-ID"), h2.Get("X-Conversation-Request-ID"))
	require.NotEqual(t, h1.Get("X-Request-ID"), h2.Get("X-Request-ID"))
	require.Equal(t, h3.Get("X-Conversation-ID"), h1.Get("X-Conversation-ID"))
	require.NotEqual(t, h3.Get("X-Conversation-Request-ID"), h1.Get("X-Conversation-Request-ID"))
	for _, key := range []string{"X-Conversation-ID", "X-Conversation-Request-ID", "X-Conversation-Message-ID", "X-Request-ID"} {
		require.Regexp(t, `^[a-f0-9]{32}$`, h1.Get(key))
	}
	require.Equal(t, h1.Get("X-Request-ID"), h1.Get("X-Conversation-Message-ID"))
	// A client cached/recreated for the same session stays in the conversation;
	// changing only the backup IP has no effect.
	session.IPBackup = "192.0.2.99"
	for _, tc := range []struct {
		session  typ.SessionID
		provider string
		same     bool
	}{
		{session, "account-one", true},
		{typ.SessionID{Source: typ.SessionSourceHeader, Value: "another"}, "account-one", false},
		{session, "another-account", false},
	} {
		h := http.Header{}
		p2 := *p
		p2.UUID = tc.provider
		newWorkBuddyRoundTripper(http.DefaultTransport, &p2, tc.session).setChatIDs(h, []byte(first))
		require.Equal(t, tc.same, h.Get("X-Conversation-ID") == h1.Get("X-Conversation-ID"))
	}
	for _, s := range []typ.SessionID{{}, {Source: typ.SessionSourceIP, Value: "192.0.2.10"}} {
		h, next := http.Header{}, http.Header{}
		rt := newWorkBuddyRoundTripper(http.DefaultTransport, p, s)
		rt.setChatIDs(h, []byte(first))
		rt.setChatIDs(next, []byte(first))
		for _, k := range []string{"X-Conversation-ID", "X-Conversation-Request-ID", "X-Conversation-Message-ID"} {
			require.Regexp(t, `^[a-f0-9]{32}$`, h.Get(k))
			if k == "X-Conversation-Message-ID" {
				require.NotEqual(t, h.Get(k), next.Get(k))
			} else {
				require.Equal(t, h.Get(k), next.Get(k))
			}
		}
	}
	explicit := http.Header{}
	for _, k := range []string{"X-Conversation-ID", "X-Conversation-Request-ID", "X-Request-ID", "X-Conversation-Message-ID"} {
		explicit.Set(k, "caller-"+k)
	}
	before := explicit.Clone()
	rt.setChatIDs(explicit, []byte(first))
	require.Equal(t, before, explicit)
}

func TestWorkBuddyTurnIdentityIgnoresToolOutput(t *testing.T) {
	first := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"image-a"}}]}]}`)
	withResults := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"image-a"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"tool output"}]}]}`)
	require.Equal(t, workbuddyTurnKey(first), workbuddyTurnKey(withResults))
	other := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"image-b"}}]}]}`)
	require.NotEqual(t, workbuddyTurnKey(first), workbuddyTurnKey(other))
}

func TestWorkBuddyInstructionConversationFallback(t *testing.T) {
	p := &typ.Provider{UUID: "account-one"}
	id := func(body string, provider *typ.Provider) string {
		h := http.Header{}
		newWorkBuddyRoundTripper(http.DefaultTransport, provider, typ.SessionID{}).setChatIDs(h, []byte(body))
		return h.Get("X-Conversation-ID")
	}
	first := `{"messages":[{"role":"user","content":"hello"}]}`
	want := id(first, p)
	require.Equal(t, want, id(`{"messages":[{"role":"system","content":"changed"},{"role":"developer","content":"instructions"},{"role":"user","content":"hello"},{"role":"assistant","content":"reply"},{"role":"user","content":"next"}]}`, p))
	require.Equal(t, want, id(`{ "messages": [{"content":"h\u0065llo", "role":"user"}] }`, p))
	require.NotEqual(t, want, id(`{"messages":[{"role":"user","content":"other"}]}`, p))
	require.NotEqual(t, want, id(first, &typ.Provider{UUID: "account-two"}))
	require.NotEqual(t, want, id(`{"messages":[{"role":"assistant","content":"hello"}]}`, p))
	a := `{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"image-a"}}]}]}`
	b := `{"messages":[{"role":"user","content":[{ "image_url": {"url": "image-a"}, "type": "image_url"}]}]}`
	require.Equal(t, id(a, p), id(b, p))
	require.NotEqual(t, id(a, p), id(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"image-b"}}]}]}`, p))
	empty := `{"messages":[{"role":"system","content":"instructions"}]}`
	require.NotEqual(t, id(empty, p), id(empty, p))
}
