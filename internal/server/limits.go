package server

import (
	"context"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/routing"
	limitsmodule "github.com/tingly-dev/tingly-box/internal/server/module/limits"
)

// initLimits enables account concurrency/usage limits on the selection
// pipeline (.design/routing-limits.md) and returns the guard.
func initLimits(ctx context.Context, cfg *config.Config, sel *routing.ServiceSelector) *routing.LimitsGuard {
	reg := limits.NewRegistry()
	guard := routing.NewLimitsGuard(reg, cfg)
	sel.SetLimits(guard)
	reg.StartSweeper(ctx)
	return guard
}

// wireLimitsQuota lets provider limits' quota_percent read ai/quota usage, with
// the same rule as the service_quota smart op: only standard, self-healing
// windows count and unknown never blocks.
func wireLimitsQuota(guard *routing.LimitsGuard, qp routing.QuotaProvider) {
	if guard == nil || qp == nil {
		return
	}
	guard.Registry().SetQuotaFunc(func(providerUUID string) (float64, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		usage, err := qp.GetQuota(ctx, providerUUID)
		if err != nil || usage == nil {
			return 0, false
		}
		return usage.Pct(quota.WindowKindLimit)
	})
}

// warmStartLimits seeds usage windows from usage_records so a restart does not
// hand out a fresh budget. Only keys that actually configure a budget are queried.
func warmStartLimits(guard *routing.LimitsGuard, cfg *config.Config, usage *db.UsageStore) {
	if guard == nil || cfg == nil || usage == nil {
		return
	}
	reg := guard.Registry()
	seed := func(key string, subs []limits.Subject, provider, model, rule string) {
		for _, s := range subs {
			if s.Key != key || !s.Limits.HasBudget() {
				continue
			}
			rows, err := usage.UsageMinutesSince(provider, model, rule, time.Now().Add(-s.Limits.Window()))
			if err != nil {
				logrus.WithError(err).Warnf("[limits] warm start for %s failed", key)
				return
			}
			reg.Seed(key, s.Limits, usageEvents(rows))
		}
	}
	seenAcct := map[string]bool{}
	rules := cfg.GetRequestConfigs()
	for i := range rules {
		rule := &rules[i]
		for _, svc := range rule.Services {
			if svc == nil {
				continue
			}
			subs := guard.Subjects(rule.UUID, svc)
			if acct := routing.AccountLimitKey(svc.Provider); !seenAcct[acct] {
				seenAcct[acct] = true
				seed(acct, subs, svc.Provider, "", "")
			}
			seed(routing.ServiceLimitKey(rule.UUID, svc), subs, svc.Provider, svc.Model, rule.UUID)
		}
	}
}

// policyWindowSince returns how far back a policy's window reaches.
func policyWindowSince(l ai.Limits) time.Time { return time.Now().Add(-l.Window()) }

func usageEvents(rows []db.UsageMinute) []limits.UsageEvent {
	events := make([]limits.UsageEvent, len(rows))
	for i, r := range rows {
		events[i] = limits.UsageEvent{At: r.At, Requests: r.Requests, Tokens: r.Tokens}
	}
	return events
}

func seedWindow(guard *routing.LimitsGuard, key string, l ai.Limits, rows []db.UsageMinute) {
	reg := guard.Registry()
	if !reg.WindowEmpty(key, l) { // already counting: seeding again would double count
		return
	}
	reg.Seed(key, l, usageEvents(rows))
}

