// Package limits exposes the live state of account concurrency / usage limits
// (see .design/routing-limits.md) so the UI can show who is busy right now.
package limits

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// LimitStatusEntry is the live state of one limit: an account, a rule's
// service share, a model, a person or a team.
type LimitStatusEntry struct {
	Scope         string    `json:"scope" example:"account" description:"account | service | model | user | team"`
	Key           string    `json:"key"`
	Ref           string    `json:"ref,omitempty" description:"policy ref for user/team/model scopes (user_id, team id, model name)"`
	Label         string    `json:"label,omitempty" description:"human-readable name (key display name, team name, provider name)"`
	TeamID        string    `json:"team_id,omitempty"`
	InheritedFrom string    `json:"inherited_from,omitempty" description:"team_member when a per-member template governs this person"`
	Overflow      string    `json:"overflow,omitempty" description:"actor scopes: queue | reject"`
	QueueTimeout  int       `json:"queue_timeout_seconds,omitempty"`
	ProviderUUID  string    `json:"provider_uuid,omitempty"`
	ProviderName  string    `json:"provider_name,omitempty"`
	RuleUUID      string    `json:"rule_uuid,omitempty"`
	Scenario      string    `json:"scenario,omitempty"`
	RequestModel  string    `json:"request_model,omitempty"`
	Model         string    `json:"model,omitempty"`
	Limits        ai.Limits `json:"limits"`
	Inflight      int       `json:"inflight"`
	Requests      int64     `json:"requests" description:"requests admitted in the current usage window"`
	Tokens        int64     `json:"tokens" description:"tokens recorded in the current usage window"`
	Pressure      float64   `json:"pressure" description:"largest used/limit ratio; 1 = at the hard limit"`
	State         string    `json:"state" example:"ok" description:"ok | hot | blocked"`
}

// LimitsStatusResponse is the response of GET /limits/status.
type LimitsStatusResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    []LimitStatusEntry `json:"data"`
}

// LimitPolicyUpsertRequest is the body of PUT /limit-policies/{kind}/{ref}.
// An empty `limits` object deletes the policy.
type LimitPolicyUpsertRequest struct {
	Limits              ai.Limits `json:"limits"`
	Overflow            string    `json:"overflow,omitempty" description:"user/team/team_member: queue (default) | reject"`
	QueueTimeoutSeconds int       `json:"queue_timeout_seconds,omitempty"`
}

// LimitPolicyQuery filters GET /limit-policies.
type LimitPolicyQuery struct {
	Kind string `form:"kind" description:"user | team | team_member | model"`
	Ref  string `form:"ref" description:"user_id, team id or model name"`
}

// LimitPoliciesResponse is the response of GET /limit-policies.
type LimitPoliciesResponse struct {
	Success bool             `json:"success" example:"true"`
	Data    []ai.LimitPolicy `json:"data"`
}

// LimitPolicyResponse is the response of PUT/DELETE /limit-policies/{kind}/{ref}.
type LimitPolicyResponse struct {
	Success bool            `json:"success" example:"true"`
	Exists  bool            `json:"exists" description:"false after a delete or when the saved policy held no limit"`
	Data    *ai.LimitPolicy `json:"data,omitempty"`
}

// KeyInfo describes one sharing key as far as limits care (never its secret).
type KeyInfo struct {
	UserID      string
	TeamID      string
	DisplayName string
	Enabled     bool
}

// Directory resolves the people and teams policies refer to.
type Directory interface {
	// TeamName returns the team's name ("" when the team does not exist).
	TeamName(teamID string) string
	// KeysOfTeam lists the sharing keys bound to a team.
	KeysOfTeam(teamID string) []KeyInfo
	// KeyByUserID finds the sharing key behind a user_id.
	KeyByUserID(userID string) (KeyInfo, bool)
}

// Deps are the optional collaborators of the person / team / model scopes.
// A nil Policies store switches those scopes (and their API) off.
type Deps struct {
	Policies  *db.LimitPolicyStore
	Directory Directory
	// OnPolicyChanged runs after a policy is saved (warm start of its window).
	OnPolicyChanged func(ai.LimitPolicy)
}

