package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// newZCodeTestClient points a client's control plane and business API at one
// test server, so a single handler can serve the whole flow.
func newZCodeTestClient(t *testing.T, variant string, handler http.Handler) *ZCodeClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := NewZCodeClient(server.Client(), variant)
	client.APIBase = server.URL + "/api/v1"
	client.BizHost = server.URL
	return client
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal envelope data: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"code":0,"msg":"","data":%s}`, raw)
}

func TestZCodeVariantMapping(t *testing.T) {
	if got := ZCodeVariant(ai.IssuerZCode); got != ZCodeVariantZai {
		t.Errorf("ZCodeVariant(zcode) = %q, want %q", got, ZCodeVariantZai)
	}
	if got := ZCodeVariant(ai.IssuerZCodeCN); got != ZCodeVariantBigModel {
		t.Errorf("ZCodeVariant(zcode_cn) = %q, want %q", got, ZCodeVariantBigModel)
	}
	if got := ZCodeVariant(ai.IssuerClaudeCode); got != "" {
		t.Errorf("ZCodeVariant(claude_code) = %q, want empty", got)
	}
	if IsZCodeIssuer(ai.IssuerKimiCode) {
		t.Error("IsZCodeIssuer(kimi_code) = true, want false")
	}
}

// The authorize URL the server returns must be handed to the browser with the
// zcode.z.ai interstitial appended, since that page is what records the
// authorization server-side and flips the poll to "ready". The parameter name
// differs per platform.
func TestZCodeStartAppliesInterstitial(t *testing.T) {
	for _, tc := range []struct {
		variant string
		param   string
	}{
		{ZCodeVariantZai, "redirect_uri"},
		{ZCodeVariantBigModel, "redirect"},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			var gotBody map[string]string
			var gotAuth string
			client := newZCodeTestClient(t, tc.variant, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/oauth/cli/init" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				gotAuth = r.Header.Get("Authorization")
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				writeEnvelope(t, w, map[string]any{
					"flow_id":           "flow-1",
					"authorize_url":     "https://chat.z.ai/authorize?client_id=zcode",
					"expires_at":        time.Now().Add(5 * time.Minute).Unix(),
					"poll_interval_sec": 2,
				})
			}))

			flow, err := client.Start(context.Background())
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			if gotBody["provider"] != tc.variant {
				t.Errorf("init body provider = %q, want %q", gotBody["provider"], tc.variant)
			}
			if !strings.HasPrefix(gotAuth, "Bearer ") || len(gotAuth) < len("Bearer ")+64 {
				t.Errorf("init Authorization = %q, want a 32-byte hex bearer", gotAuth)
			}
			if flow.PollToken == "" || !strings.HasSuffix(gotAuth, flow.PollToken) {
				t.Errorf("poll token %q not the bearer sent on init (%q)", flow.PollToken, gotAuth)
			}

			parsed, err := url.Parse(flow.AuthorizeURL)
			if err != nil {
				t.Fatalf("parse authorize URL: %v", err)
			}
			if parsed.Query().Get("client_id") != "zcode" {
				t.Error("interstitial dropped the server's own query parameters")
			}
			interstitial := parsed.Query().Get(tc.param)
			if interstitial == "" {
				t.Fatalf("authorize URL has no %s parameter: %s", tc.param, flow.AuthorizeURL)
			}
			inner, err := url.Parse(interstitial)
			if err != nil {
				t.Fatalf("parse interstitial: %v", err)
			}
			if inner.Path != "/app/oauth/login" {
				t.Errorf("interstitial path = %q, want /app/oauth/login", inner.Path)
			}
			if got := inner.Query().Get("redirect"); got != "zcode://oauth/callback" {
				t.Errorf("interstitial redirect = %q", got)
			}
			if got := inner.Query().Get("app_version"); got != ZCodeAppVersion {
				t.Errorf("interstitial app_version = %q, want %q", got, ZCodeAppVersion)
			}
			if flow.PollInterval != 2*time.Second {
				t.Errorf("poll interval = %v, want 2s", flow.PollInterval)
			}
		})
	}
}

// An envelope error (code != 0) on a 200 is still a failure: the HTTP status
// alone is not authoritative on this API.
func TestZCodeStartRejectsEnvelopeError(t *testing.T) {
	client := newZCodeTestClient(t, ZCodeVariantZai, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":1001,"msg":"client disabled"}`)
	}))

	_, err := client.Start(context.Background())
	if err == nil {
		t.Fatal("Start succeeded on envelope error")
	}
	if !strings.Contains(err.Error(), "client disabled") {
		t.Errorf("error %q does not carry the server message", err)
	}
}

