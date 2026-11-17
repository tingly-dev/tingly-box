package command

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

func TestVModelGatewayIntegration(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.CloseStores()
	gatewayServer := server.NewServer(cfg, server.WithUI(true))
	defer gatewayServer.CloseMCPRuntime()
	defer gatewayServer.CloseVirtualModelServer()
	gateway := httptest.NewServer(gatewayServer.GetRouter())
	defer gateway.Close()
	beforeRules, beforeProviders := len(cfg.GetRequestConfigs()), len(cfg.ListProviders())
	files := []string{vmodelConfigFile(t, "id: injected-one\ncontent: gateway response\n"), vmodelConfigFile(t, "id: injected-two\ncontent: second response\n")}
	handler, ids, err := (&VModelCmdKong{Config: files}).handler()
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(handler)
	defer upstream.Close()
	ready := vmodelReady{URL: upstream.URL, OpenAIBaseURL: upstream.URL + "/openai/v1", AnthropicBaseURL: upstream.URL + "/anthropic", Models: ids}
	g := &vmodelGateway{baseURL: gateway.URL, token: cfg.GetUserToken(), client: gateway.Client()}
	cleanup, err := g.attach(context.Background(), &ready)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if ready.GatewayBaseURL != gateway.URL+"/tingly/vmodel" {
		t.Fatalf("readiness: %+v", ready)
	}
	if rule := cfg.GetRuleByRequestModelAndScenario("injected-one", typ.ScenarioVModel); rule == nil || !rule.Active {
		t.Fatal("missing active scenario rule")
	}
	if _, err := g.attach(context.Background(), &ready); err == nil {
		t.Fatal("existing rules were overwritten")
	}
	for _, tc := range []struct{ path, body string }{
		{"/tingly/vmodel/v1/chat/completions", `{"model":"injected-one","messages":[{"role":"user","content":"hi"}]}`},
		{"/tingly/vmodel/v1/responses", `{"model":"injected-one","input":"hi"}`},
		{"/tingly/vmodel/v1/messages", `{"model":"injected-one","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req, err := http.NewRequest("POST", gateway.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+cfg.GetModelToken())
			req.Header.Set("x-api-key", cfg.GetModelToken())
			req.Header.Set("Content-Type", "application/json")
			resp, err := gateway.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "gateway response") {
				t.Fatalf("gateway response: %d %s", resp.StatusCode, body)
			}
		})
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.GetRequestConfigs()) != beforeRules || len(cfg.ListProviders()) != beforeProviders {
		t.Fatal("cleanup changed existing resources or left injected resources")
	}

	// Fail after one rule was persisted, proving startup rolls back both the
	// completed rule and its provider without changing unrelated configuration.
	rulePosts := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/rule" {
			rulePosts++
			if rulePosts == 2 {
				http.Error(w, "injected failure", 500)
				return
			}
		}
		gatewayServer.GetRouter().ServeHTTP(w, r)
	}))
	defer proxy.Close()
	g.baseURL = proxy.URL
	if _, err := g.attach(context.Background(), &ready); err == nil {
		t.Fatal("partial registration succeeded")
	}
	if len(cfg.GetRequestConfigs()) != beforeRules || len(cfg.ListProviders()) != beforeProviders {
		t.Fatal("partial registration was not rolled back")
	}
}

func TestVModelNativeRecordingGateway(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.CloseStores()
	gatewayServer := server.NewServer(cfg, server.WithUI(true))
	defer gatewayServer.CloseMCPRuntime()
	defer gatewayServer.CloseVirtualModelServer()
	gateway := httptest.NewServer(gatewayServer.GetRouter())
	defer gateway.Close()
	text := scenario.TextScenario()
	var defs []spec.Definition
	for _, p := range []struct {
		id     string
		format scenario.ResponseFormat
	}{{"responses-only", scenario.FormatOpenAIResponses}, {"google-only", scenario.FormatGoogle}} {
		status, body := text.MockResponses[p.format].NonStream()
		defs = append(defs, spec.Definition{ID: p.id, Kind: spec.Recording, Responses: map[scenario.ResponseFormat]scenario.Response{p.format: {Status: status, Body: body}}})
	}
	r, err := spec.New(defs)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(r)
	defer upstream.Close()
	ready := vmodelReady{URL: upstream.URL, OpenAIBaseURL: upstream.URL + "/openai/v1", AnthropicBaseURL: upstream.URL + "/anthropic", GoogleBaseURL: upstream.URL, Models: r.IDs()}
	g := &vmodelGateway{baseURL: gateway.URL, token: cfg.GetUserToken(), client: gateway.Client()}
	cleanup, err := g.attach(context.Background(), &ready)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, id := range r.IDs() {
		rule := cfg.GetRuleByRequestModelAndScenario(id, typ.ScenarioVModel)
		if rule == nil {
			t.Fatalf("missing route for %s", id)
		}
		provider, err := cfg.GetProviderByUUID(rule.Services[0].Provider)
		if err != nil {
			t.Fatal(err)
		}
		if id == "responses-only" && provider.OpenAIEndpointMode != "responses" {
			t.Fatalf("wrong endpoint mode: %s", provider.OpenAIEndpointMode)
		}
		req, err := http.NewRequest("POST", gateway.URL+"/tingly/vmodel/v1/chat/completions", strings.NewReader(`{"model":"`+id+`","messages":[{"role":"user","content":"hi"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+cfg.GetModelToken())
		req.Header.Set("Content-Type", "application/json")
		response, err := gateway.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || !strings.Contains(string(body), "Paris") {
			t.Fatalf("%s: %d %s", id, response.StatusCode, body)
		}
	}
}
