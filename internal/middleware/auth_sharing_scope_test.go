package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

const sharingScopeTestToken = "tb-share-valid"

type sharingScopeTokenStore struct{}

func (sharingScopeTokenStore) ValidateToken(tokenID string) (*db.APITokenRecord, error) {
	if tokenID != sharingScopeTestToken {
		return nil, errors.New("token not found or disabled")
	}
	return &db.APITokenRecord{TokenID: tokenID, UserID: "sharing-user", Enabled: true}, nil
}

func (sharingScopeTokenStore) UpdateLastUsed(string) error { return nil }

func newSharingScopeTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{ModelToken: "global-model-token"}
	cfg.MultiTenantConfig.Enabled = true
	am := NewAuthMiddleware(cfg, nil, nil, sharingScopeTokenStore{})

	router := gin.New()
	handler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"auth_kind": c.GetString(constant.CtxKeyAuthKind),
			"user_id":   c.GetString(constant.CtxKeyUserID),
		})
	}
	router.POST("/tingly/:scenario/messages", am.ModelAuthMiddleware(), handler)
	router.POST("/tingly/:scenario/v1/messages", am.ModelAuthMiddleware(), handler)
	router.POST("/virtual/openai/v1/messages", am.ModelAuthMiddleware(), handler)
	return router
}

func performModelAuthRequest(router http.Handler, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestModelAuthMiddleware_SharingKeySurfaceAuthorization(t *testing.T) {
	router := newSharingScopeTestRouter()
	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "bare team endpoint", path: "/tingly/team/messages", wantStatus: http.StatusOK},
		{name: "team v1 endpoint", path: "/tingly/team/v1/messages", wantStatus: http.StatusOK},
		{name: "other scenario", path: "/tingly/openai/messages", wantStatus: http.StatusForbidden},
		{name: "client selected team suffix", path: "/tingly/team:p1/messages", wantStatus: http.StatusForbidden},
		{name: "virtual model surface", path: "/virtual/openai/v1/messages", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := performModelAuthRequest(router, tt.path, sharingScopeTestToken)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				if body := w.Body.String(); !strings.Contains(body, `"auth_kind":"sharing_key"`) || !strings.Contains(body, `"user_id":"sharing-user"`) {
					t.Fatalf("unexpected sharing principal context: %s", body)
				}
			} else if !strings.Contains(w.Body.String(), `"type":"forbidden_error"`) {
				t.Fatalf("expected forbidden_error: %s", w.Body.String())
			}
		})
	}
}

func TestModelAuthMiddleware_GlobalTokenKeepsAllModelSurfaces(t *testing.T) {
	router := newSharingScopeTestRouter()
	for _, path := range []string{"/tingly/team/messages", "/tingly/openai/messages", "/virtual/openai/v1/messages"} {
		w := performModelAuthRequest(router, path, "global-model-token")
		if w.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200: %s", path, w.Code, w.Body.String())
			continue
		}
		if body := w.Body.String(); !strings.Contains(body, `"auth_kind":"global_model_token"`) || !strings.Contains(body, `"user_id":"admin"`) {
			t.Errorf("%s unexpected global principal context: %s", path, body)
		}
	}
}

func TestModelAuthMiddleware_InvalidSharingKeyRemainsUnauthorized(t *testing.T) {
	router := newSharingScopeTestRouter()
	w := performModelAuthRequest(router, "/tingly/team/messages", "tb-share-invalid")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
	}
}
