package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// captureTransport records every request's User-Agent header (and the
// forwarded request, so tests can inspect header presence vs. absence).
type captureTransport struct {
	lastUA  string
	lastReq *http.Request
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.lastUA = req.Header.Get("User-Agent")
	c.lastReq = req
	return &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

// pinTransport simulates a vendor round-tripper: it force-sets a header on
// its way to the wire (inner chains run after the rule-flag layer).
type pinTransport struct {
	inner http.RoundTripper
	name  string
	value string
}

func (p *pinTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(p.name, p.value)
	return p.inner.RoundTrip(req)
}

func apiKeyProvider() *typ.Provider {
	return &typ.Provider{UUID: "p1", AuthType: ai.AuthTypeAPIKey, Token: "sk-x"}
}

func newReq(t *testing.T, ctx context.Context, ua string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://example.test/v1/foo", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return req
}

// ── User-Agent resolution ───────────────────────────────────────────────────
//
// ruleFlagTransport resolves a fixed precedence in one place:
//
//	rule/scenario custom_user_agent  >  inbound client UA  >  SDK default
//
// The tests below exercise every branch of that precedence plus the `none`
// strip sentinel, clone-before-mutate, and the nil-inner fallback.

func TestRuleFlagTransport_NoContextValue_PassesThrough(t *testing.T) {
	cap := &captureTransport{}
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", true)
	// Simulate the SDK having already stamped its default UA on the request.
	req := newReq(t, context.Background(), "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "sdk-default/1.0" {
		t.Errorf("UA = %q, want SDK default passthrough %q", cap.lastUA, "sdk-default/1.0")
	}
}

func TestRuleFlagTransport_RuleOverride(t *testing.T) {
	cap := &captureTransport{}
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", true)
	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{CustomUserAgent: "RuleUA/1.0"})
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "RuleUA/1.0" {
		t.Errorf("UA = %q, want rule override %q", cap.lastUA, "RuleUA/1.0")
	}
}

func TestRuleFlagTransport_ForwardsInboundClientUA(t *testing.T) {
	cap := &captureTransport{}
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", true)
	// Client sent "cherry-studio/1.2"; SDK stamped its own default. With no rule
	// override, the inbound client UA must be forwarded so upstream sees the real
	// caller instead of the generic SDK UA.
	ctx := typ.WithClientUserAgent(context.Background(), "cherry-studio/1.2")
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "cherry-studio/1.2" {
		t.Errorf("UA = %q, want inbound client UA %q", cap.lastUA, "cherry-studio/1.2")
	}
}

func TestRuleFlagTransport_RuleWinsOverClient(t *testing.T) {
	cap := &captureTransport{}
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", true)
	// Both present: the rule/scenario override wins over the inbound client UA.
	ctx := typ.WithClientUserAgent(context.Background(), "cherry-studio/1.2")
	ctx = typ.WithRuleFlags(ctx, typ.RuleFlags{CustomUserAgent: "RuleUA/1.0"})
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "RuleUA/1.0" {
		t.Errorf("UA = %q, want rule wins over client %q", cap.lastUA, "RuleUA/1.0")
	}
}

func TestRuleFlagTransport_NoneSentinelStripsEvenWithClientUA(t *testing.T) {
	cap := &captureTransport{}
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", true)
	// The `none` sentinel is a rule/scenario value; it must strip the UA entirely
	// and still win over a present inbound client UA.
	ctx := typ.WithClientUserAgent(context.Background(), "cherry-studio/1.2")
	ctx = typ.WithRuleFlags(ctx, typ.RuleFlags{CustomUserAgent: typ.UserAgentNone})
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "" {
		t.Errorf("UA = %q, want stripped (empty)", cap.lastUA)
	}
	// The header must be present-but-empty (not absent), otherwise net/http would
	// re-inject its default Go-http-client UA on the wire.
	if _, ok := cap.lastReq.Header["User-Agent"]; !ok {
		t.Error("expected User-Agent header present-but-empty, got absent")
	}
	// Caller's request must be untouched.
	if req.Header.Get("User-Agent") != "sdk-default/1.0" {
		t.Errorf("original request mutated: UA = %q", req.Header.Get("User-Agent"))
	}
}

