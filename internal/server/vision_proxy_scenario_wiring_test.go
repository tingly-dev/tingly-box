package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/processor"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const wiringPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

// stubVisionClient returns a canned description; implements the processor's
// (unexported) visionClient interface structurally.
type stubVisionClient struct{ desc string }

func (s stubVisionClient) Describe(_ context.Context, _ *loadbalance.Service, _, _, _ string) (string, error) {
	return s.desc, nil
}

// stubResolver implements the processor's (unexported) providerResolver so the
// configured service is treated as usable.
type stubResolver struct{}

func (stubResolver) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	return &typ.Provider{UUID: uuid, Name: "stub"}, nil
}

func newTestGinCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/tingly/claude_code/messages", nil)
	return c
}

func betaReqWithImage() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("downstream-text-model"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "what is this?"}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      wiringPNG,
						MediaType: anthropic.BetaBase64ImageSourceMediaType("image/png"),
					}),
				},
			},
		},
	}
}

func serverWithVisionScenario(scenario typ.RuleScenario, ext map[string]interface{}, desc string) *Server {
	proc := &processor.VisionProxyProcessor{
		Client:   stubVisionClient{desc: desc},
		Resolver: stubResolver{},
	}
	return &Server{
		config: &config.Config{
			Scenarios: []typ.ScenarioConfig{
				{Scenario: scenario, Extensions: ext},
			},
		},
		visionProxyProcessor: proc,
	}
}

// firstImagePresent reports whether any content block in the first message is
// still an image (i.e. the proxy did NOT replace it).
func firstImagePresent(req *anthropic.BetaMessageNewParams) bool {
	for _, b := range req.Messages[0].Content {
		if b.OfImage != nil {
			return true
		}
	}
	return false
}

// TestApplyScenarioVisionProxy_ReplacesImage is the end-to-end wiring check:
// a scenario with vision_proxy_service configured must cause applyScenarioVisionProxy
// to run the processor and replace the image block with the stub description.
func TestApplyScenarioVisionProxy_ReplacesImage(t *testing.T) {
	ext := map[string]interface{}{
		config.VisionProxyServiceKey: map[string]interface{}{
			"provider": "prov-uuid",
			"model":    "vision-model",
		},
	}
	s := serverWithVisionScenario("claude_code", ext, "a red dot")

	req := betaReqWithImage()
	c := newTestGinCtx()
	s.applyScenarioVisionProxy(c, "claude_code", req)

	if firstImagePresent(req) {
		t.Fatal("image block was NOT replaced — vision proxy did not take effect")
	}
	// The described text should be present in some text block.
	var joined strings.Builder
	for _, b := range req.Messages[0].Content {
		if b.OfText != nil {
			joined.WriteString(b.OfText.Text)
		}
	}
	if !strings.Contains(joined.String(), "a red dot") {
		t.Fatalf("expected description in text blocks, got: %q", joined.String())
	}
}

// TestApplyScenarioVisionProxy_NoServiceConfigured confirms the no-op path:
// scenario exists but no vision_proxy_service -> image left intact.
func TestApplyScenarioVisionProxy_NoServiceConfigured(t *testing.T) {
	s := serverWithVisionScenario("claude_code", map[string]interface{}{}, "x")
	req := betaReqWithImage()
	c := newTestGinCtx()
	s.applyScenarioVisionProxy(c, "claude_code", req)
	if !firstImagePresent(req) {
		t.Fatal("image was replaced even though no vision service is configured")
	}
}

// TestApplyScenarioVisionProxy_ScenarioMismatch documents the exact-match
// behavior: config stored under "claude_code" is NOT found when the request's
// scenario is "claude_code:p1" (profiled), so the proxy silently no-ops.
func TestApplyScenarioVisionProxy_ScenarioMismatch(t *testing.T) {
	ext := map[string]interface{}{
		config.VisionProxyServiceKey: map[string]interface{}{
			"provider": "prov-uuid",
			"model":    "vision-model",
		},
	}
	s := serverWithVisionScenario("claude_code", ext, "a red dot")
	req := betaReqWithImage()
	c := newTestGinCtx()
	s.applyScenarioVisionProxy(c, "claude_code:p1", req) // profiled scenario
	if !firstImagePresent(req) {
		t.Fatal("unexpected: profiled scenario matched base config")
	}
}
