package protocolserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/translation"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type translationTokenStore struct{}

func (translationTokenStore) ValidateToken(s string) (*db.APITokenRecord, error) {
	if s != "tb-share-a" && s != "tb-share-b" {
		return nil, errors.New("invalid token")
	}
	return &db.APITokenRecord{TokenID: s, UserID: s, TeamID: strings.TrimPrefix(s, "tb-share-"), Enabled: true}, nil
}
func (translationTokenStore) UpdateLastUsed(string) error { return nil }

type translationTestService struct{ url, backend, model string }

func translationTestRouter(t *testing.T, scenario typ.RuleScenario, backends ...translationTestService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	loadbalance.DefaultBreakerStore().Reset()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()), config.WithDisableMigration(), config.WithDisableBuiltIn())
	require.NoError(t, err)
	cfg.ModelToken = "gateway-token"
	cfg.MultiTenantConfig.Enabled = true
	services := []*loadbalance.Service{}
	for i, backend := range backends {
		id := "translation-" + backend.backend
		style := protocol.APIStyleOpenAI
		base := backend.url + "/v1"
		mode := typ.Provider{}.OpenAIEndpointMode
		switch backend.backend {
		case "libretranslate", "deepl":
			style = protocol.APIStyle(backend.backend)
			base = backend.url
		case "anthropic":
			style = protocol.APIStyleAnthropic
			base = backend.url
		case "responses":
			mode = "responses"
		}
		require.NoError(t, cfg.AddProvider(&typ.Provider{UUID: id, Name: id, Enabled: true, APIStyle: style, APIBase: base, OpenAIEndpointMode: mode, AuthType: typ.AuthTypeAPIKey, Token: "provider-key", Timeout: 20}))
		services = append(services, &loadbalance.Service{Provider: id, Model: backend.model, Active: true, Tier: i})
	}
	require.NoError(t, cfg.AddRule(typ.Rule{UUID: "translation-test-rule", Scenario: scenario, RequestModel: "translate", Active: true, Services: services,
		LBTactic: typ.Tactic{Type: loadbalance.TacticTier, Params: &typ.TierParams{WithinTierTactic: loadbalance.TacticRandom}}}))
	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	ph := NewHandler(ProtocolHandlerDeps{Config: cfg, ClientPool: client.NewClientPool(), LoadBalancer: lb, HealthMonitor: hm, TemplateManager: catalog.NewProviderCatalogManager(),
		RoutingSelector: routing.NewSimpleSelector(routing.NewServiceSelector(cfg, NewAffinityStore(0), lb))})
	r := gin.New()
	ph.RegisterRoutes(r, middleware.NewAuthMiddleware(cfg, nil, nil, translationTokenStore{}).ModelAuthMiddleware())
	return r
}

func translationTestCall(r http.Handler, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func translationTestReply(w http.ResponseWriter, text, finish string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": "mt-test", "object": "chat.completion", "model": "upstream-alias",
		"choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": map[string]any{"role": "assistant", "content": text}}},
		"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
}

func TestTranslationRoutesNativeBatchAndAuth(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer provider-key", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		calls = append(calls, body)
		mu.Unlock()
		translationTestReply(w, "译文", "stop")
	}))
	defer upstream.Close()
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{upstream.URL, translation.HunyuanMT, "tencent/Hy-MT2-1.8B"})
	for _, path := range []string{"/tingly/custom/translate", "/tingly/custom/v1/translate"} {
		w := translationTestCall(r, path, "gateway-token", `{"text":["Hello","Save changes"],"target_lang":"zh-CN"}`)
		require.Equal(t, 200, w.Code, w.Body.String())
		var result translation.Response
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		require.Equal(t, "translate", result.Model)
		require.Len(t, result.Translations, 2)
		require.Equal(t, "译文", result.Translations[1].Text)
	}
	mu.Lock()
	require.Len(t, calls, 4)
	for _, body := range calls {
		require.Equal(t, "tencent/Hy-MT2-1.8B", body["model"])
		msgs := body["messages"].([]any)
		require.Len(t, msgs, 1)
		require.Equal(t, "user", msgs[0].(map[string]any)["role"])
	}
	mu.Unlock()
	for _, token := range []string{"invalid", "tb-share-a"} {
		w := translationTestCall(r, "/tingly/custom/translate", token, `{"text":["x"],"target_lang":"en"}`)
		if token == "invalid" {
			require.Equal(t, 401, w.Code)
		} else {
			require.Equal(t, 403, w.Code)
		}
	}
}

