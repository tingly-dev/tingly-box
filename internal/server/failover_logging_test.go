package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	internalobs "github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// logCapture is a logrus hook that records every entry fired through it.
type logCapture struct {
	entries []*logrus.Entry
}

func (h *logCapture) Levels() []logrus.Level { return logrus.AllLevels }
func (h *logCapture) Fire(e *logrus.Entry) error {
	// Copy fields to avoid mutation after Fire returns.
	fields := make(logrus.Fields, len(e.Data))
	for k, v := range e.Data {
		fields[k] = v
	}
	h.entries = append(h.entries, &logrus.Entry{
		Logger:  e.Logger,
		Data:    fields,
		Time:    e.Time,
		Level:   e.Level,
		Message: e.Message,
	})
	return nil
}

// printCapturedLog prints entries in a human-readable table for manual inspection.
func printCapturedLog(t *testing.T, entries []*logrus.Entry) {
	t.Helper()
	t.Log("\n── captured log entries ──────────────────────────────────────────")
	for i, e := range entries {
		fields := make([]string, 0, len(e.Data))
		for k, v := range e.Data {
			fields = append(fields, fmt.Sprintf("%s=%v", k, v))
		}
		t.Logf("[%d] level=%-5s msg=%q\n        fields={%s}", i, e.Level, e.Message, strings.Join(fields, "  "))
	}
	t.Log("──────────────────────────────────────────────────────────────────")
}

// TestFailoverLogging exercises dispatchWithPriorityFailover and verifies
// the structured log fields emitted during retry and give-up scenarios.
//
// Run with:
//
//	go test -v -run TestFailoverLogging ./internal/server/
//
// The captured log entries are printed for human inspection.
func TestFailoverLogging_RetryAndGiveUp(t *testing.T) {
	// Build a real (temp-dir backed) config so GetProviderByUUID works.
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	providerT0 := &typ.Provider{UUID: "prov-t0", Name: "ProviderAlpha", Enabled: true, APIStyle: "openai", APIBase: "https://api.example-alpha.com/v1"}
	providerT1 := &typ.Provider{UUID: "prov-t1", Name: "ProviderBeta", Enabled: true, APIStyle: "openai", APIBase: "https://api.example-beta.com/v1"}
	if err := cfg.AddProvider(providerT0); err != nil {
		t.Fatalf("AddProvider T0: %v", err)
	}
	if err := cfg.AddProvider(providerT1); err != nil {
		t.Fatalf("AddProvider T1: %v", err)
	}

	rule := &typ.Rule{
		UUID:         "test-rule",
		Scenario:     typ.ScenarioClaudeCode,
		RequestModel: "cc",
		LBTactic: typ.Tactic{
			Type: loadbalance.TacticTier,
			Params: &typ.TierParams{
				WithinTierTactic: loadbalance.TacticRandom,
			},
		},
		Services: []*loadbalance.Service{
			{Provider: providerT0.UUID, Model: "gpt-4o", Active: true, Tier: 0},
			{Provider: providerT1.UUID, Model: "gpt-4o-mini", Active: true, Tier: 1},
		},
	}

	hm := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	hf := typ.NewHealthFilter(hm)
	h := NewHandler(ProtocolHandlerDeps{
		Config:        cfg,
		LoadBalancer:  NewLoadBalancer(cfg, hf),
		HealthMonitor: hm,
	})

	newCtx := func(reqID string) *gin.Context {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
		c.Request = c.Request.WithContext(obs.ContextWithRequestID(c.Request.Context(), reqID))
		return c
	}

	t.Run("retry_then_succeed", func(t *testing.T) {
		hook := &logCapture{}
		logrus.AddHook(hook)
		defer logrus.StandardLogger().ReplaceHooks(logrus.LevelHooks{})

		c := newCtx("req-retry-001")
		callCount := 0
		attempt := func(provider *typ.Provider, model string) {
			callCount++
			if callCount == 1 {
				// Tier-0 returns 429 — retryable, gate stays uncommitted.
				c.Writer.WriteHeader(http.StatusTooManyRequests)
			} else {
				// Tier-1 succeeds and commits the gate.
				c.Writer.WriteHeader(http.StatusOK)
				if gate, ok := c.Writer.(*firstChunkGate); ok {
					gate.CommitFirstChunk()
				}
			}
		}

		h.DispatchWithPriorityFailover(c, rule, providerT0, "gpt-4o", attempt)

		printCapturedLog(t, hook.entries)

		var sawRetry, sawSuccess bool
		for _, e := range hook.entries {
			if stage, _ := e.Data["stage"].(string); strings.HasPrefix(stage, "failover_") {
				if e.Data["scenario"] != string(typ.ScenarioClaudeCode) {
					t.Errorf("%s log scenario = %v, want %s", stage, e.Data["scenario"], typ.ScenarioClaudeCode)
				}
				if e.Data["request_model"] != "cc" {
					t.Errorf("%s log request_model = %v, want cc", stage, e.Data["request_model"])
				}
				if e.Data["attempt_model"] == nil {
					t.Errorf("%s log missing attempt_model", stage)
				}
			}
			if e.Data["stage"] == "failover_retry" && e.Data["to_provider"] != nil {
				sawRetry = true
				for _, field := range []string{"attempt", "status", "from_service", "to_provider", "to_model"} {
					if e.Data[field] == nil {
						t.Errorf("retry log missing field: %s", field)
					}
				}
			}
			if e.Data["stage"] == "failover_success" {
				sawSuccess = true
				if e.Data["routed_model"] != "gpt-4o-mini" {
					t.Errorf("success routed_model = %v, want gpt-4o-mini", e.Data["routed_model"])
				}
			}
		}
		if !sawRetry {
			t.Error("expected a failover retry log entry with to_provider field, got none")
		}
		if !sawSuccess {
			t.Error("expected a failover success log entry, got none")
		}
		if got := c.GetString(ContextKeyModel); got != "gpt-4o-mini" {
			t.Errorf("final tracking model = %q, want gpt-4o-mini", got)
		}
	})

	t.Run("all_services_fail_give_up", func(t *testing.T) {
		hook := &logCapture{}
		logrus.AddHook(hook)
		defer logrus.StandardLogger().ReplaceHooks(logrus.LevelHooks{})

		c := newCtx("req-giveup-001")
		attempt := func(provider *typ.Provider, model string) {
			// Every attempt 429s — gate never commits.
			c.Writer.WriteHeader(http.StatusTooManyRequests)
		}

		h.DispatchWithPriorityFailover(c, rule, providerT0, "gpt-4o", attempt)

		printCapturedLog(t, hook.entries)

		var sawGiveUp bool
		for _, e := range hook.entries {
			if e.Data["stage"] == "failover_exhausted" && e.Data["to_provider"] == nil && e.Data["attempt"] != nil {
				sawGiveUp = true
				if e.Level != logrus.WarnLevel {
					t.Errorf("give-up log should be Warn level, got %s", e.Level)
				}
				for _, field := range []string{"attempt", "status"} {
					if e.Data[field] == nil {
						t.Errorf("give-up log missing field: %s", field)
					}
				}
			}
		}
		if !sawGiveUp {
			t.Error("expected a failover give-up log entry, got none")
		}
	})
}

