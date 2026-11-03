package server

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/routing"
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
			events := make([]limits.UsageEvent, len(rows))
			for i, r := range rows {
				events[i] = limits.UsageEvent{At: r.At, Requests: r.Requests, Tokens: r.Tokens}
			}
			reg.Seed(key, s.Limits, events)
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
