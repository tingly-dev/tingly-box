package server

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// tryResolveProbeOverride inspects the request for probe-direct override
// headers. When both are present it returns a fully resolved
// provider/rule/service triple that callers can substitute for the normal
// determineRule + routingSelector.SelectService pair.
//
// ok=false with err=nil means "no override headers present, proceed
// normally". ok=true with err!=nil means the override was attempted but
// invalid (caller must return the error to the client).
func (s *Server) tryResolveProbeOverride(c *gin.Context, scenario typ.RuleScenario, requestModel string) (provider *typ.Provider, rule *typ.Rule, service *loadbalance.Service, ok bool, err error) {
	probeProviderUUID := c.GetHeader(probe.HeaderProbeProvider)
	if probeProviderUUID == "" {
		return nil, nil, nil, false, nil
	}

	probeModel := c.GetHeader(probe.HeaderProbeModel)
	if probeModel == "" {
		return nil, nil, nil, true, fmt.Errorf("%s header is required when %s is set", probe.HeaderProbeModel, probe.HeaderProbeProvider)
	}

	p, err := s.config.GetProviderByUUID(probeProviderUUID)
	if err != nil || p == nil {
		return nil, nil, nil, true, fmt.Errorf("probe provider not found: %s", probeProviderUUID)
	}
	if !p.Enabled {
		return nil, nil, nil, true, fmt.Errorf("probe provider is disabled: %s", p.Name)
	}

	syntheticRule := &typ.Rule{
		UUID:          "probe-direct:" + probeProviderUUID,
		Scenario:      scenario,
		RequestModel:  requestModel,
		ResponseModel: probeModel,
		Active:        true,
	}
	syntheticService := &loadbalance.Service{
		Provider: probeProviderUUID,
		Model:    probeModel,
		Active:   true,
	}
	return p, syntheticRule, syntheticService, true, nil
}
