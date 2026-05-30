package server

import (
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const visionProxyAppliedKey = "vision_proxy_applied"

// applyScenarioVisionProxy is the scenario-level entry point for the vision
// proxy plugin. It must run BEFORE service selection so smart routing's
// proxy_vision op (matched on HasImage) becomes a no-op once images have
// been replaced. See .design/vision-proxy-scenario.md.
func (s *Server) applyScenarioVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, typedRequest any) {
	if s.visionProxyProcessor == nil || typedRequest == nil {
		return
	}
	if c.GetBool(visionProxyAppliedKey) {
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

// parseVisionProxyService returns nil when the configured service is
// absent or malformed (missing provider or model) — both states mean
// "vision proxy off" to the caller.
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
