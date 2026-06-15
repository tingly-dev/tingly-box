package server

import (
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/processor"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GinKeyVisionDescriptions is the gin.Context key under which
// applyVisionProxy stashes the snapshot of descriptions emitted during a
// request. The response-side injectors (protocol hook + non-stream
// middleware) read this slice once per request. Each entry is already
// wrapped in <image-description>…</image-description> by the processor;
// consumers splice them in verbatim.
const GinKeyVisionDescriptions = "vision_proxy.descriptions"

// applyVisionProxy is the single entry point for the vision proxy plugin,
// covering both the rule-level and scenario-level scopes. It must run before
// service selection (after the rule is resolved). The effective service is
// chosen by resolveVisionService — rule level wins over scenario level — and
// the processor runs at most once per request. See .design/vision-proxy-rule.md.
func (s *Server) applyVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
	if s.visionProxyProcessor == nil || typedRequest == nil {
		return
	}
	svc := s.resolveVisionService(scenarioType, rule)
	if svc == nil {
		return
	}
	collector := &processor.DescriptionCollector{}
	_ = s.visionProxyProcessor.Process(&smartrouting.ProcessorContext{
		Ctx:      c.Request.Context(),
		Request:  typedRequest,
		Services: []*loadbalance.Service{svc},
		Extras: map[string]any{
			processor.ExtrasKeyVisionDescriptions: collector,
		},
	})
	// Stash collected descriptions on the gin context so the response-side
	// injector (protocol stream hook + non-stream middleware) can prepend
	// them to the model's reply. Empty snapshot is fine — downstream checks
	// the slice length and short-circuits.
	if descs := collector.Snapshot(); len(descs) > 0 {
		c.Set(GinKeyVisionDescriptions, descs)
	}
}

// resolveVisionService picks the effective vision service for this request.
// Rule level wins over scenario level when both are set — the more specific
// scope is taken to be the user's intent. Returns nil when neither scope
// configures a usable {provider, model}, in which case the proxy is skipped.
func (s *Server) resolveVisionService(scenarioType typ.RuleScenario, rule *typ.Rule) *loadbalance.Service {
	if rule != nil && rule.Flags.VisionProxyService != nil {
		if svc := buildVisionService(rule.Flags.VisionProxyService.Provider, rule.Flags.VisionProxyService.Model); svc != nil {
			return svc
		}
	}
	if cfg := s.config.GetScenarioConfig(scenarioType); cfg != nil {
		if svc := parseScenarioVisionService(cfg.Extensions); svc != nil {
			return svc
		}
	}
	return nil
}

// buildVisionService wraps a provider+model into a loadbalance.Service with
// the defaults the processor expects, or nil when either part is empty.
func buildVisionService(provider, model string) *loadbalance.Service {
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

// parseScenarioVisionService reads the scenario-level vision service from a
// scenario's Extensions map (stored as a nested object under
// config.ExtensionVisionProxyService; a map[string]interface{} after JSON/YAML unmarshal).
// Returns nil when absent or malformed.
func parseScenarioVisionService(extensions map[string]interface{}) *loadbalance.Service {
	if extensions == nil {
		return nil
	}
	raw, ok := extensions[config.ExtensionVisionProxyService]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	provider, _ := m["provider"].(string)
	model, _ := m["model"].(string)
	return buildVisionService(provider, model)
}