// A flow with no expires_at must not inherit a 1970 deadline, which would make
// the very first poll report a timeout.
func TestZCodeStartWithoutExpiryPolls(t *testing.T) {
	var polls int32
	client := newZCodeTestClient(t, ZCodeVariantZai, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth/cli/init"):
			writeEnvelope(t, w, map[string]any{
				"flow_id":           "flow-1",
				"authorize_url":     "https://chat.z.ai/authorize",
				"poll_interval_sec": 1,
			})
		case strings.Contains(r.URL.Path, "/oauth/cli/poll/"):
			atomic.AddInt32(&polls, 1)
			writeEnvelope(t, w, map[string]any{"status": "failed"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))

	flow, err := client.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !flow.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero when unreported", flow.ExpiresAt)
	}
	if _, err := client.Poll(context.Background(), flow, time.Minute); err == nil {
		t.Fatal("Poll succeeded on status=failed")
	}
	if got := atomic.LoadInt32(&polls); got != 1 {
		t.Errorf("polled %d times, want 1 (the flow must not be pre-expired)", got)
	}
}

// Pending rounds and 5xx blips both keep the flow alive; only the terminal
// statuses end it.
func TestZCodePollSurvivesPendingAndServerError(t *testing.T) {
	var calls int32
	client := newZCodeTestClient(t, ZCodeVariantZai, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			writeEnvelope(t, w, map[string]any{"status": "pending"})
		case 2:
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `{"code":5000,"msg":"bad gateway"}`)
		default:
			writeEnvelope(t, w, map[string]any{
				"status": "ready",
				"token":  "plan-jwt",
				"user":   map[string]string{"user_id": "u-1"},
				"zai":    map[string]string{"access_token": "account-token"},
			})
		}
	}))

	flow := &ZCodeFlow{FlowID: "flow-1", PollToken: "tok", Variant: ZCodeVariantZai, PollInterval: time.Second}
	tokens, err := client.Poll(context.Background(), flow, 30*time.Second)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if tokens.AccountToken != "account-token" || tokens.JWT != "plan-jwt" || tokens.UserID != "u-1" {
		t.Errorf("tokens = %+v", tokens)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("polled %d times, want 3", got)
	}
}