// Handler serves the limits status and policy APIs.
type Handler struct {
	cfg   *config.Config
	guard *routing.LimitsGuard
	deps  Deps
}

// NewHandler creates the handler. guard may be nil (limits off: empty status).
func NewHandler(cfg *config.Config, guard *routing.LimitsGuard, deps Deps) *Handler {
	return &Handler{cfg: cfg, guard: guard, deps: deps}
}

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts GET /limits/status.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	rt.V1.GET("/limits/status", h.Status,
		swagger.WithTags("limits"),
		swagger.WithDescription("Live state (in-flight, usage window, pressure) of every account and rule service that has limits configured"),
		swagger.WithResponseModel(LimitsStatusResponse{}),
	)
	rt.V1.GET("/limit-policies", h.ListPolicies,
		swagger.WithTags("limits"),
		swagger.WithDescription("List person / team / model limit policies (optional kind and ref filters)"),
		swagger.WithQueryModel(LimitPolicyQuery{}),
		swagger.WithResponseModel(LimitPoliciesResponse{}),
	)
	rt.V1.PUT("/limit-policies/:kind/:ref", h.UpsertPolicy,
		swagger.WithTags("limits"),
		swagger.WithDescription("Create or replace a limit policy for a person (kind user, ref = sharing key user_id), a whole team (team), each member of a team (team_member) or an upstream model (model). An empty limits object deletes it"),
		swagger.WithRequestModel(LimitPolicyUpsertRequest{}),
		swagger.WithResponseModel(LimitPolicyResponse{}),
	)
	rt.V1.DELETE("/limit-policies/:kind/:ref", h.DeletePolicy,
		swagger.WithTags("limits"),
		swagger.WithDescription("Delete a limit policy"),
		swagger.WithResponseModel(LimitPolicyResponse{}),
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
		fill(&e, limits.Subject{Key: routing.AccountLimitKey(p.UUID), Scope: limits.ScopeAcct, Limits: *l, Quota: p.UUID})
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
			fill(&e, limits.Subject{Key: routing.ServiceLimitKey(r.UUID, svc), Scope: limits.ScopeSvc, Limits: *l})
			out = append(out, e)
		}
	}
	out = append(out, h.actorEntries(fill)...)
	c.JSON(http.StatusOK, LimitsStatusResponse{Success: true, Data: out})
}

func (h *Handler) teamName(id string) string {
	if h.deps.Directory == nil {
		return ""
	}
	return h.deps.Directory.TeamName(id)
}

// actorEntries lists the person / team / model policies with their live counters,
// plus one entry per member key of every team that has a per-member template
// (so the UI can show each person's usage under the template).
func (h *Handler) actorEntries(fill func(*LimitStatusEntry, limits.Subject)) []LimitStatusEntry {
	if h.deps.Policies == nil {
		return nil
	}
	var out []LimitStatusEntry
	explicitUsers := map[string]bool{}
	policies := h.deps.Policies.List("", "")
	for _, p := range policies {
		if p.Kind == ai.LimitPolicyUser {
			explicitUsers[p.Ref] = true
		}
	}
	for _, p := range policies {
		base := LimitStatusEntry{Ref: p.Ref, Overflow: p.Overflow, QueueTimeout: p.QueueTimeoutSeconds}
		switch p.Kind {
		case ai.LimitPolicyUser:
			e := base
			e.Scope = "user"
			if h.deps.Directory != nil {
				if k, ok := h.deps.Directory.KeyByUserID(p.Ref); ok {
					e.Label, e.TeamID = k.DisplayName, k.TeamID
				}
			}
			fill(&e, limits.Subject{Key: routing.UserLimitKey(p.Ref), Scope: limits.ScopeUser, Limits: p.Limits})
			out = append(out, e)
		case ai.LimitPolicyTeam:
			e := base
			e.Scope, e.Label, e.TeamID = "team", h.teamName(p.Ref), p.Ref
			fill(&e, limits.Subject{Key: routing.TeamLimitKey(p.Ref), Scope: limits.ScopeTeam, Limits: p.Limits})
			out = append(out, e)
		case ai.LimitPolicyModel:
			e := base
			e.Scope, e.Label, e.Model = "model", p.Ref, p.Ref
			fill(&e, limits.Subject{Key: routing.ModelLimitKey(p.Ref), Scope: limits.ScopeModel, Limits: p.Limits})
			out = append(out, e)
		case ai.LimitPolicyTeamMember:
			if h.deps.Directory == nil {
				continue
			}
			for _, k := range h.deps.Directory.KeysOfTeam(p.Ref) {
				if explicitUsers[k.UserID] {
					continue // governed by its own policy, listed above
				}
				e := base
				e.Scope, e.Label, e.TeamID, e.InheritedFrom = "user", k.DisplayName, p.Ref, "team_member"
				e.Ref = k.UserID
				fill(&e, limits.Subject{Key: routing.UserLimitKey(k.UserID), Scope: limits.ScopeUser, Limits: p.Limits})
				out = append(out, e)
			}
			// the template itself, so the UI can edit it
			e := base
			e.Scope, e.Label, e.TeamID, e.InheritedFrom = "team_member", h.teamName(p.Ref), p.Ref, "template"
			e.Key, e.Limits = "team_member:"+p.Ref, p.Limits
			e.State = "ok"
			out = append(out, e)
		}
	}
	return out
}

