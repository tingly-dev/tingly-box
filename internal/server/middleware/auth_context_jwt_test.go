package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

func newModelAuthTestRouter(t *testing.T) (*gin.Engine, *config.Config, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.ModelToken = "tb-model-token"
	secret := "test-context-secret"
	cfg.EnterpriseContextJWT = config.EnterpriseContextJWTConfig{
		Enabled:          true,
		AllowedIssuers:   []string{"tbe"},
		AllowedAudiences: []string{"tb"},
		AlgAllowlist:     []string{"HS256"},
		HS256SecretRef:   secret,
		ClockSkewSeconds: 30,
		RequireJTI:       true,
	}

	r := gin.New()
	am := NewAuthMiddleware(cfg, nil)
	r.POST("/v1/chat/completions", am.ModelAuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"enterprise_user_id":       c.GetString("enterprise_user_id"),
			"enterprise_department_id": c.GetString("enterprise_department_id"),
			"verified":                 c.GetBool("enterprise_context_verified"),
		})
	})
	return r, cfg, secret
}

func signContextJWT(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

func TestModelAuthMiddleware_ContextJWTRequiresModelToken(t *testing.T) {
	r, _, secret := newModelAuthTestRouter(t)
	now := time.Now()
	ctxJWT := signContextJWT(t, secret, jwt.MapClaims{
		"iss": "tbe",
		"aud": "tb",
		"sub": "u-1",
		"jti": "j-1",
		"iat": now.Unix(),
		"nbf": now.Add(-1 * time.Second).Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TBE-Context-JWT", ctxJWT)
	req.Header.Set("X-TBE-Enterprise-User", "forged-user")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestModelAuthMiddleware_InvalidContextJWTRejected(t *testing.T) {
	r, _, _ := newModelAuthTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tb-model-token")
	req.Header.Set("X-TBE-Context-JWT", "invalid.jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid enterprise context jwt") {
		t.Fatalf("expected invalid context error, got %s", w.Body.String())
	}
}

func TestModelAuthMiddleware_ValidContextJWTAccepted(t *testing.T) {
	r, _, secret := newModelAuthTestRouter(t)
	now := time.Now()
	ctxJWT := signContextJWT(t, secret, jwt.MapClaims{
		"iss":        "tbe",
		"aud":        "tb",
		"sub":        "u-1",
		"dept_id":    "dep-1",
		"key_prefix": "kp-1",
		"tier":       "enterprise",
		"jti":        "j-1",
		"iat":        now.Unix(),
		"nbf":        now.Add(-1 * time.Second).Unix(),
		"exp":        now.Add(5 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tb-model-token")
	req.Header.Set("X-TBE-Context-JWT", ctxJWT)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"enterprise_user_id":"u-1"`) || !strings.Contains(body, `"enterprise_department_id":"dep-1"`) || !strings.Contains(body, `"verified":true`) {
		t.Fatalf("unexpected body: %s", body)
	}
}