func TestRuleFlagTransport_DoesNotMutateOriginalRequest(t *testing.T) {
	cap := &captureTransport{}
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", true)
	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{CustomUserAgent: "RuleUA/1.0"})
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	// Clone-before-mutate: the caller's request must keep its original header so
	// concurrent retries don't race on shared headers.
	if got := req.Header.Get("User-Agent"); got != "sdk-default/1.0" {
		t.Errorf("caller's req mutated: UA = %q, want %q", got, "sdk-default/1.0")
	}
}

func TestRuleFlagTransport_EmptyFlagsAreNoOp(t *testing.T) {
	cap := &captureTransport{}
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", true)
	// Zero-value flags attached (the merge point attaches unconditionally), so
	// an existing SDK UA header is left untouched.
	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{})
	ctx = typ.WithClientUserAgent(ctx, "")
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "sdk-default/1.0" {
		t.Errorf("UA = %q, want passthrough %q", cap.lastUA, "sdk-default/1.0")
	}
}

func TestRuleFlagTransport_NoResolveUA_IgnoresUAFlags(t *testing.T) {
	cap := &captureTransport{}
	// resolveUA=false (e.g. the generic Google chain): UA flags must not apply
	// even when present in ctx.
	wrapped := wrapWithRuleFlags(cap, apiKeyProvider(), "", false)
	ctx := typ.WithClientUserAgent(context.Background(), "cherry-studio/1.2")
	ctx = typ.WithRuleFlags(ctx, typ.RuleFlags{CustomUserAgent: "RuleUA/1.0"})
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "sdk-default/1.0" {
		t.Errorf("UA = %q, want SDK default (UA resolution disabled)", cap.lastUA)
	}
}