// A 4xx is the flow being rejected, not a blip: retrying it would hang the
// login until the deadline instead of reporting what went wrong.
func TestZCodePollFatalOnClientError(t *testing.T) {
	var calls int32
	client := newZCodeTestClient(t, ZCodeVariantZai, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"code":403,"msg":"flow not found"}`)
	}))

	flow := &ZCodeFlow{FlowID: "flow-1", PollToken: "tok", Variant: ZCodeVariantZai, PollInterval: time.Second}
	_, err := client.Poll(context.Background(), flow, 30*time.Second)
	if err == nil {
		t.Fatal("Poll succeeded on 403")
	}
	if !strings.Contains(err.Error(), "flow not found") {
		t.Errorf("error %q does not carry the server message", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("polled %d times, want 1 (a 4xx must not be retried)", got)
	}
}

// Reading the other platform's token field would persist a credential aimed at
// the wrong vendor, so a ready response missing this platform's token is an
// error rather than a silent cross-platform fallback.
func TestZCodePollRejectsOtherPlatformToken(t *testing.T) {
	client := newZCodeTestClient(t, ZCodeVariantBigModel, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, map[string]any{
			"status": "ready",
			"zai":    map[string]string{"access_token": "zai-only"},
		})
	}))

	flow := &ZCodeFlow{FlowID: "flow-1", PollToken: "tok", Variant: ZCodeVariantBigModel, PollInterval: time.Second}
	_, err := client.Poll(context.Background(), flow, 30*time.Second)
	if err == nil {
		t.Fatal("Poll accepted a zai token for a bigmodel flow")
	}
	if !strings.Contains(err.Error(), "bigmodel") {
		t.Errorf("error %q should name the platform whose token is missing", err)
	}
}

// zcodeBizHandler serves the business API the credential exchange walks.
type zcodeBizHandler struct {
	t          *testing.T
	listKeys   []map[string]string
	listStatus int
	secret     string
	created    int32
	authSeen   []string
}

func (h *zcodeBizHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.authSeen = append(h.authSeen, r.Header.Get("Authorization"))
	switch {
	case r.URL.Path == "/api/auth/z/login":
		writeEnvelope(h.t, w, map[string]string{"access_token": "biz-token"})
	case r.URL.Path == "/api/biz/customer/getCustomerInfo":
		writeEnvelope(h.t, w, map[string]any{
			"organizations": []map[string]any{
				{"organizationId": "org-other", "organizationName": "Team", "projects": []map[string]string{{"projectId": "p-other", "projectName": "Team"}}},
				{"organizationId": "org-1", "organizationName": "默认机构", "projects": []map[string]string{
					{"projectId": "p-other", "projectName": "Scratch"},
					{"projectId": "p-1", "projectName": "默认项目"},
				}},
			},
		})
	case strings.HasSuffix(r.URL.Path, "/api_keys") && r.Method == http.MethodGet:
		if h.listStatus != 0 {
			w.WriteHeader(h.listStatus)
			fmt.Fprint(w, `{"msg":"forbidden"}`)
			return
		}
		writeEnvelope(h.t, w, h.listKeys)
	case strings.HasSuffix(r.URL.Path, "/api_keys") && r.Method == http.MethodPost:
		atomic.AddInt32(&h.created, 1)
		writeEnvelope(h.t, w, map[string]string{"apiKey": "created-key"})
	case strings.Contains(r.URL.Path, "/api_keys/copy/"):
		writeEnvelope(h.t, w, map[string]string{"secretKey": h.secret})
	default:
		h.t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

// Z.ai trades the account token for a business token first; BigModel sends the
// account token to the business API as-is.
func TestZCodeResolveCredential(t *testing.T) {
	t.Run("zai reuses the existing plan key", func(t *testing.T) {
		handler := &zcodeBizHandler{
			t:        t,
			listKeys: []map[string]string{{"name": "other", "apiKey": "nope"}, {"name": zcodeAPIKeyName, "apiKey": "existing-key"}},
			secret:   "s3cret",
		}
		client := newZCodeTestClient(t, ZCodeVariantZai, handler)

		cred, err := client.ResolveCredential(context.Background(), "account-token")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if cred.FullKey() != "existing-key.s3cret" {
			t.Errorf("FullKey = %q, want existing-key.s3cret", cred.FullKey())
		}
		if handler.created != 0 {
			t.Error("created a new API key although the plan already had one")
		}
		if handler.authSeen[1] != "Bearer biz-token" {
			t.Errorf("business API called with %q, want the exchanged biz token", handler.authSeen[1])
		}
	})

	t.Run("bigmodel skips the z/login exchange", func(t *testing.T) {
		handler := &zcodeBizHandler{t: t, secret: "s3cret"}
		client := newZCodeTestClient(t, ZCodeVariantBigModel, handler)

		cred, err := client.ResolveCredential(context.Background(), "account-token")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if cred.FullKey() != "created-key.s3cret" {
			t.Errorf("FullKey = %q, want created-key.s3cret", cred.FullKey())
		}
		if handler.created != 1 {
			t.Errorf("created %d keys, want 1", handler.created)
		}
		for _, auth := range handler.authSeen {
			if auth != "account-token" {
				t.Errorf("business API called with %q, want the bare account token", auth)
			}
		}
	})

	// A listing the account is not allowed to read is not a reason to fail the
	// login: creating the key reaches the same end state.
	t.Run("an unreadable key list falls through to create", func(t *testing.T) {
		handler := &zcodeBizHandler{t: t, listStatus: http.StatusForbidden, secret: "s3cret"}
		client := newZCodeTestClient(t, ZCodeVariantBigModel, handler)

		cred, err := client.ResolveCredential(context.Background(), "account-token")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if cred.APIKey != "created-key" {
			t.Errorf("APIKey = %q, want created-key", cred.APIKey)
		}
	})

	// The plan endpoints reject the bare key, so a login that cannot read the
	// secret must fail here instead of surfacing later as an opaque 401.
	t.Run("a key with no secret fails the login", func(t *testing.T) {
		handler := &zcodeBizHandler{t: t, secret: ""}
		client := newZCodeTestClient(t, ZCodeVariantZai, handler)

		_, err := client.ResolveCredential(context.Background(), "account-token")
		if err == nil {
			t.Fatal("ResolveCredential succeeded without a secret")
		}
		if !strings.Contains(err.Error(), "secret") {
			t.Errorf("error %q should name the missing secret", err)
		}
	})

	t.Run("an empty account token never reaches the network", func(t *testing.T) {
		client := newZCodeTestClient(t, ZCodeVariantZai, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request to %s", r.URL.Path)
		}))
		if _, err := client.ResolveCredential(context.Background(), "  "); err == nil {
			t.Fatal("ResolveCredential accepted an empty account token")
		}
	})
}

// The whole login, end to end through the Manager: the token it produces is
// what createProviderFromToken persists, so its shape is the contract.
func TestManagerZCodeFlowProducesStaticToken(t *testing.T) {
	handler := &zcodeBizHandler{t: t, secret: "s3cret"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/oauth/cli/init", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, map[string]any{
			"flow_id":           "flow-1",
			"authorize_url":     "https://chat.z.ai/authorize",
			"poll_interval_sec": 1,
		})
	})
	mux.HandleFunc("/api/v1/oauth/cli/poll/flow-1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, w, map[string]any{
			"status": "ready",
			"token":  "plan-jwt",
			"user":   map[string]string{"user_id": "u-1"},
			"zai":    map[string]string{"access_token": "account-token"},
		})
	})
	mux.Handle("/", handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	manager := NewManager()
	opts := []Option{
		WithHTTPClient(server.Client()),
		WithZCodeEndpoints(server.URL+"/api/v1", server.URL),
	}

	flow, err := manager.InitiateZCodeFlow(context.Background(), "user-1", ai.IssuerZCode, "/done", "My Plan", opts...)
	if err != nil {
		t.Fatalf("InitiateZCodeFlow: %v", err)
	}
	if flow.Issuer != ai.IssuerZCode || flow.Variant != ZCodeVariantZai {
		t.Errorf("flow issuer/variant = %s/%s", flow.Issuer, flow.Variant)
	}

	token, err := manager.CompleteZCodeFlow(context.Background(), flow, opts...)
	if err != nil {
		t.Fatalf("CompleteZCodeFlow: %v", err)
	}

	// What we persist as the access token is the static plan key, never the
	// account token the authorization handed back.
	if token.AccessToken != "created-key.s3cret" {
		t.Errorf("AccessToken = %q, want created-key.s3cret", token.AccessToken)
	}
	if token.RefreshToken != "account-token" {
		t.Errorf("RefreshToken = %q, want the account token kept for re-resolve", token.RefreshToken)
	}
	// No expiry: the credential is static, so the background refresher must have
	// nothing to act on.
	if !token.Expiry.IsZero() {
		t.Errorf("Expiry = %v, want zero for a static credential", token.Expiry)
	}
	if token.Name != "My Plan" || token.RedirectTo != "/done" {
		t.Errorf("flow metadata lost: name=%q redirect=%q", token.Name, token.RedirectTo)
	}
	if token.Metadata[ZCodeMetaPlatform] != ZCodeVariantZai {
		t.Errorf("metadata platform = %v", token.Metadata[ZCodeMetaPlatform])
	}
	if token.Metadata[ZCodeMetaAPIKey] != "created-key" {
		t.Errorf("metadata api key = %v, want the id without its secret", token.Metadata[ZCodeMetaAPIKey])
	}
	if token.Metadata[ZCodeMetaPlanJWT] != "plan-jwt" || token.Metadata[ZCodeMetaUserID] != "u-1" {
		t.Errorf("metadata = %v", token.Metadata)
	}

	// A refresh re-resolves from the stored account token, without a browser.
	refreshed, err := manager.ReResolveZCodeCredential(context.Background(), ai.IssuerZCode, token.RefreshToken, opts...)
	if err != nil {
		t.Fatalf("ReResolveZCodeCredential: %v", err)
	}
	if refreshed.AccessToken != token.AccessToken {
		t.Errorf("re-resolved credential = %q, want %q", refreshed.AccessToken, token.AccessToken)
	}

	if _, err := manager.InitiateZCodeFlow(context.Background(), "user-1", ai.IssuerClaudeCode, "", "", opts...); err == nil {
		t.Error("InitiateZCodeFlow accepted a non-ZCode issuer")
	}
	if _, err := manager.ReResolveZCodeCredential(context.Background(), ai.IssuerZCode, "", opts...); err == nil {
		t.Error("ReResolveZCodeCredential accepted an empty account token")
	}
}

func TestZCodeEndpointsPerIssuer(t *testing.T) {
	anthropicBase, openaiBase := ai.ZCodeEndpoints(ai.IssuerZCode)
	if anthropicBase != ai.ZCodeZaiAnthropicBase || openaiBase != ai.ZCodeZaiOpenAIBase {
		t.Errorf("zcode endpoints = (%q, %q)", anthropicBase, openaiBase)
	}
	anthropicBase, openaiBase = ai.ZCodeEndpoints(ai.IssuerZCodeCN)
	if anthropicBase != ai.ZCodeBigModelAnthropicBase || openaiBase != ai.ZCodeBigModelOpenAIBase {
		t.Errorf("zcode_cn endpoints = (%q, %q)", anthropicBase, openaiBase)
	}
	if a, o := ai.ZCodeEndpoints(ai.IssuerClaudeCode); a != "" || o != "" {
		t.Errorf("non-ZCode issuer returned endpoints (%q, %q)", a, o)
	}
}

func TestZCodeIssuersAreRegistered(t *testing.T) {
	registry := DefaultRegistry()
	for _, issuer := range []ai.Issuer{ai.IssuerZCode, ai.IssuerZCodeCN} {
		config, ok := registry.Get(issuer)
		if !ok {
			t.Fatalf("issuer %s is not registered", issuer)
		}
		if config.OAuthMethod != OAuthMethodServerPoll {
			t.Errorf("%s OAuthMethod = %v, want OAuthMethodServerPoll", issuer, config.OAuthMethod)
		}
		if _, err := ParseIssuer(issuer); err != nil {
			t.Errorf("ParseIssuer(%s): %v", issuer, err)
		}
	}
}
