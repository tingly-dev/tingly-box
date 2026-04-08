package routing

import (
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
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
		SessionID:             sessionID,
		MatchedSmartRuleIndex: -1,
	}
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
	service           *loadbalance.Service
	err               error
	updateIndexCalled bool
}

func (m *mockLoadBalancer) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	return m.service, m.err
}

func (m *mockLoadBalancer) UpdateServiceIndex(rule *typ.Rule, service *loadbalance.Service) {
	m.updateIndexCalled = true
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

func (m *mockConfig) SaveCurrentServiceID(ruleUUID string, serviceID string) error {
	return nil
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
	require.Equal(t, "session-1", ctx.SessionID)

	store := newMockAffinityStore()
	require.NotNil(t, store)
	store.Set("rule-1", "s1", testAffinityEntry(svc))
	entry, ok := store.Get("rule-1", "s1")
	require.True(t, ok)
	require.Equal(t, "m1", entry.Service.Model)
}
