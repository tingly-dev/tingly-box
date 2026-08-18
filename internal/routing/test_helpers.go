package routing

import (
	"context"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// --- Fixtures ---

func testService(provider, model string, active bool) *loadbalance.Service {
	return &loadbalance.Service{
		Provider: provider,
		Model:    model,
		Weight:   1,
		Active:   active,
	}
}

// ServiceForTest and OpenAIRequestForTest are exported wrappers around this
// package's own fixtures, for external test packages (e.g. routing_test's
// benchmarks, which must live outside package routing to import
// protocolserver without an import cycle) that need identical shapes without
// reimplementing them.
func ServiceForTest(provider, model string, active bool) *loadbalance.Service {
	return testService(provider, model, active)
}

func OpenAIRequestForTest(model string) *openai.ChatCompletionNewParams {
	return testOpenAIRequest(model)
}

func testProvider(uuid, name string, enabled bool) *typ.Provider {
	return &typ.Provider{
		UUID:    uuid,
		Name:    name,
		Enabled: enabled,
	}
}

func testRule(uuid, model string, services []*loadbalance.Service) *typ.Rule {
	return &typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: model,
		Services:     services,
		Active:       true,
	}
}

func testSmartRule(uuid, model string, services []*loadbalance.Service, ops ...smartrouting.SmartOp) *typ.Rule {
	r := testRule(uuid, model, services)
	r.SmartEnabled = true
	r.SmartRouting = []smartrouting.SmartRouting{
		{
			Description: "test-rule",
			Ops:         ops,
			Services:    services,
		},
	}
	return r
}

func testContext(rule *typ.Rule, sessionID string) *SelectionContext {
	return &SelectionContext{
		Rule:                  rule,
		SessionID:             typ.SessionID{Source: typ.SessionSourceHeader, Value: sessionID},
		MatchedSmartRuleIndex: -1,
	}
}

// testSessionKey returns the affinity-store key produced by the production
// code for a header-sourced session value. The selector uses
// ctx.SessionID.String() (a JSON encoding), so test fixtures must match.
func testSessionKey(sessionID string) string {
	return typ.SessionID{Source: typ.SessionSourceHeader, Value: sessionID}.String()
}

// testOpenAIRequest creates a minimal OpenAI request for testing.
func testOpenAIRequest(model string) *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hello"),
		},
	}
}

func testModelContainsOp(value string) smartrouting.SmartOp {
	return smartrouting.SmartOp{
		Position:  smartrouting.PositionModel,
		Operation: smartrouting.OpModelContains,
		Value:     value,
	}
}

// --- Mocks ---

// mockLoadBalancer implements LoadBalancer for testing.
type mockLoadBalancer struct {
	service *loadbalance.Service
	err     error
}

func (m *mockLoadBalancer) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	return m.service, m.err
}

// mockAffinityStore implements AffinityStore for testing.
type mockAffinityStore struct {
	entries map[string]*AffinityEntry // key: "ruleUUID:sessionID"
	sets    []setCall
}

type setCall struct {
	ruleUUID, sessionID string
}

func newMockAffinityStore() *mockAffinityStore {
	return &mockAffinityStore{
		entries: make(map[string]*AffinityEntry),
	}
}

func (m *mockAffinityStore) Get(ruleUUID, sessionID string) (*AffinityEntry, bool) {
	entry, ok := m.entries[ruleUUID+":"+sessionID]
	return entry, ok
}

func (m *mockAffinityStore) Set(ruleUUID, sessionID string, entry *AffinityEntry) {
	m.entries[ruleUUID+":"+sessionID] = entry
	m.sets = append(m.sets, setCall{ruleUUID: ruleUUID, sessionID: sessionID})
}

func (m *mockAffinityStore) CountByService(serviceID string) int {
	cutoff := time.Now().Add(-30 * time.Minute)
	count := 0
	for _, entry := range m.entries {
		if entry.LockedAt.After(cutoff) && entry.Service != nil &&
			entry.Service.ServiceID() == serviceID {
			count++
		}
	}
	return count
}

