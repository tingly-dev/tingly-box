package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAlternateOpenAIProtocol(t *testing.T) {
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIChat); got != protocol.TypeOpenAIResponses {
		t.Errorf("alternate of chat = %v, want responses", got)
	}
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIResponses); got != protocol.TypeOpenAIChat {
		t.Errorf("alternate of responses = %v, want chat", got)
	}
}

func TestIncomingToTarget(t *testing.T) {
	if got := incomingToTarget(IncomingAPIChat); got != protocol.TypeOpenAIChat {
		t.Errorf("incoming chat → %v, want chat", got)
	}
	if got := incomingToTarget(IncomingAPIResponses); got != protocol.TypeOpenAIResponses {
		t.Errorf("incoming responses → %v, want responses", got)
	}
}

func TestScenarioPreferredProtocol(t *testing.T) {
	tests := []struct {
		name     string
		scenario typ.RuleScenario
		incoming IncomingAPIType
		want     protocol.APIType
	}{
		{"codex prefers responses even on chat ingress", typ.ScenarioCodex, IncomingAPIChat, protocol.TypeOpenAIResponses},
		{"codex prefers responses on responses ingress", typ.ScenarioCodex, IncomingAPIResponses, protocol.TypeOpenAIResponses},
		{"codex profile suffix normalized", typ.RuleScenario("codex:p1"), IncomingAPIChat, protocol.TypeOpenAIResponses},
		{"openai mirrors chat ingress", typ.ScenarioOpenAI, IncomingAPIChat, protocol.TypeOpenAIChat},
		{"openai mirrors responses ingress", typ.ScenarioOpenAI, IncomingAPIResponses, protocol.TypeOpenAIResponses},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scenarioPreferredProtocol(tt.scenario, tt.incoming); got != tt.want {
				t.Errorf("scenarioPreferredProtocol(%q, %q) = %v, want %v", tt.scenario, tt.incoming, got, tt.want)
			}
		})
	}
}

// TestDispatchWithAutoFallback_CacheAttributedToServingProvider covers the
// multi-service failover interaction: when the initial provider fails and a
// fallback provider serves the request, the protocol cache entry must be
// written for the serving provider, not the initial one — otherwise the
// initial provider gets pinned to a protocol it never confirmed.
func TestDispatchWithAutoFallback_CacheAttributedToServingProvider(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}

	initial := &typ.Provider{UUID: "prov-initial"}
	serving := &typ.Provider{UUID: "prov-serving"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	// Simulate a dispatch where failover moved past the initial provider:
	// the gate commits (success) and the serving identity differs from initial.
	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string, bool) {
		gate.WriteHeader(200)
		gate.CommitFirstChunk()
		return serving, "served-model", true
	}

	s.dispatchWithAutoFallback(c, initial, "req-model", protocol.TypeOpenAIChat, dispatch)

	if _, ok := s.endpointCache.Get(initial.UUID, "req-model"); ok {
		t.Error("cache must not contain an entry for the initial provider")
	}
	got, ok := s.endpointCache.Get(serving.UUID, "served-model")
	if !ok {
		t.Fatal("cache must contain an entry for the serving provider")
	}
	if got != protocol.TypeOpenAIChat {
		t.Errorf("cached protocol = %v, want chat", got)
	}
}

// TestDispatchWithAutoFallback_NoCacheOnTransformFailure ensures a failed
// transform (ok=false, served=nil) never writes a cache entry even if the
// gate state looks successful.
func TestDispatchWithAutoFallback_NoCacheOnTransformFailure(t *testing.T) {
	s := &Server{endpointCache: NewEndpointCache(0)}
	initial := &typ.Provider{UUID: "prov-initial"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	dispatch := func(target protocol.APIType, gate *firstChunkGate) (*typ.Provider, string, bool) {
		gate.WriteHeader(200) // transform error path writes a JSON error; simulate benign status
		return nil, "", false
	}

	s.dispatchWithAutoFallback(c, initial, "m", protocol.TypeOpenAIChat, dispatch)

	if _, ok := s.endpointCache.Get(initial.UUID, "m"); ok {
		t.Error("cache must stay empty when the transform failed")
	}
}
