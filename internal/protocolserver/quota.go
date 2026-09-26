package protocolserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// QuotaReader is the read side of the quota manager: stored usage, never an
// upstream fetch, so a caller of the quota endpoint cannot drive requests to
// the vendors behind this box.
type QuotaReader interface {
	GetQuota(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
}

// HandleScenarioQuota serves GET /tingly/:scenario[/v1]/quota: the quota
// behind every model the credential can reach on this scenario, projected
// per model so no provider or account identity leaves the box. Another
// tingly-box pointed at this scenario reads it through its tingly_box quota
// fetcher. See .design/quota-relay.md.
//
// A sharing key reads quota only when its team shares it, and then only as
// percentages and reset times; the global model token belongs to the
// operator and reads the windows as stored.
func (ph *ProtocolHandler) HandleScenarioQuota(c *gin.Context) {
	sharingKey := c.GetString(constant.CtxKeyAuthKind) == constant.AuthKindSharingKey
	if sharingKey {
		teamID := c.GetString(constant.CtxKeyTeamID)
		if ph.deps.TeamQuotaVisible == nil || !ph.deps.TeamQuotaVisible(teamID) {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: ErrorDetail{
				Message: "Quota is not shared with this team's keys",
				Type:    "forbidden_error",
			}})
			return
		}
	}

	cfg := ph.deps.Config
	if cfg == nil || ph.deps.QuotaReader == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: ErrorDetail{
			Message: "Quota is not available on this server",
			Type:    "service_unavailable",
		}})
		return
	}

	scenario := typ.RuleScenario(c.Param("scenario"))
	if !IsValidRuleScenario(scenario) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: ErrorDetail{
			Message: fmt.Sprintf("invalid scenario: %s", scenario),
			Type:    "invalid_request_error",
		}})
		return
	}

	ctx := c.Request.Context()
	usageByProvider := make(map[string]*quota.ProviderUsage)
	usageFor := func(providerUUID string) *quota.ProviderUsage {
		if usage, looked := usageByProvider[providerUUID]; looked {
			return usage
		}
		usage, err := ph.deps.QuotaReader.GetQuota(ctx, providerUUID)
		if err != nil {
			usage = nil
		}
		usageByProvider[providerUUID] = usage
		return usage
	}

	resp := quota.GatewayQuota{Models: []quota.ModelQuota{}}
	for _, rule := range cfg.GetRequestConfigs() {
		if !rule.Active || !ShouldIncludeRuleInModelList(scenario, rule.GetScenario()) {
			continue
		}
		var services []quota.ServiceQuota
		for _, svc := range ruleServices(&rule) {
			if usage := usageFor(svc.Provider); usage != nil {
				services = append(services, quota.ServiceQuota{Model: svc.Model, Usage: usage})
			}
		}
		resp.Models = append(resp.Models, quota.ProjectModelQuota(rule.RequestModel, services, sharingKey))
	}
	c.JSON(http.StatusOK, resp)
}

// ruleServices lists every active service a request for the rule can land
// on: the default pool and each smart-routing branch, once per service.
func ruleServices(rule *typ.Rule) []*loadbalance.Service {
	seen := make(map[string]struct{})
	var out []*loadbalance.Service
	add := func(services []*loadbalance.Service) {
		for _, svc := range services {
			if svc == nil || !svc.Active || svc.Provider == "" {
				continue
			}
			if _, ok := seen[svc.ServiceID()]; ok {
				continue
			}
			seen[svc.ServiceID()] = struct{}{}
			out = append(out, svc)
		}
	}
	add(rule.GetServices())
	for _, branch := range rule.SmartRouting {
		add(branch.Services)
	}
	return out
}
