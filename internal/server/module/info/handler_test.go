package info

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

func TestHealthCarriesServiceAndInstanceHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/info/health", NewHandler("v", "/cfg", "/dir", "binary", "tok-123").GetHealthInfo)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/info/health", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get(constant.HealthServiceHeader); got != constant.HealthServiceName {
		t.Errorf("%s = %q, want %q", constant.HealthServiceHeader, got, constant.HealthServiceName)
	}
	if got := w.Header().Get(constant.HealthInstanceHeader); got != "tok-123" {
		t.Errorf("%s = %q, want tok-123", constant.HealthInstanceHeader, got)
	}
	if !strings.Contains(w.Body.String(), `"service":"tingly-box"`) {
		t.Errorf("body should name the service, got: %s", w.Body.String())
	}
}

// Without an instance token (a server started by a call site that did not ask
// for one) the header is absent rather than empty.
func TestHealthOmitsInstanceHeaderWhenUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/info/health", NewHandler("v", "/cfg", "/dir", "binary", "").GetHealthInfo)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/info/health", nil))

	if got := w.Header().Get(constant.HealthInstanceHeader); got != "" {
		t.Errorf("%s = %q, want empty", constant.HealthInstanceHeader, got)
	}
	if got := w.Header().Get(constant.HealthServiceHeader); got != constant.HealthServiceName {
		t.Errorf("%s = %q, want %q", constant.HealthServiceHeader, got, constant.HealthServiceName)
	}
}
