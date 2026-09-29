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
