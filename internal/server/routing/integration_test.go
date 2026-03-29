package routing_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// delayModelResponseID must match virtualmodel.delayModelResponseID
const delayModelResponseID = "delay-model"

// routingTestServer wraps a real Server for E2E routing pipeline tests.
type routingTestServer struct {
	appConfig  *config.AppConfig
	httpServer *httptest.Server
}

func newRoutingTestServer(t *testing.T) *routingTestServer {
	t.Helper()

	configDir, err := os.MkdirTemp("", "routing-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(configDir) })

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	require.NoError(t, err)

	httpServer := server.NewServer(appConfig.GetGlobalConfig(), server.WithAdaptor(false))
	ts := &routingTestServer{
		appConfig:  appConfig,
		httpServer: httptest.NewServer(httpServer.GetRouter()),
	}
	t.Cleanup(func() { ts.httpServer.Close() })
	return ts
}

// addDelayProvider registers a DelayProvider as a provider + service in the config.
func (ts *routingTestServer) addDelayProvider(t *testing.T, name string, dp *virtualmodel.DelayProvider) *loadbalance.Service {
	t.Helper()

	provider := dp.Provider(name)
	require.NoError(t, ts.appConfig.AddProvider(provider))

	svc := &loadbalance.Service{
		Provider:   provider.UUID,
		Model:      delayModelResponseID,
		Weight:     1,
		Active:     true,
		TimeWindow: 300,
	}
	return svc
}

// addRule adds a rule to the config with the given services.
func (ts *routingTestServer) addRule(t *testing.T, rule typ.Rule) {
	t.Helper()
	require.NoError(t, ts.appConfig.GetGlobalConfig().AddRequestConfig(rule))
}

func (ts *routingTestServer) token() string {
	return ts.appConfig.GetGlobalConfig().GetModelToken()
}

func sendRequest(t *testing.T, baseURL, token, model, sessionID string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"model":  model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})
	req, _ := http.NewRequest("POST", baseURL+"/tingly/openai/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if sessionID != "" {
		req.Header.Set("X-Tingly-Session-ID", sessionID)
	}

	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestIntegration_BasicRouting(t *testing.T) {
	dp := virtualmodel.NewDelayProvider()
	defer dp.Close()

	ts := newRoutingTestServer(t)
	svc := ts.addDelayProvider(t, "dp-basic", dp)
	ts.addRule(t, typ.Rule{
		UUID: "rule-basic", Scenario: typ.ScenarioOpenAI,
		RequestModel: "model-basic", ResponseModel: delayModelResponseID,
		Services: []*loadbalance.Service{svc}, Active: true,
	})

	code, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-basic", "")
	assert.Equal(t, http.StatusOK, code, "basic routing should succeed")
}

func TestIntegration_SmartRouting_Match(t *testing.T) {
	dp := virtualmodel.NewDelayProvider()
	defer dp.Close()

	ts := newRoutingTestServer(t)
	svc := ts.addDelayProvider(t, "dp-smart", dp)

	rule := typ.Rule{
		UUID: "rule-smart", Scenario: typ.ScenarioOpenAI,
		RequestModel: "model-smart", ResponseModel: delayModelResponseID,
		Services: []*loadbalance.Service{svc}, Active: true,
		SmartEnabled: true,
		SmartRouting: []smartrouting.SmartRouting{
			{
				Description: "route smart to delay",
				Ops: []smartrouting.SmartOp{
					{Position: smartrouting.PositionModel, Operation: smartrouting.OpModelContains, Value: "smart"},
				},
				Services: []*loadbalance.Service{svc},
			},
		},
	}
	ts.addRule(t, rule)

	code, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-smart", "")
	assert.Equal(t, http.StatusOK, code, "smart routing match should succeed")
}

func TestIntegration_SmartRouting_NoMatch(t *testing.T) {
	dp := virtualmodel.NewDelayProvider()
	defer dp.Close()

	ts := newRoutingTestServer(t)
	svc := ts.addDelayProvider(t, "dp-nomatch", dp)

	rule := typ.Rule{
		UUID: "rule-nomatch", Scenario: typ.ScenarioOpenAI,
		RequestModel: "model-nomatch", ResponseModel: delayModelResponseID,
		Services: []*loadbalance.Service{svc}, Active: true,
		SmartEnabled: true,
		SmartRouting: []smartrouting.SmartRouting{
			{
				Description: "route claude only",
				Ops: []smartrouting.SmartOp{
					{Position: smartrouting.PositionModel, Operation: smartrouting.OpModelContains, Value: "claude"},
				},
				Services: []*loadbalance.Service{svc},
			},
		},
	}
	ts.addRule(t, rule)

	// Model doesn't match "claude", falls through to normal LB
	code, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-nomatch", "")
	assert.Equal(t, http.StatusOK, code, "should fall through to LB when smart doesn't match")
}

