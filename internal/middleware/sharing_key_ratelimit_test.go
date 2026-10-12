package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/ratelimit"
)

// rlFixedLimiter answers every call with a fixed decision (allow, or deny
// with retryAfter) and records what it was asked, so the middleware's
// routing/scoping logic is tested without depending on bucket math (covered
// in package ratelimit).
type rlFixedLimiter struct {
	allow      bool
	retryAfter time.Duration
	calls      []struct {
		key   string
		limit int
	}
}

func (f *rlFixedLimiter) Allow(key string, limit int) ratelimit.Decision {
	f.calls = append(f.calls, struct {
		key   string
		limit int
	}{key, limit})
	if f.allow {
		return ratelimit.Decision{Allowed: true}
	}
	return ratelimit.Decision{Allowed: false, RetryAfter: f.retryAfter}
}

// rlRouter stands in for ModelAuthMiddleware by seeding the context the way
// auth.go does for the given principal. rpm is stored as-is (any type) so a
// test can pin the type-mismatch behaviour. reached reports whether the
// terminal handler ran — the abort contract of a 429.
func rlRouter(l ratelimit.Limiter, authKind, keyID string, rpm any, reached *bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	seed := func(c *gin.Context) {
		c.Set(constant.CtxKeyAuthKind, authKind)
		if authKind == constant.AuthKindSharingKey {
			c.Set(constant.CtxKeySharingKeyID, keyID)
			c.Set(constant.CtxKeySharingKeyRPMLimit, rpm)
		}
		c.Next()
	}
	terminal := func(c *gin.Context) {
		if reached != nil {
			*reached = true
		}
		c.Status(http.StatusOK)
	}
	// Registered with the same templates as production (routes.go) so the
	// exact-match exemption is exercised, not a lookalike.
	r.POST("/tingly/:scenario/v1/messages", seed, SharingKeyRateLimitMiddleware(l), terminal)
	r.POST("/tingly/:scenario/v1/messages/count_tokens", seed, SharingKeyRateLimitMiddleware(l), terminal)
	r.POST("/tingly/:scenario/messages/count_tokens", seed, SharingKeyRateLimitMiddleware(l), terminal)
	// A lookalike route that only shares the suffix must NOT be exempt.
	r.POST("/other/messages/count_tokens", seed, SharingKeyRateLimitMiddleware(l), terminal)
	r.GET("/tingly/:scenario/v1/models", seed, SharingKeyRateLimitMiddleware(l), terminal)
	return r
}

func rlRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSharingKeyRateLimit_DeniesWith429AndRetryAfter(t *testing.T) {
	l := &rlFixedLimiter{allow: false, retryAfter: 1500 * time.Millisecond}
	reached := false
	r := rlRouter(l, constant.AuthKindSharingKey, "tb-share-abc", 60, &reached)

	w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body=%s", w.Code, w.Body.String())
	}
	if reached {
		t.Fatal("terminal handler ran after a 429: the chain was not aborted")
	}
	// 1.5s rounds up to 2 whole seconds.
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want \"2\"", got)
	}
	if len(l.calls) != 1 || l.calls[0].key != "tb-share-abc" || l.calls[0].limit != 60 {
		t.Fatalf("limiter called with %+v, want key tb-share-abc limit 60", l.calls)
	}
	body := w.Body.String()
	for _, want := range []string{`"type":"rate_limit_error"`, "60 requests/minute"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %s missing %s", body, want)
		}
	}
}

func TestSharingKeyRateLimit_RetryAfterRounding(t *testing.T) {
	for _, tc := range []struct {
		wait time.Duration
		want string
	}{
		{time.Millisecond, "1"}, // never below one second
		{12 * time.Second, "12"},
		{12*time.Second + time.Millisecond, "13"},
	} {
		l := &rlFixedLimiter{allow: false, retryAfter: tc.wait}
		r := rlRouter(l, constant.AuthKindSharingKey, "tb-share-abc", 60, nil)
		w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages")
		if got := w.Header().Get("Retry-After"); got != tc.want {
			t.Fatalf("wait %v: Retry-After = %q, want %q", tc.wait, got, tc.want)
		}
	}
}

func TestSharingKeyRateLimit_AllowsWhenLimiterAllows(t *testing.T) {
	l := &rlFixedLimiter{allow: true}
	reached := false
	r := rlRouter(l, constant.AuthKindSharingKey, "tb-share-abc", 60, &reached)
	if w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages"); w.Code != http.StatusOK || !reached {
		t.Fatalf("status = %d reached=%v, want 200 and the terminal handler", w.Code, reached)
	}
	if len(l.calls) != 1 {
		t.Fatalf("limiter should be consulted once, got %d", len(l.calls))
	}
}

