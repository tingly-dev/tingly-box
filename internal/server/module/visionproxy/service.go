// Package visionproxy is the single entry point for the vision proxy plugin,
// covering both the rule-level and scenario-level scopes. See
// .design/vision-proxy.md for the full design.
package visionproxy

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/processor"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Service wraps the vision proxy processor and resolves the effective vision
// service ({provider, model}) for a request before applying it in place.
type Service struct {
	Processor *processor.VisionProxyProcessor
}

// NewService builds a Service around the given processor. A nil processor
// makes Apply a no-op.
func NewService(p *processor.VisionProxyProcessor) *Service {
	return &Service{Processor: p}
}

// Apply runs the vision proxy plugin against typedRequest, covering both the
// rule-level and scenario-level scopes. It must run before service selection
// (after the rule is resolved). The effective service is chosen by Resolve —
// rule level wins over scenario level — and the processor runs at most once
// per request.
func (s *Service) Apply(ctx context.Context, cfg *config.Config, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
	if s == nil || s.Processor == nil || typedRequest == nil {
		return
	}
	svc := s.Resolve(cfg, scenarioType, rule)
	if svc == nil {
		return
	}
	_ = s.Processor.Process(&smartrouting.ProcessorContext{
		Ctx:      ctx,
		Request:  typedRequest,
		Services: []*loadbalance.Service{svc},
	})
}

// Resolve picks the effective vision service for this request. Rule level
// wins over scenario level when both are set — the more specific scope is
// taken to be the user's intent. Returns nil when neither scope configures a
// usable {provider, model}, in which case the proxy is skipped.
func (s *Service) Resolve(cfg *config.Config, scenarioType typ.RuleScenario, rule *typ.Rule) *loadbalance.Service {
	if rule != nil && rule.Flags.VisionProxyService != nil {
		if svc := buildVisionService(rule.Flags.VisionProxyService.Provider, rule.Flags.VisionProxyService.Model); svc != nil {
			return svc
		}
	}
	if cfg != nil {
		if scCfg := cfg.GetScenarioConfig(scenarioType); scCfg != nil {
			if svc := parseScenarioVisionService(scCfg.Extensions); svc != nil {
				return svc
			}
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