func TestRuleFlagTransport_NilInnerFallsBackToDefault(t *testing.T) {
	// When inner is nil the transport must not panic; it falls back to
	// http.DefaultTransport. Exercise it against an httptest server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	wrapped := wrapWithRuleFlags(nil, apiKeyProvider(), "", true)
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := wrapped.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

// ── extra_headers ───────────────────────────────────────────────────────────

func newHeadersReq(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()
	return newReq(t, typ.WithRuleFlags(context.Background(), typ.RuleFlags{ExtraHeaders: headers}), "")
}

func TestRuleFlagTransport_AppliesExtraHeaders(t *testing.T) {
	capture := &captureTransport{}
	rt := wrapWithRuleFlags(capture, apiKeyProvider(), "", true)

	if _, err := rt.RoundTrip(newHeadersReq(t, map[string]string{"X-Title": "tingly"})); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("X-Title"); got != "tingly" {
		t.Errorf("X-Title = %q, want tingly", got)
	}
}

func TestRuleFlagTransport_ExtraHeadersNonAPIKeyIsNoOp(t *testing.T) {
	capture := &captureTransport{}
	oauth := &typ.Provider{UUID: "p2", AuthType: ai.AuthTypeOAuth}
	rt := wrapWithRuleFlags(capture, oauth, "", false)
	if rt != http.RoundTripper(capture) {
		t.Fatal("a chain where no flag can apply must get the inner transport unchanged")
	}

	// Even ctx-carried rule headers cannot reach a non-api_key chain.
	if _, err := rt.RoundTrip(newHeadersReq(t, map[string]string{"X-Rule": "r"})); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("X-Rule"); got != "" {
		t.Errorf("X-Rule = %q, want unset on non-api_key provider", got)
	}
}

// TestRuleFlagTransport_VendorPinWins is the ordering invariant of
// .design/provider-flags.md §5.2: the rule-flag layer sits outside vendor
// round-trippers, which write later (closer to the wire) and therefore win on
// a name conflict. Moot for the api_key-only release, but it must hold from
// day one for a future OAuth rollout.
func TestRuleFlagTransport_VendorPinWins(t *testing.T) {
	capture := &captureTransport{}
	vendor := &pinTransport{inner: capture, name: "X-Vendor-Pin", value: "pinned"}
	rt := wrapWithRuleFlags(vendor, apiKeyProvider(), "", true)

	if _, err := rt.RoundTrip(newHeadersReq(t, map[string]string{
		"X-Vendor-Pin": "user-tries-to-override",
		"X-Title":      "tingly",
	})); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("X-Vendor-Pin"); got != "pinned" {
		t.Errorf("X-Vendor-Pin = %q, want pinned (vendor writes last)", got)
	}
	if got := capture.lastReq.Header.Get("X-Title"); got != "tingly" {
		t.Errorf("X-Title = %q, want tingly (non-conflicting header still applied)", got)
	}
}

// TestRuleFlagTransport_AppliesExtraHeadersVerbatim: user-driven config — the
// transport does not filter, even for gateway-adjacent names like
// Authorization. The user asked for it; the user owns it.
func TestRuleFlagTransport_AppliesExtraHeadersVerbatim(t *testing.T) {
	capture := &captureTransport{}
	rt := wrapWithRuleFlags(capture, apiKeyProvider(), "", true)

	req := newHeadersReq(t, map[string]string{
		"Authorization": "Bearer custom",
		"X-Title":       "fine",
	})
	req.Header.Set("Authorization", "Bearer original")
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("Authorization"); got != "Bearer custom" {
		t.Errorf("Authorization = %q, want the user-configured value", got)
	}
	if got := capture.lastReq.Header.Get("X-Title"); got != "fine" {
		t.Errorf("X-Title = %q, want fine", got)
	}
}

func TestRuleFlagTransport_ExtraHeadersDoNotMutateCallerRequest(t *testing.T) {
	capture := &captureTransport{}
	rt := wrapWithRuleFlags(capture, apiKeyProvider(), "", true)

	orig := newHeadersReq(t, map[string]string{"X-Title": "tingly"})
	if _, err := rt.RoundTrip(orig); err != nil {
		t.Fatal(err)
	}
	if got := orig.Header.Get("X-Title"); got != "" {
		t.Errorf("caller's request mutated: X-Title = %q", got)
	}
}

// ── combined precedence ─────────────────────────────────────────────────────

// TestRuleFlagTransport_UAWinsOverExtraHeaderUA pins the merged ordering: an
// extra_headers entry naming User-Agent loses to the UA resolution, matching
// the old two-transport stack (extra headers outermost, UA innermost).
func TestRuleFlagTransport_UAWinsOverExtraHeaderUA(t *testing.T) {
	capture := &captureTransport{}
	rt := wrapWithRuleFlags(capture, apiKeyProvider(), "", true)

	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{
		CustomUserAgent: "RuleUA/1.0",
		ExtraHeaders:    map[string]string{"User-Agent": "extra-header-ua"},
	})
	if _, err := rt.RoundTrip(newReq(t, ctx, "sdk-default/1.0")); err != nil {
		t.Fatal(err)
	}
	if capture.lastUA != "RuleUA/1.0" {
		t.Errorf("UA = %q, want the UA resolution to win over extra_headers", capture.lastUA)
	}
}

// TestWrapWithLogging_IsLoggingOnly pins the un-bundling: wrapWithLogging no
// longer smuggles a flag layer, so ctx-carried extra headers do NOT apply
// through it — mounting wrapWithRuleFlags is the constructors' explicit job.
func TestWrapWithLogging_IsLoggingOnly(t *testing.T) {
	capture := &captureTransport{}
	rt := wrapWithLogging(capture, apiKeyProvider())

	if _, err := rt.RoundTrip(newHeadersReq(t, map[string]string{"X-Title": "tingly"})); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("X-Title"); got != "" {
		t.Errorf("X-Title = %q, want unset (logging layer must not apply flags)", got)
	}
}

