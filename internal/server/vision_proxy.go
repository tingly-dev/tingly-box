package server

import (
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/outputinjector"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/output"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// VisionDescriptionsKey is the gin.Context key under which applyVisionProxy
// stashes the per-image raw descriptions collected during request-side
// processing. Handlers pick this up and feed it to outputinjector so the
// descriptions become visible on the response side too (vision-proxy.md §4).
const VisionDescriptionsKey = "vision_descriptions"

// applyVisionProxy is the single entry point for the vision proxy plugin,
// covering both the rule-level and scenario-level scopes. It must run before
// service selection (after the rule is resolved). The effective service is
// chosen by resolveVisionService — rule level wins over scenario level — and
// the processor runs at most once per request. See .design/vision-proxy.md.
//
// Successfully described images leave their raw description on
// gin.Context[VisionDescriptionsKey] so the handler can attach a
// VisionTextPrefix injector to the response stream / non-stream response.
// No descriptions stash (key missing) means "vision proxy not active for
// this request" — the response side reads it as no-op.
func (s *Server) applyVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
	if s.visionProxyProcessor == nil || typedRequest == nil {
		return
	}
	svc := s.resolveVisionService(scenarioType, rule)
	if svc == nil {
		return
	}
	pctx := &smartrouting.ProcessorContext{
		Ctx:      c.Request.Context(),
		Request:  typedRequest,
		Services: []*loadbalance.Service{svc},
	}
	_ = s.visionProxyProcessor.Process(pctx)
	if len(pctx.Descriptions) > 0 {
		c.Set(VisionDescriptionsKey, pctx.Descriptions)
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

// newVisionOutputInjector returns the output injector for this request, or
// nil when vision proxy did not collect any descriptions. Callers use the
// nil-safe outputinjector helpers, so an unconditional call is fine.
func newVisionOutputInjector(c *gin.Context) outputinjector.OutputInjector {
	v, ok := c.Get(VisionDescriptionsKey)
	if !ok {
		return nil
	}
	descs, ok := v.([]string)
	if !ok || len(descs) == 0 {
		return nil
	}
	return output.NewVisionTextPrefix(descs)
}

// attachVisionStreamInjector wires vision descriptions (if any) into the
// HandleContext so they are prepended into the first text-bearing stream
// event reaching the client. No-op when vision proxy is inactive for this
// request — safe to call at every HandleContext construction site.
func attachVisionStreamInjector(c *gin.Context, hc *protocol.HandleContext) {
	outputinjector.AttachToHandleContext(hc, newVisionOutputInjector(c))
}

// prependVisionToNonStreamResponse mutates a fully-formed response in place
// to prepend the vision-proxy descriptions to the first text content slot.
// No-op when vision proxy is inactive for this request or the response
// carries no text-bearing slot. Returns true if a mutation happened (mostly
// useful in tests).
func prependVisionToNonStreamResponse(c *gin.Context, resp any) bool {
	return outputinjector.PrependToNonStreamResponse(newVisionOutputInjector(c), resp)
}

// sendNonStreamModelResponse runs vision (and any future non-stream
// injectors) on resp in place, then writes c.JSON(200, resp). Use this for
// every model-response c.JSON(http.StatusOK, ...) so output injection works
// uniformly across all dispatch paths.
func sendNonStreamModelResponse(c *gin.Context, resp any) {
	prependVisionToNonStreamResponse(c, resp)
	c.JSON(200, resp)
}

// parseScenarioVisionService reads the scenario-level vision service from a
// scenario's Extensions map (stored as a nested object under
// VisionProxyServiceKey; a map[string]interface{} after JSON/YAML unmarshal).
// Returns nil when absent or malformed.
func parseScenarioVisionService(extensions map[string]interface{}) *loadbalance.Service {
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
	return buildVisionService(provider, model)
}
