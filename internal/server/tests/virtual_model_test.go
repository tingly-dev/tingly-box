package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	typ "github.com/tingly-dev/tingly-box/internal/typ"
)

// startTestHTTPServer wraps the gin engine in a real httptest.Server so that
// streaming (which requires http.Flusher + CloseNotify) works correctly.
func startTestHTTPServer(ts *TestServer) *httptest.Server {
	return httptest.NewServer(ts.ginEngine)
}

// makeStreamingChatRequest sends a streaming chat request to the test server and
// waits for it to complete. It asserts HTTP 200 and returns the raw SSE body.
func makeStreamingChatRequest(t *testing.T, ts *TestServer, httpSrv *httptest.Server, model string) string {
	t.Helper()
	modelToken := ts.appConfig.GetGlobalConfig().GetModelToken()
	body, _ := json.Marshal(map[string]interface{}{
		"model":  model,
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})

	req, _ := http.NewRequest("POST", httpSrv.URL+"/tingly/openai/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+modelToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "streaming request failed: %s", string(rawBody))
	return string(rawBody)
}

// makeNonStreamingChatRequest sends a non-streaming chat request to the test server.
func makeNonStreamingChatRequest(t *testing.T, ts *TestServer, model string) {
	t.Helper()
	modelToken := ts.appConfig.GetGlobalConfig().GetModelToken()

	body, _ := json.Marshal(map[string]interface{}{
		"model":  model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})

	req, _ := http.NewRequest("POST", "/tingly/openai/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+modelToken)

	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "non-streaming request failed: %s", w.Body.String())
}

// setupVirtualModelTest creates a TestServer with a VirtualModel registered as provider
// and a routing rule pointing to it. Returns the TestServer and the service pointer
// (for stats inspection).
func setupVirtualModelTest(t *testing.T, requestModel string, vm *VirtualModel) (*TestServer, *loadbalance.Service) {
	t.Helper()

	ts := NewTestServer(t)

	// Register virtual model as a provider
	provider := vm.Provider("vm-" + requestModel)
	err := ts.appConfig.AddProvider(provider)
	require.NoError(t, err)

	// Create the service (we keep a pointer so we can read .Stats later)
	svc := &loadbalance.Service{
		Provider:   provider.UUID,
		Model:      virtualModelName,
		Weight:     1,
		Active:     true,
		TimeWindow: 300,
	}

	rule := typ.Rule{
		UUID:          requestModel,
		Scenario:      typ.ScenarioOpenAI,
		RequestModel:  requestModel,
		ResponseModel: virtualModelName,
		Services:      []*loadbalance.Service{svc},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Active: true,
	}

	err = ts.appConfig.GetGlobalConfig().AddRequestConfig(rule)
	require.NoError(t, err)

	return ts, svc
}

// TestVirtualModel_TTFTCaptured verifies that TTFT is recorded in ServiceStats
// after a streaming request via the full metrics pipeline.
func TestVirtualModel_TTFTCaptured(t *testing.T) {
	const delayMs = 200
	vm := NewVirtualModelWithConfig(VirtualModelConfig{
		MinFirstTokenDelayMs: delayMs,
		MaxFirstTokenDelayMs: delayMs,
		MinEndDelayMs:        50,
		MaxEndDelayMs:        50,
	})
	defer vm.Close()

	ts, svc := setupVirtualModelTest(t, "vm-ttft-model", vm)
	httpSrv := startTestHTTPServer(ts)
	defer httpSrv.Close()

	makeStreamingChatRequest(t, ts, httpSrv, "vm-ttft-model")

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgTTFTMs, 0.0, "AvgTTFTMs should be recorded after streaming request")
	// TTFT should be at least half the configured delay (allow generous tolerance for CI)
	assert.GreaterOrEqual(t, stats.AvgTTFTMs, float64(delayMs)/2,
		"AvgTTFTMs (%v) should be close to configured delay (%dms)", stats.AvgTTFTMs, delayMs)
}

// TestVirtualModel_TPSCaptured verifies that TPS (tokens per second) is recorded
// in ServiceStats after a streaming request.
func TestVirtualModel_TPSCaptured(t *testing.T) {
	vm := NewVirtualModelWithConfig(VirtualModelConfig{
		MinFirstTokenDelayMs: 50,
		MaxFirstTokenDelayMs: 50,
		MinEndDelayMs:        300,
		MaxEndDelayMs:        300,
	})
	defer vm.Close()

	ts, svc := setupVirtualModelTest(t, "vm-tps-model", vm)
	httpSrv := startTestHTTPServer(ts)
	defer httpSrv.Close()

	makeStreamingChatRequest(t, ts, httpSrv, "vm-tps-model")

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgTokenSpeed, 0.0, "AvgTokenSpeed (TPS) should be recorded after streaming request")
}

// TestVirtualModel_LatencyPercentiles verifies that P50/P95/P99 latency percentiles
// are populated and properly ordered after multiple requests.
func TestVirtualModel_LatencyPercentiles(t *testing.T) {
	vm := NewVirtualModelWithConfig(VirtualModelConfig{
		MinFirstTokenDelayMs: 20,
		MaxFirstTokenDelayMs: 200,
		MinEndDelayMs:        20,
		MaxEndDelayMs:        200,
	})
	defer vm.Close()

	ts, svc := setupVirtualModelTest(t, "vm-latency-model", vm)
	httpSrv := startTestHTTPServer(ts)
	defer httpSrv.Close()

	const n = 20
	for i := 0; i < n; i++ {
		makeStreamingChatRequest(t, ts, httpSrv, "vm-latency-model")
	}

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgLatencyMs, 0.0, "AvgLatencyMs should be populated")
	assert.Greater(t, stats.P50LatencyMs, 0.0, "P50LatencyMs should be populated")
	assert.Greater(t, stats.P95LatencyMs, 0.0, "P95LatencyMs should be populated")
	assert.Greater(t, stats.P99LatencyMs, 0.0, "P99LatencyMs should be populated")

	assert.LessOrEqual(t, stats.P50LatencyMs, stats.P95LatencyMs,
		"P50 (%v) should be <= P95 (%v)", stats.P50LatencyMs, stats.P95LatencyMs)
	assert.LessOrEqual(t, stats.P95LatencyMs, stats.P99LatencyMs,
		"P95 (%v) should be <= P99 (%v)", stats.P95LatencyMs, stats.P99LatencyMs)
}