func TestTranslationTeamIsolation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { translationTestReply(w, "hello", "stop") }))
	defer upstream.Close()
	r := translationTestRouter(t, typ.RuleScenario("team:a"), translationTestService{upstream.URL, translation.QwenMT, "qwen-mt-flash"})
	body := `{"text":["你好"],"target_lang":"en"}`
	w := translationTestCall(r, "/tingly/team/translate", "tb-share-a", body)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = translationTestCall(r, "/tingly/team/translate", "tb-share-b", body)
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "route_not_configured")
	w = translationTestCall(r, "/tingly/team:a/translate", "tb-share-a", body)
	require.NotEqual(t, 200, w.Code)
}

func TestTranslationFailoverRebuildsNativeRequestWithoutReplayingCompletedItems(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	hunyuan := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		text := body.Messages[0].Content
		mu.Lock()
		seen = append(seen, "hy:"+text)
		mu.Unlock()
		if strings.HasSuffix(text, "second") {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"message":"unavailable","type":"api_error"}}`))
			return
		}
		translationTestReply(w, "第一", "stop")
	}))
	defer hunyuan.Close()
	qwen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		text := body["messages"].([]any)[0].(map[string]any)["content"].(string)
		mu.Lock()
		seen = append(seen, "qw:"+text)
		mu.Unlock()
		require.Equal(t, "second", text)
		require.Equal(t, map[string]any{"source_lang": "auto", "target_lang": "Chinese"}, body["translation_options"])
		translationTestReply(w, "第二", "stop")
	}))
	defer qwen.Close()
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{hunyuan.URL, translation.HunyuanMT, "tencent/Hy-MT2-1.8B"}, translationTestService{qwen.URL, translation.QwenMT, "qwen-mt-flash"})
	w := translationTestCall(r, "/tingly/custom/translate", "gateway-token", `{"text":["first","second"],"target_lang":"zh-Hans"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	var out translation.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, []translation.Result{{Text: "第一"}, {Text: "第二"}}, out.Translations)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 3)
	require.Equal(t, "qw:second", seen[2])
}

func TestTranslationInvalidRequestsAndIncompleteOutputs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { translationTestReply(w, "partial", "length") }))
	defer upstream.Close()
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{upstream.URL, translation.HunyuanMT, "tencent/Hy-MT2-1.8B"})
	for _, body := range []string{
		`{"text":"scalar","target_lang":"en"}`, `{"text":["x"],"target_lang":"en","stream":true}`,
		`{"text":[""],"target_lang":"en"}`, `{"text":["x"],"target_lang":"auto"}`,
		`{"text":["x"],"target_lang":"en"} {}`, `{"text":["x"],"target_lang":"pt-BR"}`,
	} {
		w := translationTestCall(r, "/tingly/custom/translate", "gateway-token", body)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	w := translationTestCall(r, "/tingly/custom/translate", "gateway-token", `{"text":["x"],"target_lang":"en"}`)
	require.Equal(t, 502, w.Code)
	require.Contains(t, w.Body.String(), `"input_index":0`)
	require.NotContains(t, w.Body.String(), "translations")
	w = translationTestCall(r, "/tingly/custom/translate", "gateway-token", strings.Repeat("x", translation.MaxBodyBytes+1))
	require.Equal(t, 413, w.Code)
}

func TestTranslationRequestCancellation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{upstream.URL, translation.HunyuanMT, "tencent/Hy-MT2-1.8B"})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/tingly/custom/translate", strings.NewReader(`{"text":["x"],"target_lang":"en"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer gateway-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 504, w.Code, w.Body.String())
}

