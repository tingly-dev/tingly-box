package protocoltest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// SetupDecisionRoute wires a gateway rule on the openai scenario to an
// OpenAI-style provider backed by a real vmodel virtualserver (the production
// decisions service, not a scenario fixture), answering as vmodelID (e.g.
// "decision-first"). Decisions are an OpenAI-native endpoint, so this is an
// ordinary openai provider; only the upstream is virtual. It returns the
// gateway-facing request model.
func (env *TestEnv) SetupDecisionRoute(t *testing.T, vmodelID string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	virtualserver.NewService().SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	requestModel := "decision-" + vmodelID
	providerUUID := "virtual-decision-" + vmodelID
	_ = env.appConfig.AddProvider(&typ.Provider{
		UUID: providerUUID, Name: providerUUID, APIBase: srv.URL + "/v1", APIStyle: protocol.APIStyleOpenAI,
		Token: "decision-token", Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
	})
	rule := newHarnessRule(requestModel, typ.ScenarioOpenAI, requestModel, vmodelID,
		harnessService(providerUUID, vmodelID))
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add decision rule: %v", err)
	}
	return requestModel
}

// SendDecision posts body to the gateway's decisions endpoint and returns the
// HTTP status and raw response body.
func (env *TestEnv) SendDecision(t *testing.T, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest("POST", env.gatewayServer.URL+"/tingly/openai/v1/decisions", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.modelToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send decision: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read decision response: %v", fmt.Sprint(err))
	}
	return resp.StatusCode, raw
}
