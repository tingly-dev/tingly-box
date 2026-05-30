package server

import (
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// visionProxyAppliedKey marks (on the gin context) that the scenario-level
// vision proxy already ran for this request, so a second handler entry or a
// later code path does not describe the same images twice.
const visionProxyAppliedKey = "vision_proxy_applied"

// applyScenarioVisionProxy runs the scenario-level vision proxy plugin: when
// the scenario has a vision service configured (Extensions["vision_proxy_service"]),
// it reuses VisionProxyProcessor to replace image content blocks in
// typedRequest with text descriptions (latest message described via the
// configured vision upstream; historical images stripped with a marker).
//
// A configured service IS the enabled state — there is no separate flag.
//
// It must be called BEFORE service selection so that smart routing's
// proxy_vision op (which matches on RequestContext.HasImage) naturally
// becomes a no-op once images have been replaced — keeping the two paths
// from describing the same request twice.
//
// typedRequest is the typed request struct (e.g. *anthropic.BetaMessageNewParams,
// *anthropic.MessageNewParams, *openai.ChatCompletionNewParams). Unknown
// shapes are left untouched by the processor.
func (s *Server) applyScenarioVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, typedRequest any) {
	if s.visionProxyProcessor == nil || typedRequest == nil {
		return
	}
	if applied, _ := c.Get(visionProxyAppliedKey); applied == true {
		return
	}

	cfg := s.config.GetScenarioConfig(scenarioType)
	if cfg == nil {
		return
	}
	svc := parseVisionProxyService(cfg.Extensions)
	if svc == nil {
		return
	}

	c.Set(visionProxyAppliedKey, true)
	_ = s.visionProxyProcessor.Process(&smartrouting.ProcessorContext{
		Ctx:      c.Request.Context(),
		Request:  typedRequest,
		Services: []*loadbalance.Service{svc},
	})
}

// parseVisionProxyService reads the configured vision service (provider +
// model) from a scenario's Extensions map. The value is stored as a nested
// object under VisionProxyServiceKey; after JSON/YAML unmarshal it is a
// map[string]interface{}. Returns nil when absent or malformed (provider and
// model are both required), in which case the proxy is skipped.
func parseVisionProxyService(extensions map[string]interface{}) *loadbalance.Service {
	if extensions == nil {
		return nil
	}
	raw, ok := extensions[config.VisionProxyServiceKey]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	provider, _ := m["provider"].(string)
	model, _ := m["model"].(string)
	if provider == "" || model == "" {
		return nil
	}
	return &loadbalance.Service{
		Provider: provider,
		Model:    model,
		Active:   true,
		Weight:   1,
	}
}