// warmStartPolicy seeds the usage window(s) of a person / team / model policy
// from usage_records, so a restart (or a freshly saved policy) does not hand
// out a fresh budget. Idempotent: windows that already count are left alone.
// Enterprise identities ("euser:") are not recorded in usage_records and start empty.
func warmStartPolicy(guard *routing.LimitsGuard, usage *db.UsageStore, policies *db.LimitPolicyStore, p ai.LimitPolicy) {
	if guard == nil || usage == nil || !p.Limits.HasBudget() {
		return
	}
	since := policyWindowSince(p.Limits)
	fail := func(err error) {
		logrus.WithError(err).Warnf("[limits] warm start for %s %s failed", p.Kind, p.Ref)
	}
	// The usage_records column and limits key of the kinds that map to one.
	var column, key string
	switch p.Kind {
	case ai.LimitPolicyUser:
		if strings.HasPrefix(p.Ref, "euser:") {
			return
		}
		column, key = "user_id", routing.UserLimitKey(p.Ref)
	case ai.LimitPolicyTeam:
		column, key = "team_id", routing.TeamLimitKey(p.Ref)
	case ai.LimitPolicyModel:
		column, key = "model", routing.ModelLimitKey(p.Ref)
	case ai.LimitPolicyTeamMember:
		byUser, err := usage.UsageMinutesByUser(p.Ref, since)
		if err != nil {
			fail(err)
			return
		}
		for uid, rows := range byUser {
			if policies != nil {
				if _, explicit := policies.GetLimitPolicy(ai.LimitPolicyUser, uid); explicit {
					continue // the explicit policy governs (and seeds) this person
				}
			}
			seedWindow(guard, routing.UserLimitKey(uid), p.Limits, rows)
		}
		return
	default:
		return
	}
	rows, err := usage.UsageMinutesWhere(map[string]string{column: p.Ref}, since)
	if err != nil {
		fail(err)
		return
	}
	seedWindow(guard, key, p.Limits, rows)
}

// initLimitPolicies connects the person / team / model policy store to the
// guard and warm-starts every stored policy. Returns the store (nil when the
// database is unavailable: those scopes are then simply off).
func initLimitPolicies(guard *routing.LimitsGuard, cfg *config.Config, usage *db.UsageStore) *db.LimitPolicyStore {
	sm := cfg.StoreManager()
	if guard == nil || sm == nil || sm.LimitPolicy() == nil {
		return nil
	}
	store := sm.LimitPolicy()
	guard.SetPolicies(store)
	for _, p := range store.List("", "") {
		warmStartPolicy(guard, usage, store, p)
	}
	return store
}

// limitsDirectory resolves the people and teams that limit policies refer to
// from the team and API-token stores (membership = "has a key bound to the
// team"; the person is the key's user_id, never its secret).
type limitsDirectory struct {
	teams  *db.TeamStore
	tokens *db.APITokenStore
}

func (d limitsDirectory) TeamName(teamID string) string {
	if d.teams == nil {
		return ""
	}
	if t, err := d.teams.Get(teamID); err == nil && t != nil {
		return t.Name
	}
	return ""
}

func keyInfo(r db.APITokenRecord) limitsmodule.KeyInfo {
	return limitsmodule.KeyInfo{UserID: r.UserID, TeamID: r.TeamID, DisplayName: r.DisplayName, Enabled: r.Enabled}
}

func (d limitsDirectory) KeysOfTeam(teamID string) []limitsmodule.KeyInfo {
	if d.tokens == nil {
		return nil
	}
	records, _, err := d.tokens.ListTokensForTeam("", teamID, nil, 1000, 0)
	if err != nil {
		return nil
	}
	out := make([]limitsmodule.KeyInfo, 0, len(records))
	for _, r := range records {
		out = append(out, keyInfo(r))
	}
	return out
}

func (d limitsDirectory) KeyByUserID(userID string) (limitsmodule.KeyInfo, bool) {
	if d.tokens == nil || userID == "" {
		return limitsmodule.KeyInfo{}, false
	}
	records, _, err := d.tokens.ListTokensForTeam(userID, "", nil, 1, 0)
	if err != nil || len(records) == 0 {
		return limitsmodule.KeyInfo{}, false
	}
	return keyInfo(records[0]), true
}

// limitsModuleDeps builds the person / team / model collaborators of the limits API.
func (s *Server) limitsModuleDeps() limitsmodule.Deps {
	deps := limitsmodule.Deps{Policies: s.limitPolicies}
	if sm := s.config.StoreManager(); sm != nil {
		deps.Directory = limitsDirectory{teams: sm.Team(), tokens: sm.APIToken()}
	}
	deps.OnPolicyChanged = func(p ai.LimitPolicy) {
		warmStartPolicy(s.limitsGuard, s.UsageStore(), s.limitPolicies, p)
	}
	return deps
}
