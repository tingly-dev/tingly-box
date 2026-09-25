package scenario

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type fakePreview struct{ scenarios []string }

func (f *fakePreview) PreviewRoute(scenario, modelID string) *statusline.Route {
	f.scenarios = append(f.scenarios, scenario)
	return &statusline.Route{ProviderName: "Acme", Model: "acme-" + modelID}
}

func getModels(t *testing.T, h *Handler, url string) (int, ClaudeCodeModelsData) {
	t.Helper()
	router := gin.New()
	router.GET("/scenario/:scenario/models", h.GetClaudeCodeModels)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var resp ClaudeCodeModelsResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, resp.Data
}

func TestGetClaudeCodeModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	separate, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work", false)
	if err != nil {
		t.Fatal(err)
	}
	unified, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "one", true)
	if err != nil {
		t.Fatal(err)
	}
	preview := &fakePreview{}
	h := NewHandler(cfg, &mockRemoteControlController{}).WithRoutePreview(preview)

	code, main := getModels(t, h, "/scenario/claude_code/models")
	if code != http.StatusOK || !main.Unified || len(main.Tiers) != 1 || main.Tiers[0].Model == "" {
		t.Fatalf("main routing: %d %+v, want one unified tier", code, main)
	}
	if main.Tiers[0].ProviderName != "Acme" || preview.scenarios[0] != "claude_code" {
		t.Fatalf("main tier route = %+v via %v, want previewed in claude_code", main.Tiers[0], preview.scenarios)
	}

	code, sep := getModels(t, h, "/scenario/claude_code/models?profile="+separate.ID)
	if code != http.StatusOK || sep.Unified || len(sep.Tiers) != 4 {
		t.Fatalf("separate profile: %d %+v, want 4 separate tiers", code, sep)
	}
	for i, alias := range []string{"", "opus", "sonnet", "haiku"} {
		if sep.Tiers[i].Alias != alias || sep.Tiers[i].Model == "" {
			t.Fatalf("tier %d = %+v, want alias %q with a model", i, sep.Tiers[i], alias)
		}
	}
	if last := preview.scenarios[len(preview.scenarios)-1]; last != string(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, separate.ID)) {
		t.Fatalf("profile tiers previewed in %q, want the profile's scenario", last)
	}

	code, one := getModels(t, h, "/scenario/claude_code/models?profile="+unified.ID)
	if code != http.StatusOK || !one.Unified || len(one.Tiers) != 1 {
		t.Fatalf("unified profile: %d %+v, want one tier", code, one)
	}

	// Listing reads; it never materializes a profile's settings file.
	path, exists, err := agent.InspectCCProfileSettings(separate.ID, separate.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(path); exists || statErr == nil {
		t.Fatalf("settings file %s written by a GET", path)
	}

	if code, _ := getModels(t, h, "/scenario/claude_code/models?profile=nope"); code != http.StatusNotFound {
		t.Fatalf("unknown profile: %d, want 404", code)
	}
	if code, _ := getModels(t, h, "/scenario/opencode/models"); code != http.StatusBadRequest {
		t.Fatalf("other scenario: %d, want 400", code)
	}
}
