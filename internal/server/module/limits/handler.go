// Package limits exposes the live state of account concurrency / usage limits
// (see .design/routing-limits.md) so the UI can show who is busy right now.
package limits

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// LimitStatusEntry is the live state of one limited account or rule service.
type LimitStatusEntry struct {
	Scope        string    `json:"scope" example:"account" description:"account (Provider.Limits) or service (one rule's share)"`
	Key          string    `json:"key"`
	ProviderUUID string    `json:"provider_uuid"`
	ProviderName string    `json:"provider_name,omitempty"`
	RuleUUID     string    `json:"rule_uuid,omitempty"`
	Scenario     string    `json:"scenario,omitempty"`
	RequestModel string    `json:"request_model,omitempty"`
	Model        string    `json:"model,omitempty"`
	Limits       ai.Limits `json:"limits"`
	Inflight     int       `json:"inflight"`
	Requests     int64     `json:"requests" description:"requests admitted in the current usage window"`
	Tokens       int64     `json:"tokens" description:"tokens recorded in the current usage window"`
	Pressure     float64   `json:"pressure" description:"largest used/limit ratio; 1 = at the hard limit"`
	State        string    `json:"state" example:"ok" description:"ok | hot | blocked"`
}

// LimitsStatusResponse is the response of GET /limits/status.
type LimitsStatusResponse struct {
	Success bool          `json:"success" example:"true"`
	Data    []LimitStatusEntry `json:"data"`
}

// Handler serves the limits status API.
type Handler struct {
	cfg   *config.Config
	guard *routing.LimitsGuard
}

// NewHandler creates the handler. guard may be nil (limits off: empty status).
func NewHandler(cfg *config.Config, guard *routing.LimitsGuard) *Handler {
	return &Handler{cfg: cfg, guard: guard}
}

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts GET /limits/status.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	rt.V1.GET("/limits/status", h.Status,
		swagger.WithTags("limits"),
		swagger.WithDescription("Live state (in-flight, usage window, pressure) of every account and rule service that has limits configured"),
		swagger.WithResponseModel(LimitsStatusResponse{}),
	)
}

// Status lists every configured limit with its live counters.
func (h *Handler) Status(c *gin.Context) {
	out := []LimitStatusEntry{}
	if h.guard == nil || h.cfg == nil {
		c.JSON(http.StatusOK, LimitsStatusResponse{Success: true, Data: out})
		return
	}
	reg := h.guard.Registry()
	fill := func(e *LimitStatusEntry, sub limits.Subject) {
		st := reg.Stat(sub.Key)
		v := reg.Assess([]limits.Subject{sub})
		e.Key, e.Limits = sub.Key, sub.Limits
		e.Inflight, e.Requests, e.Tokens = st.Inflight, st.Requests, st.Tokens
		e.Pressure, e.State = v.Pressure, v.State.String()
	}

	for _, p := range h.cfg.ListProviders() {
		l := p.Limits.Normalize()
		if l == nil {
			continue
		}
		e := LimitStatusEntry{Scope: "account", ProviderUUID: p.UUID, ProviderName: p.Name}
		fill(&e, limits.Subject{Key: routing.AccountLimitKey(p.UUID), Limits: *l, Quota: p.UUID})
		out = append(out, e)
	}
	rules := h.cfg.GetRequestConfigs()
	for i := range rules {
		r := &rules[i]
		for _, svc := range r.Services {
			if svc == nil {
				continue
			}
			l := svc.Limits.Normalize()
			if l == nil {
				continue
			}
			l.QuotaPercent = 0
			e := LimitStatusEntry{
				Scope: "service", ProviderUUID: svc.Provider, RuleUUID: r.UUID, Scenario: string(r.GetScenario()),
				RequestModel: r.RequestModel, Model: svc.Model,
			}
			if p, err := h.cfg.GetProviderByUUID(svc.Provider); err == nil && p != nil {
				e.ProviderName = p.Name
			}
			fill(&e, limits.Subject{Key: routing.ServiceLimitKey(r.UUID, svc), Limits: *l})
			out = append(out, e)
		}
	}
	c.JSON(http.StatusOK, LimitsStatusResponse{Success: true, Data: out})
}
