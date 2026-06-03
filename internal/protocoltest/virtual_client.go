package protocoltest

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/vmodel/vmodeltest"
)

// ParsedResponse is the result of a request sent to a virtual server.
type ParsedResponse = vmodeltest.ParsedResponse

// VirtualClient sends provider-native HTTP requests for testing.
// It embeds vmodeltest.Client for model-parameterized methods and adds
// scenario-based methods that auto-register on a bound VirtualServer.
type VirtualClient struct {
	*vmodeltest.Client
	server *VirtualServer // optional; set via WithServer
}

// NewVirtualClient creates a client pointing at baseURL.
func NewVirtualClient(baseURL string) *VirtualClient {
	return &VirtualClient{
		Client: vmodeltest.NewClient(baseURL),
	}
}

// WithServer binds the client to a VirtualServer.
func (vc *VirtualClient) WithServer(vs *VirtualServer) *VirtualClient {
	vc.server = vs
	return vc
}

// Client returns a VirtualClient pre-pointed at this VirtualServer and bound to it.
func (vs *VirtualServer) Client() *VirtualClient {
	return NewVirtualClient(vs.URL()).WithServer(vs)
}

// ─── Scenario-based send helpers ─────────────────────────────────────────────

// SendOpenAIChat sends a request to the OpenAI Chat Completions endpoint.
func (vc *VirtualClient) SendOpenAIChat(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"model":    "gpt-4o",
		"messages": []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":   streaming,
	}
	return vc.DoRequest(t, "POST", vc.BaseURL+"/v1/chat/completions", body, streaming, protocol.APIStyleOpenAI)
}

// SendOpenAIResponses sends a request to the OpenAI Responses API endpoint.
func (vc *VirtualClient) SendOpenAIResponses(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"model":  "gpt-4o",
		"input":  "What is the capital of France?",
		"stream": streaming,
	}
	return vc.DoRequest(t, "POST", vc.BaseURL+"/v1/responses", body, streaming, protocol.APIStyleOpenAI)
}

// SendAnthropicV1 sends a request to the Anthropic Messages endpoint.
func (vc *VirtualClient) SendAnthropicV1(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages":   []map[string]string{{"role": "user", "content": "What is the capital of France?"}},
		"stream":     streaming,
	}
	return vc.DoRequest(t, "POST", vc.BaseURL+"/v1/messages", body, streaming, protocol.APIStyleAnthropic)
}

// SendGoogle sends a request to the Google GenerateContent endpoint.
func (vc *VirtualClient) SendGoogle(t *testing.T, s Scenario, streaming bool) *ParsedResponse {
	t.Helper()
	vc.maybeRegister(s)
	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"role": "user", "parts": []map[string]string{{"text": "What is the capital of France?"}}},
		},
	}
	suffix := "generateContent"
	if streaming {
		suffix = "streamGenerateContent"
	}
	return vc.DoRequest(t, "POST", vc.BaseURL+"/v1beta/models/gemini-2.0-flash/"+suffix, body, streaming, protocol.APIStyleGoogle)
}

func (vc *VirtualClient) maybeRegister(s Scenario) {
	if vc.server != nil {
		vc.server.RegisterScenario(s)
	}
}