// ── Supply-side headers (provider ∪ model) ──────────────────────────────────

func providerWithHeaders(t *testing.T, provider, model map[string]string) *typ.Provider {
	t.Helper()
	p := apiKeyProvider()
	if provider != nil {
		p.Flags = typ.ProviderFlags{ExtraHeaders: provider}
	}
	if model != nil {
		p.ModelFlags = map[string]typ.ProviderFlags{"m1": {ExtraHeaders: model}}
	}
	return p
}

// Supply-side headers ride the client, not the request context, so paths that
// never pass through protocol dispatch (probes, model-list fetch) send them.
func TestRuleFlagTransport_SupplyHeadersWithoutCtx(t *testing.T) {
	capture := &captureTransport{}
	rt := wrapWithRuleFlags(capture, providerWithHeaders(t, map[string]string{"X-Title": "tingly"}, nil), "", true)

	if _, err := rt.RoundTrip(newReq(t, context.Background(), "")); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("X-Title"); got != "tingly" {
		t.Errorf("X-Title = %q, want tingly", got)
	}
}

// The full precedence provider < model < rule, produced by write order alone.
func TestRuleFlagTransport_ThreeLevelPrecedence(t *testing.T) {
	capture := &captureTransport{}
	provider := providerWithHeaders(t,
		map[string]string{"X-Shared": "provider", "X-Provider": "p"},
		map[string]string{"X-Shared": "model", "X-Model": "m"},
	)
	rt := wrapWithRuleFlags(capture, provider, "m1", true)

	ctx := typ.WithRuleFlags(context.Background(), typ.RuleFlags{ExtraHeaders: map[string]string{
		"X-Shared": "rule",
		"X-Rule":   "r",
	}})
	if _, err := rt.RoundTrip(newReq(t, ctx, "")); err != nil {
		t.Fatal(err)
	}
	for header, want := range map[string]string{
		"X-Shared":   "rule", // provider < model < rule
		"X-Provider": "p",
		"X-Model":    "m",
		"X-Rule":     "r",
	} {
		if got := capture.lastReq.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

// A client built for another model gets only the provider level.
func TestRuleFlagTransport_ModelHeadersAreModelScoped(t *testing.T) {
	capture := &captureTransport{}
	provider := providerWithHeaders(t,
		map[string]string{"X-Provider": "p"},
		map[string]string{"X-Model": "m"},
	)
	rt := wrapWithRuleFlags(capture, provider, "other-model", true)

	if _, err := rt.RoundTrip(newReq(t, context.Background(), "")); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("X-Provider"); got != "p" {
		t.Errorf("X-Provider = %q, want p", got)
	}
	if got := capture.lastReq.Header.Get("X-Model"); got != "" {
		t.Errorf("X-Model = %q, want unset for a different model", got)
	}
}

// The api_key gate covers supply-side headers too: a non-api_key provider
// gets no transport at all, so nothing configured on it can reach the wire.
func TestRuleFlagTransport_SupplyHeadersRespectAPIKeyGate(t *testing.T) {
	capture := &captureTransport{}
	oauth := &typ.Provider{
		UUID:     "p2",
		AuthType: ai.AuthTypeOAuth,
		Flags:    typ.ProviderFlags{ExtraHeaders: map[string]string{"X-Title": "cfg"}},
	}

	rt := wrapWithRuleFlags(capture, oauth, "m1", false)
	if rt != http.RoundTripper(capture) {
		t.Fatal("non-api_key provider with no UA resolution must get the inner transport unchanged")
	}
	if _, err := rt.RoundTrip(newReq(t, context.Background(), "")); err != nil {
		t.Fatal(err)
	}
	if got := capture.lastReq.Header.Get("X-Title"); got != "" {
		t.Errorf("X-Title = %q, want unset on a non-api_key provider", got)
	}
}
