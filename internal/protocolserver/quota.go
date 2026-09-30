package protocolserver

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// QuotaReader reads stored provider quota; it never fetches upstream, so the
// quota endpoint cannot drive requests to the vendors behind this box.
type QuotaReader interface {
	GetQuota(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
}

// HandleScenarioQuota serves GET /tingly/:scenario[/v1]/quota: the quota of
// the providers behind the scenario's rules, relayed as one
// ProviderUsage for a downstream tingly-box to display
// (.design/quota-relay.md). Only the owner's model token reads it; sharing
// keys are refused until a team can opt in.
func (ph *ProtocolHandler) HandleScenarioQuota(c *gin.Context) {
	if c.GetString(constant.CtxKeyAuthKind) == constant.AuthKindSharingKey {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: ErrorDetail{Message: "Quota is not shared with sharing keys", Type: "forbidden_error"}})
		return
	}
	if ph.deps.QuotaReader == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: ErrorDetail{Message: "Quota is not available on this server", Type: "service_unavailable"}})
		return
	}

	scenario := typ.RuleScenario(c.Param("scenario"))
	seen := make(map[string]bool)
	var upstreams []*quota.ProviderUsage
	for _, rule := range ph.deps.Config.GetRequestConfigs() {
		if !rule.Active || rule.GetScenario() != scenario {
			continue
		}
		for _, svc := range rule.GetServices() {
			if svc == nil || !svc.Active || seen[svc.Provider] {
				continue
			}
			seen[svc.Provider] = true
			if usage, err := ph.deps.QuotaReader.GetQuota(c.Request.Context(), svc.Provider); err == nil {
				upstreams = append(upstreams, usage)
			}
		}
	}
	c.JSON(http.StatusOK, quota.RelayUsage(upstreams))
}
