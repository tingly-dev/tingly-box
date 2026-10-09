package protocolserver

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestResolveAttemptTarget pins the provider-style → target table the four
// entry points used to each carry a copy of.
func TestResolveAttemptTarget(t *testing.T) {
	anthropicStyle := &typ.Provider{Name: "a", APIStyle: protocol.APIStyleAnthropic}
	googleStyle := &typ.Provider{Name: "g", APIStyle: protocol.APIStyleGoogle}
	openAIBoth := &typ.Provider{Name: "o", APIStyle: protocol.APIStyleOpenAI, OpenAIEndpointMode: ai.EndpointModeBoth}
	unknownStyle := &typ.Provider{Name: "u", APIStyle: "mystery"}

	tests := []struct {
		name     string
		provider *typ.Provider
		source   protocol.APIType
		want     protocol.APIType
		wantErr  bool
	}{
		{"anthropic v1 on anthropic", anthropicStyle, protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, false},
		{"anthropic beta on anthropic", anthropicStyle, protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, false},
		{"chat on anthropic", anthropicStyle, protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, false},
		{"responses on anthropic", anthropicStyle, protocol.TypeOpenAIResponses, protocol.TypeAnthropicBeta, false},

		{"anthropic on google", googleStyle, protocol.TypeAnthropicBeta, protocol.TypeGoogle, false},
		{"chat on google", googleStyle, protocol.TypeOpenAIChat, protocol.TypeGoogle, false},
		{"responses on google", googleStyle, protocol.TypeOpenAIResponses, "", true},

		// Endpoint routing: Chat clients prefer Chat, everyone else Responses.
		{"chat on openai", openAIBoth, protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, false},
		{"responses on openai", openAIBoth, protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, false},
		{"anthropic on openai", openAIBoth, protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, false},

		{"anthropic on unknown style", unknownStyle, protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, false},
		{"chat on unknown style", unknownStyle, protocol.TypeOpenAIChat, "", true},
	}
	ph := &ProtocolHandler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/", nil)
			got, err := ph.resolveAttemptTarget(c, &typ.Rule{}, tt.provider, "m", tt.source)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("target = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveAttemptStyle pins the anthropic_endpoint_override style
// resolution: the inbound client's own style by default, the Anthropic style
// when the rule forces it and the provider supports one, and the client style
// (with just a warning) when it does not.
func TestResolveAttemptStyle(t *testing.T) {
	dualOpenAI := &typ.Provider{Name: "dual", APIStyle: protocol.APIStyleOpenAI, APIBase: "https://o.example", APIBaseAnthropic: "https://a.example"}
	dualOAuthBound := &typ.Provider{Name: "dual-oauth", AuthType: ai.AuthTypeOAuth, APIStyle: protocol.APIStyleOpenAI, APIBase: "https://o.example", APIBaseAnthropic: "https://a.example"}
	nativeAnthropic := &typ.Provider{Name: "native", APIStyle: protocol.APIStyleAnthropic, APIBase: "https://a.example"}
	chatOnly := &typ.Provider{Name: "chat", APIStyle: protocol.APIStyleOpenAI, APIBase: "https://o.example"}
	googleStyle := &typ.Provider{Name: "google", APIStyle: protocol.APIStyleGoogle, APIBase: "https://g.example"}

	tests := []struct {
		name     string
		flags    typ.RuleFlags
		provider *typ.Provider
		source   protocol.APIType
		want     ai.APIStyle
	}{
		{"no flag follows client style", typ.RuleFlags{}, dualOpenAI, protocol.TypeOpenAIChat, ai.APIStyleOpenAI},
		{"auto flag follows client style", typ.RuleFlags{AnthropicEndpointOverride: "auto"}, dualOpenAI, protocol.TypeOpenAIChat, ai.APIStyleOpenAI},
		{"unknown flag value follows client style", typ.RuleFlags{AnthropicEndpointOverride: "bogus"}, dualOpenAI, protocol.TypeOpenAIChat, ai.APIStyleOpenAI},
		{"force on dual provider picks anthropic", typ.RuleFlags{AnthropicEndpointOverride: "anthropic"}, dualOpenAI, protocol.TypeOpenAIChat, ai.APIStyleAnthropic},
		// OAuth bearers are endpoint-scoped: the dual URL is ignored unless the
		// issuer allows dual use, so the force cannot select it.
		{"force on non-dual-eligible oauth provider falls back", typ.RuleFlags{AnthropicEndpointOverride: "anthropic"}, dualOAuthBound, protocol.TypeOpenAIChat, ai.APIStyleOpenAI},
		{"force on native anthropic provider stays anthropic", typ.RuleFlags{AnthropicEndpointOverride: "anthropic"}, nativeAnthropic, protocol.TypeAnthropicBeta, ai.APIStyleAnthropic},
		{"force on openai-only provider falls back", typ.RuleFlags{AnthropicEndpointOverride: "anthropic"}, chatOnly, protocol.TypeOpenAIChat, ai.APIStyleOpenAI},
		{"force on google provider falls back", typ.RuleFlags{AnthropicEndpointOverride: "anthropic"}, googleStyle, protocol.TypeOpenAIChat, ai.APIStyleOpenAI},
	}
	ph := &ProtocolHandler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/", nil)
			got := ph.resolveAttemptStyle(c, &typ.Rule{Flags: tt.flags}, tt.provider, tt.source)
			if got != tt.want {
				t.Errorf("style = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAttemptPlanForcedAnthropic pins the end-to-end style+target pair for a
// forced rule: a dual-URL OpenAI-style provider serves an OpenAI Chat client
// from its Anthropic endpoint, with the target protocol following the
// resolved provider style.
func TestAttemptPlanForcedAnthropic(t *testing.T) {
	dualOpenAI := &typ.Provider{Name: "dual", APIStyle: protocol.APIStyleOpenAI, APIBase: "https://o.example", APIBaseAnthropic: "https://a.example"}
	ph := &ProtocolHandler{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)

	resolved := dualOpenAI.ResolveStyle(ph.resolveAttemptStyle(c, &typ.Rule{Flags: typ.RuleFlags{AnthropicEndpointOverride: "anthropic"}}, dualOpenAI, protocol.TypeOpenAIChat))
	if resolved.APIBase != "https://a.example" || resolved.APIStyle != protocol.APIStyleAnthropic {
		t.Fatalf("resolved = %s %s, want https://a.example anthropic", resolved.APIBase, resolved.APIStyle)
	}
	target, err := ph.resolveAttemptTarget(c, &typ.Rule{}, resolved, "m", protocol.TypeOpenAIChat)
	if err != nil {
		t.Fatalf("resolveAttemptTarget: %v", err)
	}
	if target != protocol.TypeAnthropicBeta {
		t.Errorf("target = %q, want anthropic beta", target)
	}
}

func TestAttemptPlanServedByStage(t *testing.T) {
	tests := []struct {
		source, target protocol.APIType
		want           bool
	}{
		{protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, true},
		{protocol.TypeAnthropicBeta, protocol.TypeOpenAIResponses, true},
		{protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, true},
		{protocol.TypeOpenAIResponses, protocol.TypeAnthropicBeta, true},
		{protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, false},
		{protocol.TypeAnthropicV1, protocol.TypeGoogle, false},
		{protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses, false},
		{protocol.TypeOpenAIResponses, protocol.TypeOpenAIChat, false},
		{protocol.TypeOpenAIChat, protocol.TypeGoogle, false},
	}
	for _, tt := range tests {
		plan := &attemptPlan{Source: tt.source, Target: tt.target}
		if got := plan.servedByStage(); got != tt.want {
			t.Errorf("%s -> %s: servedByStage = %v, want %v", tt.source, tt.target, got, tt.want)
		}
	}
}
