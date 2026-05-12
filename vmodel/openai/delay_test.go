package openai_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
)

// metricsTestServer is a minimal test harness that wraps the proxy server.
type metricsTestServer struct {
	appConfig *config.AppConfig
	ginEngine interface {
		ServeHTTP(http.ResponseWriter, *http.Request)
	}
}

func newMetricsTestServer(t *testing.T) *metricsTestServer {
	t.Helper()
	configDir, err := os.MkdirTemp("", "vm-metrics-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(configDir) })

	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	require.NoError(t, err)

	httpServer := server.NewServer(appConfig.GetGlobalConfig(), server.WithAdaptor(false))
	return &metricsTestServer{
		appConfig: appConfig,
		ginEngine: httpServer.GetRouter(),
	}
}

// addDelayProvider registers a DelayProvider as a provider + routing rule.
func (ts *metricsTestServer) addDelayProvider(t *testing.T, requestModel string, dp *openaivm.DelayProvider) *loadbalance.Service {
	t.Helper()

	provider := dp.Provider("dp-" + requestModel)
	require.NoError(t, ts.appConfig.AddProvider(provider))

	svc := &loadbalance.Service{
		Provider:   provider.UUID,
		Model:      delayModelResponseID,
		Weight:     1,
		Active:     true,
		TimeWindow: 300,
	}

	rule := typ.Rule{
		UUID:          requestModel,
		Scenario:      typ.ScenarioOpenAI,
		RequestModel:  requestModel,
		ResponseModel: delayModelResponseID,
		Services:      []*loadbalance.Service{svc},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Active: true,
	}
	require.NoError(t, ts.appConfig.GetGlobalConfig().AddRequestConfig(rule))
	return svc
}

func (ts *metricsTestServer) modelToken() string {
	return ts.appConfig.GetGlobalConfig().GetModelToken()
}

// delayModelResponseID is the model name that the DelayProvider reports in responses.
const delayModelResponseID = "delay-model"

func (ts *metricsTestServer) startHTTP() *httptest.Server {
	return httptest.NewServer(ts.ginEngine)
}

func sendStreamingRequest(t *testing.T, baseURL, modelToken, model string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"model":  model,
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})
	req, _ := http.NewRequest("POST", baseURL+"/tingly/openai/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+modelToken)

	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func sendNonStreamingRequest(t *testing.T, ts *metricsTestServer, model string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"model":  model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})
	req, _ := http.NewRequest("POST", "/tingly/openai/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.modelToken())

	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "non-streaming request failed: %s", w.Body.String())
}

func TestDelayProvider_TTFTCaptured(t *testing.T) {
	const delayMs = 200
	dp := openaivm.NewDelayProviderWithConfig(openaivm.DelayConfig{
		MinFirstTokenDelayMs: delayMs,
		MaxFirstTokenDelayMs: delayMs,
		MinEndDelayMs:        50,
		MaxEndDelayMs:        50,
	})
	defer dp.Close()

	ts := newMetricsTestServer(t)
	svc := ts.addDelayProvider(t, "dp-ttft", dp)
	httpSrv := ts.startHTTP()
	defer httpSrv.Close()

	code, body := sendStreamingRequest(t, httpSrv.URL, ts.modelToken(), "dp-ttft")
	assert.Equal(t, http.StatusOK, code, "request failed: %s", body)

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgTTFTMs, 0.0, "AvgTTFTMs should be recorded")
	assert.GreaterOrEqual(t, stats.AvgTTFTMs, float64(delayMs)/2,
		"AvgTTFTMs (%.1f) should reflect configured delay (%dms)", stats.AvgTTFTMs, delayMs)
	t.Logf("metrics: TTFT avg=%.1fms p50=%.1fms p95=%.1fms p99=%.1fms (configured delay=%dms)",
		stats.AvgTTFTMs, stats.P50TTFTMs, stats.P95TTFTMs, stats.P99TTFTMs, delayMs)
}

func TestDelayProvider_TPSCaptured(t *testing.T) {
	dp := openaivm.NewDelayProviderWithConfig(openaivm.DelayConfig{
		MinFirstTokenDelayMs: 50,
		MaxFirstTokenDelayMs: 50,
		MinEndDelayMs:        300,
		MaxEndDelayMs:        300,
	})
	defer dp.Close()

	ts := newMetricsTestServer(t)
	svc := ts.addDelayProvider(t, "dp-tps", dp)
	httpSrv := ts.startHTTP()
	defer httpSrv.Close()

	code, body := sendStreamingRequest(t, httpSrv.URL, ts.modelToken(), "dp-tps")
	assert.Equal(t, http.StatusOK, code, "request failed: %s", body)

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgTokenSpeed, 0.0, "AvgTokenSpeed (TPS) should be recorded")
	t.Logf("metrics: TPS avg=%.2f tok/s  latency avg=%.1fms  TTFT avg=%.1fms",
		stats.AvgTokenSpeed, stats.AvgLatencyMs, stats.AvgTTFTMs)
}

