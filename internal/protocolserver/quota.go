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

// HandleScenarioQuota serves GET /tingly/:scenario[/v1]/quota: the quota of
// the providers behind the scenario's rules, relayed as one anonymised
// ProviderUsage for a downstream tingly-box to display. See
// .design/quota-relay.md.
//
// A sharing key reads it only when its team shares quota, and then as
// percentages only; the global model token belongs to the operator and reads
// the windows as stored.
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

	// Each provider once, in rule order, so "upstream N" stays stable while
	// the configuration does.
	seen := make(map[string]struct{})
	var upstreams []*quota.ProviderUsage
	for _, rule := range cfg.GetRequestConfigs() {
		if !rule.Active || !ShouldIncludeRuleInModelList(scenario, rule.GetScenario()) {
			continue
		}
		for _, svc := range ruleServices(&rule) {
			if _, ok := seen[svc.Provider]; ok {
				continue
			}
			seen[svc.Provider] = struct{}{}
			if usage, err := ph.deps.QuotaReader.GetQuota(c.Request.Context(), svc.Provider); err == nil {
				upstreams = append(upstreams, usage)
			}
		}
	}
	c.JSON(http.StatusOK, quota.RelayUsage(upstreams, sharingKey))
}

// ruleServices lists the active services a request for the rule can land on:
// the default pool and each smart-routing branch.
func ruleServices(rule *typ.Rule) []*loadbalance.Service {
	var out []*loadbalance.Service
	add := func(services []*loadbalance.Service) {
		for _, svc := range services {
			if svc != nil && svc.Active && svc.Provider != "" {
				out = append(out, svc)
			}
		}
	}
	add(rule.GetServices())
	for _, branch := range rule.SmartRouting {
		add(branch.Services)
	}
	return out
}
