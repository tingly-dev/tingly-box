package routing

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ginCtxWithHeader creates a minimal gin.Context carrying a single request header.
func ginCtxWithHeader(t *testing.T, key, value string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/", nil)
	if key != "" {
		req.Header.Set(key, value)
	}
	c.Request = req
	return c
}

func newSimpleSelector(cfg *mockConfig) *SimpleSelector {
	lb := &mockLoadBalancer{}
	store := newMockAffinityStore()
	sel := NewServiceSelector(cfg, store, lb)
	return NewSimpleSelector(sel)
}

// TestSelectService_ProbeServicePin verifies that X-Tingly-Probe-Service bypasses
// the selection pipeline and pins to the specified provider+model.
func TestSelectService_ProbeServicePin(t *testing.T) {
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
			"provider-b": testProvider("provider-b", "ProviderB", true),
		},
	}
	// Pipeline would normally return provider-b via load balancer.
	svcB := testService("provider-b", "claude-3-opus", true)
	lb := &mockLoadBalancer{service: svcB}
	store := newMockAffinityStore()
	sel := NewServiceSelector(cfg, store, lb)
	simple := NewSimpleSelector(sel)

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svcB})
	c := ginCtxWithHeader(t, "X-Tingly-Probe-Service", "provider-a:gpt-4-turbo")

	provider, svc, err := simple.SelectService(c, typ.ScenarioOpenAI, rule, nil)
	require.NoError(t, err)

	// Must return provider-a (pinned), not provider-b (LB choice).
	assert.Equal(t, "provider-a", provider.UUID)
	assert.Equal(t, "provider-a", svc.Provider)
	assert.Equal(t, "gpt-4-turbo", svc.Model)
}

// TestSelectService_ProbeServicePin_DisabledProvider errors when the pinned
// provider is disabled.
func TestSelectService_ProbeServicePin_DisabledProvider(t *testing.T) {
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", false), // disabled
		},
	}
	simple := newSimpleSelector(cfg)
	rule := testRule("rule-1", "gpt-4", nil)
	c := ginCtxWithHeader(t, "X-Tingly-Probe-Service", "provider-a:gpt-4")

	_, _, err := simple.SelectService(c, typ.ScenarioOpenAI, rule, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

// TestSelectService_ProbeServicePin_UnknownProvider errors when the pinned
// provider UUID doesn't exist.
func TestSelectService_ProbeServicePin_UnknownProvider(t *testing.T) {
	cfg := &mockConfig{providers: map[string]*typ.Provider{}}
	simple := newSimpleSelector(cfg)
	rule := testRule("rule-1", "gpt-4", nil)
	c := ginCtxWithHeader(t, "X-Tingly-Probe-Service", "no-such-uuid:gpt-4")

	_, _, err := simple.SelectService(c, typ.ScenarioOpenAI, rule, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSelectService_NoProbeHeader_FallsThrough confirms that without the
// probe header the normal pipeline runs (load balancer picks the service).
func TestSelectService_NoProbeHeader_FallsThrough(t *testing.T) {
	svc := testService("provider-a", "gpt-4", true)
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	sel := NewServiceSelector(cfg, store, lb)
	simple := NewSimpleSelector(sel)

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svc})
	c := ginCtxWithHeader(t, "", "") // no probe header

	provider, _, err := simple.SelectService(c, typ.ScenarioOpenAI, rule, nil)
	require.NoError(t, err)
	assert.Equal(t, "provider-a", provider.UUID)
}
