package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/config"
)

func TestVModelSceneDirectModelsWithoutRules(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.CloseStores()
	gateway := NewServer(cfg)
	defer gateway.CloseVirtualModelServer()
	defer gateway.CloseMCPRuntime()
	beforeRules, beforeProviders := len(cfg.GetRequestConfigs()), len(cfg.ListProviders())
	host := httptest.NewServer(gateway.GetRouter())
	defer host.Close()
	request := func(method, path, body, token string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(method, host.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("x-api-key", token)
		}
		res, err := host.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, string(raw)
	}
	if status, _ := request("GET", "/tingly/vmodel/v1/models", "", ""); status == 200 {
		t.Fatal("model auth bypassed")
	}
	token := cfg.GetModelToken()
	status, body := request("GET", "/tingly/vmodel/v1/models", "", token)
	if status != 200 || !strings.Contains(body, "echo-model") || !strings.Contains(body, "ask-confirmation") {
		t.Fatalf("catalog: %d %s", status, body)
	}
	for _, tc := range []struct{ path, body, want string }{
		{"/v1/chat/completions", `{"model":"echo-model","messages":[]}`, "Echo:"},
		{"/v1/responses", `{"model":"echo-model","input":"hi"}`, "Echo:"},
		{"/v1/messages", `{"model":"echo-model","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`, "Echo:"},
		{"/v1/chat/completions", `{"model":"ask-confirmation","messages":[]}`, "ask_user_question"},
		{"/v1/chat/completions", `{"model":"echo-model","messages":[],"stream":true}`, "[DONE]"},
	} {
		status, body := request("POST", "/tingly/vmodel"+tc.path, tc.body, token)
		if status != 200 || !strings.Contains(body, tc.want) {
			t.Fatalf("%s: %d %s", tc.path, status, body)
		}
	}
	if len(cfg.GetRequestConfigs()) != beforeRules || len(cfg.ListProviders()) != beforeProviders {
		t.Fatal("direct model calls changed rules/providers")
	}
}
