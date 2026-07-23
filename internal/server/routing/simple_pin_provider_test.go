package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestSelectService_PinProvider verifies that X-Tingly-Pin-Provider picks the
// named service from the rule's OWN services, overriding what the load
// balancer would otherwise choose — same mechanics as the probe pin, but
// scoped to services the rule already has configured.
func TestSelectService_PinProvider(t *testing.T) {
	svcA := testService("provider-a", "claude-sonnet", true)
	svcB := testService("provider-b", "claude-sonnet", true)
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
			"provider-b": testProvider("provider-b", "ProviderB", true),
		},
	}
	// Pipeline would normally return provider-b via load balancer.
	lb := &mockLoadBalancer{service: svcB}
	store := newMockAffinityStore()
	sel := NewServiceSelector(cfg, store, lb)
	simple := NewSimpleSelector(sel)

	rule := testRule("rule-1", "claude-sonnet", []*loadbalance.Service{svcA, svcB})
	c := ginCtxWithHeader(t, "X-Tingly-Pin-Provider", "provider-a")

	provider, svc, err := simple.SelectService(c, typ.ScenarioAnthropic, rule, nil)
	require.NoError(t, err)

	assert.Equal(t, "provider-a", provider.UUID)
	assert.Equal(t, "provider-a", svc.Provider)
}

// TestSelectService_PinProvider_RejectsProviderNotOnRule is the scoping
// guarantee that makes this header safe to expose to clients: it cannot
// reach a provider the rule wasn't already configured with.
func TestSelectService_PinProvider_RejectsProviderNotOnRule(t *testing.T) {
	svcA := testService("provider-a", "claude-sonnet", true)
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a":         testProvider("provider-a", "ProviderA", true),
			"unrelated-provider": testProvider("unrelated-provider", "Unrelated", true),
		},
	}
	simple := newSimpleSelector(cfg)
	rule := testRule("rule-1", "claude-sonnet", []*loadbalance.Service{svcA})
	c := ginCtxWithHeader(t, "X-Tingly-Pin-Provider", "unrelated-provider")

	_, _, err := simple.SelectService(c, typ.ScenarioAnthropic, rule, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an active service")
}

// TestSelectService_PinProvider_RejectsInactiveService confirms a service
// present on the rule but not active cannot be pinned to either.
func TestSelectService_PinProvider_RejectsInactiveService(t *testing.T) {
	svcA := testService("provider-a", "claude-sonnet", false) // inactive
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}
	simple := newSimpleSelector(cfg)
	rule := testRule("rule-1", "claude-sonnet", []*loadbalance.Service{svcA})
	c := ginCtxWithHeader(t, "X-Tingly-Pin-Provider", "provider-a")

	_, _, err := simple.SelectService(c, typ.ScenarioAnthropic, rule, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an active service")
}

// TestSelectService_PinProvider_DisabledProvider errors when the pinned
// provider is itself disabled, even though it's a configured service.
func TestSelectService_PinProvider_DisabledProvider(t *testing.T) {
	svcA := testService("provider-a", "claude-sonnet", true)
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", false), // disabled
		},
	}
	simple := newSimpleSelector(cfg)
	rule := testRule("rule-1", "claude-sonnet", []*loadbalance.Service{svcA})
	c := ginCtxWithHeader(t, "X-Tingly-Pin-Provider", "provider-a")

	_, _, err := simple.SelectService(c, typ.ScenarioAnthropic, rule, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

// TestSelectService_NoPinHeader_FallsThrough confirms that without the pin
// header the normal pipeline still runs unaffected.
func TestSelectService_NoPinHeader_FallsThrough(t *testing.T) {
	svc := testService("provider-a", "claude-sonnet", true)
	cfg := &mockConfig{
		providers: map[string]*typ.Provider{
			"provider-a": testProvider("provider-a", "ProviderA", true),
		},
	}
	lb := &mockLoadBalancer{service: svc}
	store := newMockAffinityStore()
	sel := NewServiceSelector(cfg, store, lb)
	simple := NewSimpleSelector(sel)

	rule := testRule("rule-1", "claude-sonnet", []*loadbalance.Service{svc})
	c := ginCtxWithHeader(t, "", "")

	provider, _, err := simple.SelectService(c, typ.ScenarioAnthropic, rule, nil)
	require.NoError(t, err)
	assert.Equal(t, "provider-a", provider.UUID)
}