func TestSharingKeyRateLimit_SkipsGlobalModelToken(t *testing.T) {
	l := &rlFixedLimiter{allow: false, retryAfter: time.Second}
	r := rlRouter(l, constant.AuthKindGlobalModelToken, "", 0, nil)
	if w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages"); w.Code != http.StatusOK {
		t.Fatalf("global model token must bypass the limiter; status = %d", w.Code)
	}
	if len(l.calls) != 0 {
		t.Fatalf("limiter must not be consulted for the global token, got %d calls", len(l.calls))
	}
}

func TestSharingKeyRateLimit_SkipsUnlimitedKeyAndGET(t *testing.T) {
	l := &rlFixedLimiter{allow: false, retryAfter: time.Second}

	// rpm_limit 0 = unlimited.
	r := rlRouter(l, constant.AuthKindSharingKey, "tb-share-abc", 0, nil)
	if w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages"); w.Code != http.StatusOK {
		t.Fatalf("unlimited key denied: status = %d", w.Code)
	}

	// GET discovery endpoints are not counted even for a limited key.
	r = rlRouter(l, constant.AuthKindSharingKey, "tb-share-abc", 1, nil)
	if w := rlRequest(r, http.MethodGet, "/tingly/team/v1/models"); w.Code != http.StatusOK {
		t.Fatalf("GET must bypass the limiter; status = %d", w.Code)
	}
	if len(l.calls) != 0 {
		t.Fatalf("limiter must not be consulted, got %d calls", len(l.calls))
	}
}

func TestSharingKeyRateLimit_SkipsCountTokens(t *testing.T) {
	// count_tokens tokenizes without invoking the model; it must not spend
	// the budget even for an exhausted key — on both route shapes.
	l := &rlFixedLimiter{allow: false, retryAfter: time.Second}
	r := rlRouter(l, constant.AuthKindSharingKey, "tb-share-abc", 1, nil)
	for _, path := range []string{"/tingly/team/v1/messages/count_tokens", "/tingly/team/messages/count_tokens"} {
		if w := rlRequest(r, http.MethodPost, path); w.Code != http.StatusOK {
			t.Fatalf("%s must bypass the limiter; status = %d", path, w.Code)
		}
	}
	if len(l.calls) != 0 {
		t.Fatalf("limiter must not be consulted, got %d calls", len(l.calls))
	}
	// …while the sibling /messages route on the same key is still limited,
	// and a route that merely ends in the same suffix is limited too.
	for _, path := range []string{"/tingly/team/v1/messages", "/other/messages/count_tokens"} {
		if w := rlRequest(r, http.MethodPost, path); w.Code != http.StatusTooManyRequests {
			t.Fatalf("%s must still be limited; status = %d", path, w.Code)
		}
	}
}

func TestSharingKeyRateLimit_SkipsWhenKeyIDMissing(t *testing.T) {
	// Auth kind says sharing key but no key id was stored: nothing to bucket
	// by, so pass through rather than lump all such callers together.
	l := &rlFixedLimiter{allow: false, retryAfter: time.Second}
	r := rlRouter(l, constant.AuthKindSharingKey, "", 5, nil)
	if w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages"); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(l.calls) != 0 {
		t.Fatalf("limiter must not be consulted, got %d calls", len(l.calls))
	}
}

func TestSharingKeyRateLimit_NonIntLimitFailsOpen(t *testing.T) {
	// The auth.go contract is an int. Any other dynamic type (including a
	// nil stored under the key) must not be silently read as 0 by accident;
	// it is reported exactly once per process and treated as unlimited
	// (fail-open), so the limiter is not consulted.
	rpmLimitTypeWarning = sync.Once{} // fresh for this test
	hook := logtest.NewGlobal()
	defer hook.Reset()

	for _, rpm := range []any{int64(5), float64(5), "5", nil} {
		l := &rlFixedLimiter{allow: false, retryAfter: time.Second}
		r := rlRouter(l, constant.AuthKindSharingKey, "tb-share-abc", rpm, nil)
		for i := 0; i < 3; i++ {
			if w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages"); w.Code != http.StatusOK {
				t.Fatalf("rpm %T: status = %d, want 200 (fail-open)", rpm, w.Code)
			}
		}
		if len(l.calls) != 0 {
			t.Fatalf("rpm %T: limiter must not be consulted, got %d calls", rpm, len(l.calls))
		}
	}

	warnings := 0
	for _, e := range hook.AllEntries() {
		if e.Level == logrus.WarnLevel && strings.Contains(e.Message, constant.CtxKeySharingKeyRPMLimit) {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("type-mismatch warning logged %d times across 12 requests, want exactly 1", warnings)
	}
}

func TestSharingKeyRateLimit_NilLimiterIsNoop(t *testing.T) {
	r := rlRouter(nil, constant.AuthKindSharingKey, "tb-share-abc", 1, nil)
	for i := 0; i < 5; i++ {
		if w := rlRequest(r, http.MethodPost, "/tingly/team/v1/messages"); w.Code != http.StatusOK {
			t.Fatalf("nil limiter must pass everything; status = %d", w.Code)
		}
	}
}