func TestFailoverRecordsBreakerFailureIndependentlyOfRecorder(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	providerT0 := &typ.Provider{UUID: "breaker-prov-t0", Name: "ProviderAlpha", Enabled: true, APIStyle: "openai", APIBase: "https://api.example-alpha.com/v1"}
	providerT1 := &typ.Provider{UUID: "breaker-prov-t1", Name: "ProviderBeta", Enabled: true, APIStyle: "openai", APIBase: "https://api.example-beta.com/v1"}
	if err := cfg.AddProvider(providerT0); err != nil {
		t.Fatalf("AddProvider T0: %v", err)
	}
	if err := cfg.AddProvider(providerT1); err != nil {
		t.Fatalf("AddProvider T1: %v", err)
	}

	rule := &typ.Rule{
		UUID: "breaker-recording-disabled-rule",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{
			{Provider: providerT0.UUID, Model: "primary", Active: true, Tier: 0},
			{Provider: providerT1.UUID, Model: "fallback", Active: true, Tier: 1},
		},
	}

	hm := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	hf := typ.NewHealthFilter(hm)
	h := NewHandler(ProtocolHandlerDeps{
		Config:        cfg,
		LoadBalancer:  NewLoadBalancer(cfg, hf),
		HealthMonitor: hm,
	})

	for _, tc := range []struct {
		name           string
		attachRecorder bool
	}{
		{name: "recorder_disabled"},
		{name: "recorder_attached", attachRecorder: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loadbalance.DefaultBreakerStore().Reset()
			defer loadbalance.DefaultBreakerStore().Reset()

			for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", nil)
				var protocolRecorder *recording.ProtocolRecorder
				if tc.attachRecorder {
					protocolRecorder, err = recording.NewProtocolRecorder(c, nil, "test", internalobs.RecordModeStagedRequestResponse, []byte(`{}`))
					if err != nil {
						t.Fatalf("NewProtocolRecorder: %v", err)
					}
					c.Set(recording.RecorderContextKey, protocolRecorder)
				}

				attempt := func(provider *typ.Provider, model string) {
					if provider.UUID == providerT0.UUID {
						c.Writer.WriteHeader(http.StatusTooManyRequests)
						if protocolRecorder != nil {
							protocolRecorder.RecordError(errors.New("upstream 429"))
						}
						return
					}
					c.Writer.WriteHeader(http.StatusOK)
					if gate, ok := c.Writer.(*firstChunkGate); ok {
						gate.CommitFirstChunk()
					}
				}

				h.DispatchWithPriorityFailover(c, rule, providerT0, "primary", attempt)

				id := loadbalance.FormatServiceID(providerT0.UUID, "primary")
				state := loadbalance.DefaultBreakerStore().Get(rule.UUID, id).State()
				if i < loadbalance.DefaultBreakerFailureThreshold-1 && state != loadbalance.BreakerClosed {
					t.Fatalf("after %d failure(s), breaker state for %s = %s, want closed", i+1, id, state)
				}
			}

			id := loadbalance.FormatServiceID(providerT0.UUID, "primary")
			if got := loadbalance.DefaultBreakerStore().Get(rule.UUID, id).State(); got != loadbalance.BreakerOpen {
				t.Fatalf("breaker state for %s = %s, want open", id, got)
			}
		})
	}
}