func (h *Handler) policiesAvailable(c *gin.Context) bool {
	if h.deps.Policies == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "limit policies are not available"})
		return false
	}
	return true
}

// ListPolicies GET /limit-policies.
func (h *Handler) ListPolicies(c *gin.Context) {
	if !h.policiesAvailable(c) {
		return
	}
	list := h.deps.Policies.List(ai.LimitPolicyKind(c.Query("kind")), c.Query("ref"))
	c.JSON(http.StatusOK, LimitPoliciesResponse{Success: true, Data: list})
}

// validateRef checks that the policy's subject exists.
func (h *Handler) validateRef(kind ai.LimitPolicyKind, ref string) string {
	if !kind.IsValid() {
		return "unknown policy kind '" + string(kind) + "' (use user, team, team_member or model)"
	}
	if strings.TrimSpace(ref) == "" {
		return "ref is required"
	}
	switch kind {
	case ai.LimitPolicyTeam, ai.LimitPolicyTeamMember:
		if h.deps.Directory != nil && h.deps.Directory.TeamName(ref) == "" {
			return "team '" + ref + "' not found"
		}
	case ai.LimitPolicyUser:
		if !strings.HasPrefix(ref, "euser:") && h.deps.Directory != nil {
			if _, ok := h.deps.Directory.KeyByUserID(ref); !ok {
				return "no sharing key with user_id '" + ref + "'"
			}
		}
	}
	return ""
}

// UpsertPolicy PUT /limit-policies/:kind/:ref.
func (h *Handler) UpsertPolicy(c *gin.Context) {
	if !h.policiesAvailable(c) {
		return
	}
	kind, ref := ai.LimitPolicyKind(c.Param("kind")), c.Param("ref")
	if msg := h.validateRef(kind, ref); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": msg})
		return
	}
	var req LimitPolicyUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	saved, exists, err := h.deps.Policies.Upsert(ai.LimitPolicy{
		Kind: kind, Ref: ref, Limits: req.Limits, Overflow: req.Overflow, QueueTimeoutSeconds: req.QueueTimeoutSeconds,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusOK, LimitPolicyResponse{Success: true, Exists: false})
		return
	}
	if h.deps.OnPolicyChanged != nil {
		h.deps.OnPolicyChanged(saved)
	}
	c.JSON(http.StatusOK, LimitPolicyResponse{Success: true, Exists: true, Data: &saved})
}

// DeletePolicy DELETE /limit-policies/:kind/:ref.
func (h *Handler) DeletePolicy(c *gin.Context) {
	if !h.policiesAvailable(c) {
		return
	}
	kind, ref := ai.LimitPolicyKind(c.Param("kind")), c.Param("ref")
	if !kind.IsValid() || ref == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "unknown policy kind or empty ref"})
		return
	}
	if err := h.deps.Policies.Delete(kind, ref); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, LimitPolicyResponse{Success: true, Exists: false})
}
