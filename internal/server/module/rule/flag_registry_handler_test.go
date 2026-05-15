package rule

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/server/config"
)

func TestGetFlagRegistry_ReturnsCatalog(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	router := setupTestRouter(cfg)
	handler := NewHandler(cfg)
	router.GET("/rule/flags/registry", handler.GetFlagRegistry)

	req, _ := http.NewRequest("GET", "/rule/flags/registry", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}

	var resp FlagRegistryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, w.Body.String())
	}
	if !resp.Success {
		t.Errorf("Success = false, want true")
	}
	if len(resp.Data) == 0 {
		t.Fatalf("expected non-empty flag registry, got 0 entries")
	}

	// At least the historical cursor flags must be exposed.
	want := map[string]bool{
		"cursor_compat":      false,
		"cursor_compat_auto": false,
	}
	for _, spec := range resp.Data {
		if _, ok := want[spec.Key]; ok {
			want[spec.Key] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("registry missing required key %q", k)
		}
	}
}

func TestGetFlagRegistry_NilConfigStillServes(t *testing.T) {
	// The registry is static metadata that doesn't depend on persisted rule
	// state, so it must succeed even when the handler was constructed
	// without a config (e.g. early boot path).
	router := setupTestRouter(nil)
	handler := NewHandler(nil)
	router.GET("/rule/flags/registry", handler.GetFlagRegistry)

	req, _ := http.NewRequest("GET", "/rule/flags/registry", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}