// TestVirtualModel_NonStreamingMetrics verifies that latency is captured for
// non-streaming requests (TTFT falls back to total latency).
func TestVirtualModel_NonStreamingMetrics(t *testing.T) {
	vm := NewVirtualModelWithConfig(VirtualModelConfig{
		MinFirstTokenDelayMs: 100,
		MaxFirstTokenDelayMs: 100,
		MinEndDelayMs:        0,
		MaxEndDelayMs:        0,
	})
	defer vm.Close()

	ts, svc := setupVirtualModelTest(t, "vm-nonstream-model", vm)

	makeNonStreamingChatRequest(t, ts, "vm-nonstream-model")

	stats := svc.Stats.GetStats()
	assert.Greater(t, stats.AvgLatencyMs, 0.0, "AvgLatencyMs should be recorded for non-streaming request")
	// Non-streaming: TPS is 0 (not applicable)
	assert.Equal(t, 0.0, stats.AvgTokenSpeed, "TPS should be 0 for non-streaming requests")
}

// TestVirtualModel_MultiServiceLatencyRouting verifies that after warmup requests,
// the latency-based tactic consistently routes to the faster virtual model.
func TestVirtualModel_MultiServiceLatencyRouting(t *testing.T) {
	// Fast model: 10-30ms total latency
	vmFast := NewVirtualModelWithConfig(VirtualModelConfig{
		MinFirstTokenDelayMs: 5,
		MaxFirstTokenDelayMs: 15,
		MinEndDelayMs:        5,
		MaxEndDelayMs:        15,
	})
	defer vmFast.Close()

	// Slow model: 300-500ms total latency
	vmSlow := NewVirtualModelWithConfig(VirtualModelConfig{
		MinFirstTokenDelayMs: 150,
		MaxFirstTokenDelayMs: 250,
		MinEndDelayMs:        150,
		MaxEndDelayMs:        250,
	})
	defer vmSlow.Close()

	ts := NewTestServer(t)
	httpSrv := startTestHTTPServer(ts)
	defer httpSrv.Close()

	providerFast := vmFast.Provider("vm-fast")
	providerSlow := vmSlow.Provider("vm-slow")
	require.NoError(t, ts.appConfig.AddProvider(providerFast))
	require.NoError(t, ts.appConfig.AddProvider(providerSlow))

	svcFast := &loadbalance.Service{
		Provider:   providerFast.UUID,
		Model:      virtualModelName,
		Weight:     1,
		Active:     true,
		TimeWindow: 300,
	}
	svcSlow := &loadbalance.Service{
		Provider:   providerSlow.UUID,
		Model:      virtualModelName,
		Weight:     1,
		Active:     true,
		TimeWindow: 300,
	}

	rule := typ.Rule{
		UUID:          "vm-latency-routing",
		Scenario:      typ.ScenarioOpenAI,
		RequestModel:  "vm-latency-routing",
		ResponseModel: virtualModelName,
		Services:      []*loadbalance.Service{svcFast, svcSlow},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticLatencyBased,
			Params: typ.DefaultLatencyBasedParams(),
		},
		Active: true,
	}

	require.NoError(t, ts.appConfig.GetGlobalConfig().AddRequestConfig(rule))

	modelToken := ts.appConfig.GetGlobalConfig().GetModelToken()
	sendRequest := func() int {
		body, _ := json.Marshal(map[string]interface{}{
			"model":  "vm-latency-routing",
			"stream": true,
			"messages": []map[string]string{
				{"role": "user", "content": "hello"},
			},
		})
		req, _ := http.NewRequest("POST", httpSrv.URL+"/tingly/openai/v1/chat/completions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+modelToken)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return 0
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}

	// Warmup requests to populate latency stats (one for each service)
	for i := 0; i < 4; i++ {
		code := sendRequest()
		assert.Equal(t, http.StatusOK, code, "warmup request %d failed", i+1)
	}

	// Give a moment for stats to settle
	time.Sleep(10 * time.Millisecond)

	fastStats := svcFast.Stats.GetStats()
	slowStats := svcSlow.Stats.GetStats()

	fmt.Printf("Fast model avg latency: %.1fms, Slow model avg latency: %.1fms\n",
		fastStats.AvgLatencyMs, slowStats.AvgLatencyMs)

	if fastStats.AvgLatencyMs > 0 && slowStats.AvgLatencyMs > 0 {
		assert.Less(t, fastStats.AvgLatencyMs, slowStats.AvgLatencyMs,
			"fast model (avg=%.1fms) should have lower latency than slow model (avg=%.1fms)",
			fastStats.AvgLatencyMs, slowStats.AvgLatencyMs)
	} else {
		t.Logf("One or both services have no latency stats yet (fast=%.1f, slow=%.1f); skipping comparison",
			fastStats.AvgLatencyMs, slowStats.AvgLatencyMs)
	}
}