func TestDelayProvider_LatencyPercentiles(t *testing.T) {
	dp := openaivm.NewDelayProviderWithConfig(openaivm.DelayConfig{
		MinFirstTokenDelayMs: 20,
		MaxFirstTokenDelayMs: 200,
		MinEndDelayMs:        20,
		MaxEndDelayMs:        200,
	})
	defer dp.Close()

	ts := newMetricsTestServer(t)
	svc := ts.addDelayProvider(t, "dp-latency", dp)
	httpSrv := ts.startHTTP()
	defer httpSrv.Close()

	for i := 0; i < 20; i++ {
		code, body := sendStreamingRequest(t, httpSrv.URL, ts.modelToken(), "dp-latency")
		assert.Equal(t, http.StatusOK, code, "request %d failed: %s", i+1, body)
	}

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgLatencyMs, 0.0, "AvgLatencyMs should be populated")
	assert.Greater(t, stats.P50LatencyMs, 0.0, "P50LatencyMs should be populated")
	assert.Greater(t, stats.P95LatencyMs, 0.0, "P95LatencyMs should be populated")
	assert.Greater(t, stats.P99LatencyMs, 0.0, "P99LatencyMs should be populated")
	assert.LessOrEqual(t, stats.P50LatencyMs, stats.P95LatencyMs, "P50 <= P95")
	assert.LessOrEqual(t, stats.P95LatencyMs, stats.P99LatencyMs, "P95 <= P99")
	t.Logf("metrics (n=20): latency avg=%.1fms p50=%.1fms p95=%.1fms p99=%.1fms",
		stats.AvgLatencyMs, stats.P50LatencyMs, stats.P95LatencyMs, stats.P99LatencyMs)
}

func TestDelayProvider_NonStreamingMetrics(t *testing.T) {
	dp := openaivm.NewDelayProviderWithConfig(openaivm.DelayConfig{
		MinFirstTokenDelayMs: 100,
		MaxFirstTokenDelayMs: 100,
	})
	defer dp.Close()

	ts := newMetricsTestServer(t)
	svc := ts.addDelayProvider(t, "dp-nonstream", dp)

	sendNonStreamingRequest(t, ts, "dp-nonstream")

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgLatencyMs, 0.0, "AvgLatencyMs should be recorded")
	assert.Equal(t, 0.0, stats.AvgTokenSpeed, "TPS should be 0 for non-streaming")
	t.Logf("metrics: latency avg=%.1fms  TPS=%.2f tok/s  TTFT avg=%.1fms",
		stats.AvgLatencyMs, stats.AvgTokenSpeed, stats.AvgTTFTMs)
}

func TestDelayProvider_MultiServiceLatencyRouting(t *testing.T) {
	dpFast := openaivm.NewDelayProviderWithConfig(openaivm.DelayConfig{
		MinFirstTokenDelayMs: 5, MaxFirstTokenDelayMs: 15,
		MinEndDelayMs: 5, MaxEndDelayMs: 15,
	})
	defer dpFast.Close()

	dpSlow := openaivm.NewDelayProviderWithConfig(openaivm.DelayConfig{
		MinFirstTokenDelayMs: 150, MaxFirstTokenDelayMs: 250,
		MinEndDelayMs: 150, MaxEndDelayMs: 250,
	})
	defer dpSlow.Close()

	ts := newMetricsTestServer(t)
	httpSrv := ts.startHTTP()
	defer httpSrv.Close()

	providerFast := dpFast.Provider("dp-fast")
	providerSlow := dpSlow.Provider("dp-slow")
	require.NoError(t, ts.appConfig.AddProvider(providerFast))
	require.NoError(t, ts.appConfig.AddProvider(providerSlow))

	svcFast := &loadbalance.Service{Provider: providerFast.UUID, Model: delayModelResponseID, Weight: 1, Active: true, TimeWindow: 300}
	svcSlow := &loadbalance.Service{Provider: providerSlow.UUID, Model: delayModelResponseID, Weight: 1, Active: true, TimeWindow: 300}

	require.NoError(t, ts.appConfig.GetGlobalConfig().AddRequestConfig(typ.Rule{
		UUID: "dp-routing", Scenario: typ.ScenarioOpenAI,
		RequestModel: "dp-routing", ResponseModel: delayModelResponseID,
		Services: []*loadbalance.Service{svcFast, svcSlow},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticLatencyBased,
			Params: typ.DefaultLatencyBasedParams(),
		},
		Active: true,
	}))

	for i := 0; i < 4; i++ {
		code, body := sendStreamingRequest(t, httpSrv.URL, ts.modelToken(), "dp-routing")
		assert.Equal(t, http.StatusOK, code, "warmup %d failed: %s", i+1, body)
	}
	time.Sleep(10 * time.Millisecond)

	fastStats := svcFast.Stats.GetStats()
	slowStats := svcSlow.Stats.GetStats()
	t.Logf("metrics: fast avg=%.1fms (p50=%.1f p95=%.1f)  slow avg=%.1fms (p50=%.1f p95=%.1f)",
		fastStats.AvgLatencyMs, fastStats.P50LatencyMs, fastStats.P95LatencyMs,
		slowStats.AvgLatencyMs, slowStats.P50LatencyMs, slowStats.P95LatencyMs)

	if fastStats.AvgLatencyMs > 0 && slowStats.AvgLatencyMs > 0 {
		assert.Less(t, fastStats.AvgLatencyMs, slowStats.AvgLatencyMs,
			"fast (%.1fms) should be lower than slow (%.1fms)", fastStats.AvgLatencyMs, slowStats.AvgLatencyMs)
	} else {
		t.Logf("stats not yet populated (fast=%.1f slow=%.1f); skipping comparison", fastStats.AvgLatencyMs, slowStats.AvgLatencyMs)
	}
}

func init() {
	gin.SetMode(gin.TestMode)
	logrus.SetOutput(io.Discard)
	_ = constant.DefaultRequestTimeout
}
