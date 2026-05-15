package oauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGeminiHook_AfterToken_LoadCodeAssistDirect verifies that GeminiHook
// extracts cloudaicompanionProject from a loadCodeAssist response directly
// without needing the onboardUser fallback.
func TestGeminiHook_AfterToken_LoadCodeAssistDirect(t *testing.T) {
	var loadCalled, onboardCalled bool

	// Stub the Code Assist endpoint. The userinfo call goes to a different host
	// and will fail in tests — that's expected; only project_id matters here.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			loadCalled = true
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("expected Bearer auth, got %q", got)
			}
			_, _ = io.WriteString(w, `{"cloudaicompanionProject":"discovered-project","currentTier":{"id":"standard-tier"}}`)
		case strings.HasSuffix(r.URL.Path, ":onboardUser"):
			onboardCalled = true
			_, _ = io.WriteString(w, `{"done":true,"response":{"cloudaicompanionProject":{"id":"onboarded"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Redirect Code Assist calls to the test server by using its URL as the
	// endpoint constant. We do this with a transport that rewrites the host.
	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"cloudcode-pa.googleapis.com": true, "www.googleapis.com": true},
	}}

	hook := &GeminiHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}

	if !loadCalled {
		t.Error("expected loadCodeAssist to be called")
	}
	if onboardCalled {
		t.Error("expected onboardUser NOT to be called when project is returned directly")
	}
	if got := meta["project_id"]; got != "discovered-project" {
		t.Errorf("expected project_id=discovered-project, got %v", got)
	}
	if got := meta["user_tier"]; got != "standard-tier" {
		t.Errorf("expected user_tier=standard-tier (from currentTier), got %v", got)
	}
	if got, ok := meta["onboarded"].(bool); !ok || got {
		t.Errorf("expected onboarded=false when project came from loadCodeAssist, got %v", meta["onboarded"])
	}
}

// TestGeminiHook_AfterToken_OnboardFallback verifies that when loadCodeAssist
// returns no project, GeminiHook falls back to onboardUser and extracts the
// project id from the long-running operation response.
func TestGeminiHook_AfterToken_OnboardFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			_, _ = io.WriteString(w, `{"allowedTiers":[{"id":"free-tier","isDefault":true}]}`)
		case strings.HasSuffix(r.URL.Path, ":onboardUser"):
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			_ = json.Unmarshal(body, &parsed)
			if parsed["tierId"] != "free-tier" {
				t.Errorf("expected tierId=free-tier picked from default tier, got %v", parsed["tierId"])
			}
			_, _ = io.WriteString(w, `{"done":true,"response":{"cloudaicompanionProject":{"id":"onboarded-project"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	httpClient := &http.Client{Transport: &hostRewriteTransport{
		base:   http.DefaultTransport,
		target: srv.URL,
		hosts:  map[string]bool{"cloudcode-pa.googleapis.com": true, "www.googleapis.com": true},
	}}

	hook := &GeminiHook{}
	meta, err := hook.AfterToken(context.Background(), "tok", httpClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if got := meta["project_id"]; got != "onboarded-project" {
		t.Errorf("expected project_id=onboarded-project, got %v", got)
	}
	if got, ok := meta["onboarded"].(bool); !ok || !got {
		t.Errorf("expected onboarded=true when onboardUser created the project, got %v", meta["onboarded"])
	}
	if got := meta["user_tier"]; got != "free-tier" {
		t.Errorf("expected user_tier=free-tier (default tier), got %v", got)
	}
}

// hostRewriteTransport reroutes requests for any host in `hosts` to `target`
// (preserving the path/query). Used by tests so we don't need to monkey-patch
// the Code Assist endpoint constants.
type hostRewriteTransport struct {
	base   http.RoundTripper
	target string
	hosts  map[string]bool
}

func (h *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if h.hosts[req.URL.Host] {
		u := *req.URL
		// Replace scheme+host with the test server's.
		parsed, err := http.NewRequest(req.Method, h.target+req.URL.RequestURI(), req.Body)
		if err != nil {
			return nil, err
		}
		parsed.Header = req.Header.Clone()
		_ = u
		return h.base.RoundTrip(parsed)
	}
	return h.base.RoundTrip(req)
}
