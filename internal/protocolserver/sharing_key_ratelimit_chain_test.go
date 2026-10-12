package protocolserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/ratelimit"
)

// TestModelAuthChain_SharingKeyRateLimit drives the real chain —
// ModelAuthMiddleware (key resolved from a real APITokenStore) →
// debugHeaderGuard → SharingKeyRateLimitMiddleware (real RPMLimiter) — so
// the context-key contract between auth.go and the limiter middleware is
// exercised rather than restated by hand in each package's own tests.
func TestModelAuthChain_SharingKeyRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store, err := db.NewAPITokenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const limited, unlimited = "tb-share-limited", "tb-share-unlimited"
	if _, err := store.CreateTokenForTeamWithRPMLimit("u1", limited, db.DefaultTeamID, "limited", "test", nil, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTokenForTeamWithRPMLimit("u2", unlimited, db.DefaultTeamID, "unlimited", "test", nil, 0); err != nil {
		t.Fatal(err)
	}

	const globalToken = "global-model-token"
	cfg := &config.Config{ModelToken: globalToken}
	cfg.MultiTenantConfig.Enabled = true
	am := middleware.NewAuthMiddleware(cfg, nil, nil, store)

	ph := NewHandler(ProtocolHandlerDeps{SharingKeyLimiter: ratelimit.NewRPMLimiter()})
	reached := 0
	terminal := func(c *gin.Context) { reached++; c.Status(http.StatusOK) }
	router := gin.New()
	router.POST("/tingly/:scenario/v1/messages", ph.modelAuthChain(am.ModelAuthMiddleware(), terminal)...)
	router.POST("/tingly/:scenario/v1/messages/count_tokens", ph.modelAuthChain(am.ModelAuthMiddleware(), terminal)...)
	router.GET("/tingly/:scenario/v1/models", ph.modelAuthChain(am.ModelAuthMiddleware(), terminal)...)

	callPath := func(method, path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	call := func(method, token string) *httptest.ResponseRecorder {
		path := "/tingly/team/v1/messages"
		if method == http.MethodGet {
			path = "/tingly/team/v1/models"
		}
		return callPath(method, path, token)
	}

	// Limited key: a burst of exactly rpm_limit, then 429 with Retry-After,
	// and the terminal handler is not reached on the denied request.
	for i := 1; i <= 2; i++ {
		if rec := call(http.MethodPost, limited); rec.Code != http.StatusOK {
			t.Fatalf("limited request %d: status = %d, want 200: %s", i, rec.Code, rec.Body.String())
		}
	}
	before := reached
	rec := call(http.MethodPost, limited)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("limited request 3: status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 without Retry-After header")
	}
	if reached != before {
		t.Fatal("terminal handler ran on a 429")
	}

	// GET discovery and POST count_tokens are not counted even while the key
	// is exhausted — the exemption is matched on the registered route.
	if rec := call(http.MethodGet, limited); rec.Code != http.StatusOK {
		t.Fatalf("GET /models with exhausted key: status = %d, want 200", rec.Code)
	}
	if rec := callPath(http.MethodPost, "/tingly/team/v1/messages/count_tokens", limited); rec.Code != http.StatusOK {
		t.Fatalf("POST count_tokens with exhausted key: status = %d, want 200", rec.Code)
	}

	// Another key is unaffected; an rpm_limit of 0 is unlimited.
	for i := 0; i < 10; i++ {
		if rec := call(http.MethodPost, unlimited); rec.Code != http.StatusOK {
			t.Fatalf("unlimited key request %d: status = %d, want 200", i, rec.Code)
		}
	}

	// The global model token never reaches the limiter.
	for i := 0; i < 10; i++ {
		if rec := call(http.MethodPost, globalToken); rec.Code != http.StatusOK {
			t.Fatalf("global token request %d: status = %d, want 200: %s", i, rec.Code, rec.Body.String())
		}
	}

	// Raising the limit through the store applies on the next request: the
	// exhausted key gets a fresh bucket under the new budget.
	if err := store.SetTokenRPMLimit(limited, 5); err != nil {
		t.Fatal(err)
	}
	if rec := call(http.MethodPost, limited); rec.Code != http.StatusOK {
		t.Fatalf("after raising the limit: status = %d, want 200", rec.Code)
	}
}
