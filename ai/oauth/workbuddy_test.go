package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

func writeWorkBuddy(t *testing.T, w http.ResponseWriter, code int, msg string, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{"code": code, "msg": msg}
	if data != nil {
		body["data"] = data
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

// workbuddyServer fakes a WorkBuddy site: the token is pending for the first
// two polls and the account for the first one, as a real sign-in would be.
func workbuddyServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var tokenPolls, accountPolls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/plugin/auth/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Query().Get("platform") != "workbuddy-ai" {
			t.Errorf("state: %s platform=%q", r.Method, r.URL.Query().Get("platform"))
		}
		if r.Header.Get("X-Domain") != "www.workbuddy.ai" || r.Header.Get("X-Product-Code") != "workbuddy" {
			t.Errorf("state: identification headers missing: %v", r.Header)
		}
		writeWorkBuddy(t, w, 0, "ok", map[string]string{"state": "st-1", "authUrl": "https://www.workbuddy.ai/login?state=st-1"})
	})
	mux.HandleFunc("/v2/plugin/auth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "st-1" {
			t.Errorf("token poll state = %q", r.URL.Query().Get("state"))
		}
		if tokenPolls.Add(1) <= 2 {
			writeWorkBuddy(t, w, workbuddyCodeTokenPending, "waiting", nil)
			return
		}
		writeWorkBuddy(t, w, 0, "ok", map[string]any{"accessToken": "at-1", "refreshToken": "rt-1", "expiresIn": 3600})
	})
	mux.HandleFunc("/v2/plugin/login/account", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-1" {
			t.Errorf("account: Authorization = %q", r.Header.Get("Authorization"))
		}
		if accountPolls.Add(1) == 1 {
			writeWorkBuddy(t, w, workbuddyCodeAccountPending, "waiting", nil)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"uid":100000123456789012,"nickname":"Alice"}}`))
	})
	mux.HandleFunc("/v2/plugin/auth/token/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("refresh method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer at-1" || r.Header.Get("X-User-Id") != "u-42" {
			t.Errorf("refresh: credential headers = %q / %q", r.Header.Get("Authorization"), r.Header.Get("X-User-Id"))
		}
		if r.Header.Get("X-Refresh-Token") != "rt-1" {
			writeWorkBuddy(t, w, 401, "invalid refresh token", nil)
			return
		}
		writeWorkBuddy(t, w, 0, "ok", map[string]any{"access_token": "at-2", "refresh_token": "rt-2", "expires_in": "7200"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &tokenPolls
}

func TestWorkBuddyLoginFlow(t *testing.T) {
	server, tokenPolls := workbuddyServer(t)
	manager := NewManager()
	opts := []Option{WithHTTPClient(server.Client()), WithWorkBuddyEndpoint(server.URL, time.Millisecond)}

	flow, err := manager.InitiateServerPollFlow(context.Background(), "user-1", ai.IssuerWorkBuddy, "/done", "", opts...)
	if err != nil {
		t.Fatalf("InitiateServerPollFlow: %v", err)
	}
	if flow.AuthorizeURL != "https://www.workbuddy.ai/login?state=st-1" || flow.FlowID != "st-1" {
		t.Errorf("flow = %+v", flow)
	}

	token, err := manager.CompleteServerPollFlow(context.Background(), flow, opts...)
	if err != nil {
		t.Fatalf("CompleteServerPollFlow: %v", err)
	}
	if got := tokenPolls.Load(); got != 3 {
		t.Errorf("token polls = %d, want 3 (two pending rounds)", got)
	}
	if token.AccessToken != "at-1" || token.RefreshToken != "rt-1" || token.Issuer != ai.IssuerWorkBuddy {
		t.Errorf("token = %+v", token)
	}
	if left := time.Until(token.Expiry); left < 59*time.Minute || left > time.Hour {
		t.Errorf("expiry in %v, want ~1h", left)
	}
	// A numeric uid past 2^53 survives exactly.
	if token.Metadata[WorkBuddyMetaUserID] != "100000123456789012" || token.Metadata["account_id"] != "100000123456789012" {
		t.Errorf("metadata = %v", token.Metadata)
	}
	if token.Name != "Alice" {
		t.Errorf("name = %q, want the account nickname", token.Name)
	}
}

// Refresh goes through Manager.RefreshToken — the entry point both the
// background refresher and POST /oauth/refresh use — not an OAuth grant.
func TestWorkBuddyRefreshViaManager(t *testing.T) {
	server, _ := workbuddyServer(t)
	manager := NewManager()
	opts := []Option{WithHTTPClient(server.Client()), WithWorkBuddyEndpoint(server.URL, time.Millisecond), WithWorkBuddyCredential("at-1", "u-42")}

	token, err := manager.RefreshToken(context.Background(), "user-1", ai.IssuerWorkBuddy, "rt-1", opts...)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if token.AccessToken != "at-2" || token.RefreshToken != "rt-2" {
		t.Errorf("token = %+v", token)
	}
	if left := time.Until(token.Expiry); left < 119*time.Minute || left > 2*time.Hour {
		t.Errorf("expiry in %v, want ~2h", left)
	}

	_, err = manager.RefreshToken(context.Background(), "user-1", ai.IssuerWorkBuddy, "revoked", opts...)
	if err == nil || !strings.Contains(err.Error(), "invalid refresh token") {
		t.Errorf("revoked refresh err = %v, want the server's message", err)
	}
}

// A rejection other than "not ready" ends the login at once with the server's
// message instead of polling until the deadline.
func TestWorkBuddyPollFatalError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/plugin/auth/token", func(w http.ResponseWriter, _ *http.Request) {
		writeWorkBuddy(t, w, 11218, "state expired", nil)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	site, _ := ai.WorkBuddySiteFor(ai.IssuerWorkBuddyCN)
	site.Origin = server.URL
	client := NewWorkBuddyClient(server.Client(), site)
	client.PollInterval = time.Millisecond

	_, err := client.PollToken(context.Background(), &WorkBuddyFlow{State: "st"}, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "state expired") {
		t.Fatalf("err = %v, want the server's message", err)
	}
}

// Transport blips and 5xx are one more round, never the end of a login the
// user is in the middle of.
func TestWorkBuddyPollRetriesTransient(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/plugin/auth/token", func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>bad gateway</html>"))
		default:
			writeWorkBuddy(t, w, 0, "ok", map[string]any{"accessToken": "at"})
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	site, _ := ai.WorkBuddySiteFor(ai.IssuerWorkBuddyCN)
	site.Origin = server.URL
	client := NewWorkBuddyClient(server.Client(), site)
	client.PollInterval = time.Millisecond

	token, err := client.PollToken(context.Background(), &WorkBuddyFlow{State: "st"}, time.Minute)
	if err != nil || token.AccessToken != "at" {
		t.Fatalf("token=%v err=%v", token, err)
	}
}

func TestWorkBuddyExpiry(t *testing.T) {
	abs := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, abs.Unix())))
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	jwt := header + "." + payload + ".sig"

	cases := map[string]struct {
		access    string
		expiresIn []json.RawMessage
		expiresAt []json.RawMessage
		want      time.Time
	}{
		"unix seconds": {expiresAt: []json.RawMessage{json.RawMessage(fmt.Sprint(abs.Unix()))}, want: abs},
		"unix millis":  {expiresAt: []json.RawMessage{json.RawMessage(fmt.Sprint(abs.UnixMilli()))}, want: abs},
		"numeric str":  {expiresAt: []json.RawMessage{json.RawMessage(fmt.Sprintf("%q", fmt.Sprint(abs.Unix())))}, want: abs},
		"rfc3339":      {expiresAt: []json.RawMessage{json.RawMessage(fmt.Sprintf("%q", abs.Format(time.RFC3339)))}, want: abs},
		"jwt exp":      {access: jwt, want: abs},
		"unknown":      {access: "opaque"},
	}
	for name, tc := range cases {
		got := workbuddyExpiry(tc.access, tc.expiresIn, tc.expiresAt)
		if !got.Equal(tc.want) {
			t.Errorf("%s: expiry = %v, want %v", name, got, tc.want)
		}
	}
}

func TestParseWorkBuddyAccount(t *testing.T) {
	got := parseWorkBuddyAccount(map[string]any{"account": map[string]any{"userId": float64(1234), "name": "Bob", "enterpriseDomain": "acme.copilot.qq.com"}})
	if got.UserID != "1234" || got.Nickname != "Bob" || got.EnterpriseDomain != "acme.copilot.qq.com" {
		t.Errorf("account = %+v", got)
	}
}
