package scenario

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type fakePreview struct{ scenarios []string }

func (f *fakePreview) PreviewRoute(scenario, modelID string) *statusline.Route {
	f.scenarios = append(f.scenarios, scenario)
	return &statusline.Route{ProviderName: "Acme", Model: "acme-" + modelID}
}

func (f *fakePreview) ResolveRoute(_ context.Context, scenario, modelID string) *statusline.Route {
	f.scenarios = append(f.scenarios, scenario)
	return &statusline.Route{ProviderName: "Acme", Model: "acme-" + modelID, Quota: []statusline.QuotaSegment{{Type: "session", Text: "60% left", UsedPercent: 40}}}
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
	separate, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	if err != nil {
		t.Fatal(err)
	}
	// "work" has every slot split off the default; "one" keeps them all on it.
	if err := cfg.SetAllClaudeCodeSlots(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, separate.ID), true); err != nil {
		t.Fatal(err)
	}
	unified, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "one")
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
	if code != http.StatusOK || sep.Unified || len(sep.Tiers) != 5 {
		t.Fatalf("separate profile: %d %+v, want 5 separate tiers", code, sep)
	}
	for i, alias := range []string{"", "opus", "sonnet", "haiku", "fable"} {
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

func TestGetClaudeCodeRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	separate, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAllClaudeCodeSlots(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, separate.ID), true); err != nil {
		t.Fatal(err)
	}
	preview := &fakePreview{}
	h := NewHandler(cfg, &mockRemoteControlController{}).WithRoutePreview(preview)
	get := func(url string) (int, ClaudeCodeRouteResponse) {
		router := gin.New()
		router.GET("/scenario/:scenario/route", h.GetClaudeCodeRoute)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		var out ClaudeCodeRouteResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	code, main := get("/scenario/claude_code/route")
	if code != http.StatusOK || main.Scenario != "claude_code" || main.RequestedModel == "" || main.ProviderName != "Acme" || len(main.Quota) != 1 {
		t.Fatalf("main routing: %d %+v, want the default tier routed with its quota", code, main)
	}
	code, opus := get("/scenario/claude_code/route?profile=" + separate.ID + "&tier=opus")
	if code != http.StatusOK || opus.Scenario != string(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, separate.ID)) || opus.RequestedModel == "" {
		t.Fatalf("profile tier: %d %+v", code, opus)
	}
	_, def := get("/scenario/claude_code/route?profile=" + separate.ID)
	if def.RequestedModel == opus.RequestedModel {
		t.Fatalf("tier opus requests %q, the same as the default tier", opus.RequestedModel)
	}
	if code, _ := get("/scenario/claude_code/route?profile=nope"); code != http.StatusNotFound {
		t.Fatalf("unknown profile: %d, want 404", code)
	}
	if code, _ := get("/scenario/opencode/route"); code != http.StatusBadRequest {
		t.Fatalf("other scenario: %d, want 400", code)
	}
}