func TestTranslationGenericModelDoesNotNeedCatalog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "local-mt", body["model"])
		messages := body["messages"].([]any)
		require.Equal(t, "system", messages[0].(map[string]any)["role"])
		require.Contains(t, messages[0].(map[string]any)["content"], "pt-BR")
		require.Equal(t, "hello", messages[1].(map[string]any)["content"])
		translationTestReply(w, "olá", "stop")
	}))
	defer upstream.Close()
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{upstream.URL, "llm", "local-mt"})
	w := translationTestCall(r, "/tingly/custom/translate", "gateway-token", `{"text":["hello"],"source_lang":"en","target_lang":"pt-BR"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "olá")
}

func TestTranslationTraditionalFailoverToLLM(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/translate", r.URL.Path)
		require.Empty(t, r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "provider-key", body["api_key"])
		require.Equal(t, "text", body["format"])
		require.NotContains(t, body, "model")
		require.NotContains(t, body, "messages")
		if body["q"] == "second" {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"translatedText":"第一","detectedLanguage":{"language":"en","confidence":90}}`))
	}))
	defer native.Close()
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		messages := body["messages"].([]any)
		require.Equal(t, "second", messages[1].(map[string]any)["content"])
		translationTestReply(w, "第二", "stop")
	}))
	defer llm.Close()
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{native.URL, "libretranslate", ""}, translationTestService{llm.URL, "llm", "arbitrary-llm"})
	w := translationTestCall(r, "/tingly/custom/translate", "gateway-token", `{"text":["first","second"],"target_lang":"zh-Hans"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	var out translation.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, []translation.Result{{Text: "第一", DetectedSourceLanguage: "en"}, {Text: "第二"}}, out.Translations)
}

func TestTranslationDeepLNativeAccounting(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/translate", r.URL.Path)
		require.Equal(t, "DeepL-Auth-Key provider-key", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "EN-GB", body["target_lang"])
		require.NotContains(t, body, "source_lang")
		require.NotContains(t, body, "model")
		_, _ = w.Write([]byte(`{"translations":[{"text":"colour","detected_source_language":"DE","billed_characters":5}]}`))
	}))
	defer native.Close()
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{native.URL, "deepl", ""})
	w := translationTestCall(r, "/tingly/custom/translate", "gateway-token", `{"text":["Farbe"],"target_lang":"en-GB"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"billed_characters":5`)
	require.Contains(t, w.Body.String(), `"detected_source_language":"de"`)
	require.NotContains(t, w.Body.String(), "tokens")
}

func TestTranslationGenericCrossProtocol(t *testing.T) {
	for _, style := range []string{"anthropic", "responses"} {
		t.Run(style, func(t *testing.T) {
			native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if style == "anthropic" {
					require.Equal(t, "/v1/messages", r.URL.Path)
					_, _ = w.Write([]byte(`{"id":"msg-1","type":"message","role":"assistant","model":"my-llm","content":[{"type":"text","text":"你好"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":2}}`))
				} else {
					require.Equal(t, "/v1/responses", r.URL.Path)
					_, _ = w.Write([]byte(`{"id":"resp-1","object":"response","status":"completed","model":"my-llm","output":[{"id":"msg-1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"你好","annotations":[]}]}],"usage":{"input_tokens":12,"output_tokens":2,"total_tokens":14}}`))
				}
			}))
			defer native.Close()
			r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{native.URL, style, "my-llm"})
			w := translationTestCall(r, "/tingly/custom/translate", "gateway-token", `{"text":["hello"],"target_lang":"zh-Hans"}`)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "你好")
		})
	}
}

// Opt-in real model check: run the Python example first and supply its base URL.
// This exercises the authenticated gateway routes, not a direct SDK shortcut.
func TestTranslationLiveHunyuan(t *testing.T) {
	base := os.Getenv("TINGLY_TRANSLATION_LIVE_BASE_URL")
	if base == "" {
		t.Skip("set TINGLY_TRANSLATION_LIVE_BASE_URL to the local Python MT server")
	}
	r := translationTestRouter(t, typ.ScenarioCustom, translationTestService{base, translation.HunyuanMT, "tencent/Hy-MT2-1.8B"})
	for _, tt := range []struct {
		target string
		text   []string
	}{
		{"zh-Hans", []string{"Hello, world!", "Save changes"}},
		{"en", []string{"今天天气真好。"}},
	} {
		body, err := json.Marshal(translation.Request{Text: tt.text, TargetLang: tt.target})
		require.NoError(t, err)
		w := translationTestCall(r, "/tingly/custom/v1/translate", "gateway-token", string(body))
		require.Equal(t, 200, w.Code, w.Body.String())
		t.Logf("source=%q target=%s response=%s", tt.text, tt.target, w.Body.String())
	}
}