// mockQuotaProvider implements QuotaProvider for testing service_quota.
// usage maps providerUUID -> cached usage; a missing entry simulates "no
// data yet" (GetQuotaNoCache's ErrUsageNotFound path), which the stage
// treats as unknown and excludes from aggregation.
type mockQuotaProvider struct {
	usage map[string]*quota.ProviderUsage
}

func newMockQuotaProvider() *mockQuotaProvider {
	return &mockQuotaProvider{usage: make(map[string]*quota.ProviderUsage)}
}

// setPct registers a single countable, standard-quota (Kind=limit) window at
// pct% used for providerUUID — mirrors how real fetchers (e.g. Anthropic's
// 5h/7d) tag their periodic windows so service_quota picks it up.
func (m *mockQuotaProvider) setPct(providerUUID string, pct float64) {
	m.usage[providerUUID] = &quota.ProviderUsage{
		ProviderUUID: providerUUID,
		Windows:      []*quota.UsageWindow{{Kind: quota.WindowKindLimit, Limit: 100, UsedPercent: pct}},
	}
}

// setResourcePct registers a single countable but resource-kind (balance,
// e.g. OpenRouter's key limit) window — used to prove service_quota ignores
// it even though ai/quota's general Pct() would happily count it.
func (m *mockQuotaProvider) setResourcePct(providerUUID string, pct float64) {
	m.usage[providerUUID] = &quota.ProviderUsage{
		ProviderUUID: providerUUID,
		Windows:      []*quota.UsageWindow{{Kind: quota.WindowKindResource, Limit: 100, UsedPercent: pct}},
	}
}

func (m *mockQuotaProvider) GetQuotaNoCache(_ context.Context, providerUUID string) (*quota.ProviderUsage, error) {
	usage, ok := m.usage[providerUUID]
	if !ok {
		return nil, quota.ErrUsageNotFound
	}
	return usage, nil
}

// mockConfig implements ProviderResolver for ServiceSelector tests.
type mockConfig struct {
	providers map[string]*typ.Provider
}

func (m *mockConfig) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	if p, ok := m.providers[uuid]; ok {
		return p, nil
	}
	return nil, nil
}

func (m *mockConfig) GetEffectiveAffinity(rule *typ.Rule) time.Duration {
	return time.Duration(rule.Flags.SessionAffinity) * time.Second
}

// testAffinityEntry creates a test affinity entry.
func testAffinityEntry(svc *loadbalance.Service) *AffinityEntry {
	return &AffinityEntry{
		Service:   svc,
		LockedAt:  time.Now(),
		ExpiresAt: time.Now().Add(2 * time.Hour),
	}
}

func TestFixtures_helpers(t *testing.T) {
	// Verify test helpers produce valid objects
	svc := testService("p1", "m1", true)
	require.NotNil(t, svc)
	require.Equal(t, "p1", svc.Provider)
	require.Equal(t, "m1", svc.Model)
	require.True(t, svc.Active)

	p := testProvider("p1", "Provider1", true)
	require.NotNil(t, p)
	require.Equal(t, "p1", p.UUID)

	rule := testRule("rule-1", "gpt-4", []*loadbalance.Service{svc})
	require.NotNil(t, rule)
	require.Equal(t, "gpt-4", rule.RequestModel)
	require.Len(t, rule.Services, 1)

	ctx := testContext(rule, "session-1")
	require.NotNil(t, ctx)
	require.Equal(t, "session-1", ctx.SessionID.Value)

	store := newMockAffinityStore()
	require.NotNil(t, store)
	store.Set("rule-1", "s1", testAffinityEntry(svc))
	entry, ok := store.Get("rule-1", "s1")
	require.True(t, ok)
	require.Equal(t, "m1", entry.Service.Model)
}