func TestIntegration_Affinity_LockAndReuse(t *testing.T) {
	dpA := virtualmodel.NewDelayProviderWithConfig(virtualmodel.DelayConfig{
		MinFirstTokenDelayMs: 10, MaxFirstTokenDelayMs: 10,
		MinEndDelayMs: 10, MaxEndDelayMs: 10,
	})
	dpB := virtualmodel.NewDelayProviderWithConfig(virtualmodel.DelayConfig{
		MinFirstTokenDelayMs: 10, MaxFirstTokenDelayMs: 10,
		MinEndDelayMs: 10, MaxEndDelayMs: 10,
	})
	defer dpA.Close()
	defer dpB.Close()

	ts := newRoutingTestServer(t)
	svcA := ts.addDelayProvider(t, "dp-aff-a", dpA)
	svcB := ts.addDelayProvider(t, "dp-aff-b", dpB)

	rule := typ.Rule{
		UUID: "rule-affinity", Scenario: typ.ScenarioOpenAI,
		RequestModel: "model-affinity", ResponseModel: delayModelResponseID,
		Services: []*loadbalance.Service{svcA, svcB},
		LBTactic: typ.Tactic{Type: loadbalance.TacticRandom},
		Active:   true, SmartEnabled: true, SmartAffinity: true,
	}
	ts.addRule(t, rule)

	session := "test-affinity-session"

	// Send two requests with the same session ID
	code1, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-affinity", session)
	assert.Equal(t, http.StatusOK, code1, "first request should succeed")

	code2, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-affinity", session)
	assert.Equal(t, http.StatusOK, code2, "second request should succeed")

	// Both should succeed — affinity ensures session stickiness
	t.Logf("both requests with session=%s succeeded", session)
}

func TestIntegration_Affinity_DifferentSessions(t *testing.T) {
	dpA := virtualmodel.NewDelayProviderWithConfig(virtualmodel.DelayConfig{MinFirstTokenDelayMs: 10, MaxFirstTokenDelayMs: 10, MinEndDelayMs: 10, MaxEndDelayMs: 10})
	dpB := virtualmodel.NewDelayProviderWithConfig(virtualmodel.DelayConfig{MinFirstTokenDelayMs: 10, MaxFirstTokenDelayMs: 10, MinEndDelayMs: 10, MaxEndDelayMs: 10})
	defer dpA.Close()
	defer dpB.Close()

	ts := newRoutingTestServer(t)
	svcA := ts.addDelayProvider(t, "dp-diff-a", dpA)
	svcB := ts.addDelayProvider(t, "dp-diff-b", dpB)

	rule := typ.Rule{
		UUID: "rule-diff", Scenario: typ.ScenarioOpenAI,
		RequestModel: "model-diff", ResponseModel: delayModelResponseID,
		Services: []*loadbalance.Service{svcA, svcB},
		Active:   true, SmartEnabled: true, SmartAffinity: true,
	}
	ts.addRule(t, rule)

	// Different sessions may get different providers (round-robin)
	codeA, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-diff", "session-x")
	codeB, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-diff", "session-y")

	assert.Equal(t, http.StatusOK, codeA, "session-x should succeed")
	assert.Equal(t, http.StatusOK, codeB, "session-y should succeed")
}

func TestIntegration_Affinity_WithSmartRouting(t *testing.T) {
	dp := virtualmodel.NewDelayProviderWithConfig(virtualmodel.DelayConfig{
		MinFirstTokenDelayMs: 10, MaxFirstTokenDelayMs: 10,
		MinEndDelayMs: 10, MaxEndDelayMs: 10,
	})
	defer dp.Close()

	ts := newRoutingTestServer(t)
	svc := ts.addDelayProvider(t, "dp-smartaff", dp)

	rule := typ.Rule{
		UUID: "rule-smartaff", Scenario: typ.ScenarioOpenAI,
		RequestModel: "model-smartaff", ResponseModel: delayModelResponseID,
		Services: []*loadbalance.Service{svc}, Active: true,
		SmartEnabled: true, SmartAffinity: true,
		SmartRouting: []smartrouting.SmartRouting{
			{
				Description: "route smartaff to delay",
				Ops: []smartrouting.SmartOp{
					{Position: smartrouting.PositionModel, Operation: smartrouting.OpModelContains, Value: "smartaff"},
				},
				Services: []*loadbalance.Service{svc},
			},
		},
	}
	ts.addRule(t, rule)

	session := "test-smartaff-session"

	// First request: smart routing matches and locks affinity
	code1, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-smartaff", session)
	assert.Equal(t, http.StatusOK, code1, "first request should succeed")

	// Second request: should use affinity (locked from first)
	code2, _ := sendRequest(t, ts.httpServer.URL, ts.token(), "model-smartaff", session)
	assert.Equal(t, http.StatusOK, code2, "second request should succeed via affinity")

	t.Logf("smart routing + affinity: both requests with session=%s succeeded", session)
}

func init() {
	gin.SetMode(gin.TestMode)
	logrus.SetOutput(io.Discard)
}
