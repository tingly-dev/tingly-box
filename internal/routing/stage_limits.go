package routing

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SourceLimits names the stage in EvaluatedStages.
const SourceLimits = "limits"

// LimitStage removes candidates that are at a concurrency/usage limit and
// prefers cool ones over "hot" ones (soft threshold). It is read-only: the
// permit is taken once, after the terminal pick (SimpleSelector). When every
// candidate is blocked the rule's overflow policy decides: queue, reject or
// ignore. See .design/routing-limits.md.
type LimitStage struct {
	guard *LimitsGuard
}

// NewLimitStage creates the stage; a nil guard makes it a pass-through.
func NewLimitStage(guard *LimitsGuard) *LimitStage { return &LimitStage{guard: guard} }

func (s *LimitStage) Name() string { return SourceLimits }

func (s *LimitStage) Evaluate(ctx *SelectionContext, candidates []*loadbalance.Service) ([]*loadbalance.Service, *SelectionResult, error) {
	if s.guard == nil || s.guard.reg == nil || len(candidates) == 0 {
		return candidates, nil, nil
	}
	rule := ctx.Rule
	reqCtx := selectionLogContext(ctx)

	if ctx.limitStart.IsZero() {
		ctx.limitStart = clock.Now()
	}

	for {
		c, limited := s.guard.classify(rule.UUID, candidates, ctx.LimitExcluded)
		if !limited {
			return candidates, nil, nil
		}
		if len(c.ok) > 0 {
			s.logDiverted(ctx, c, len(candidates), c.ok)
			return c.ok, nil, nil
		}
		if len(c.hot) > 0 {
			s.logDiverted(ctx, c, len(candidates), c.hot)
			return c.hot, nil, nil
		}

		// Every candidate is blocked.
		policy := rule.LimitOverflowPolicy()
		if policy == typ.LimitOverflowIgnore {
			ctx.LimitsIgnored = true
			logrus.WithContext(reqCtx).WithFields(logrus.Fields{
				"stage": "routing_limit_ignored", "rule_uuid": rule.UUID, "candidates": len(candidates),
			}).Warnf("[limits] all %d candidates at their limit; overflow=ignore, routing anyway", len(candidates))
			return candidates, nil, nil
		}

		var reasons limits.Reason
		retry := time.Duration(0)
		waitable := false
		for _, v := range c.verdicts {
			reasons |= v.Reasons
			if v.ConcurrencyOnly() {
				waitable = true
			}
			if retry == 0 || v.RetryAfter < retry {
				retry = v.RetryAfter
			}
		}
		waited := clock.Now().Sub(ctx.limitStart)

		if policy == typ.LimitOverflowQueue && waitable {
			remaining := rule.LimitQueueWait() - waited
			if remaining > 0 {
				changed := s.guard.reg.Changed()
				// Re-classify after grabbing the channel so a release between
				// classify and Changed() is not missed.
				if c2, _ := s.guard.classify(rule.UUID, candidates, ctx.LimitExcluded); len(c2.ok)+len(c2.hot) > 0 {
					continue
				}
				timer := time.NewTimer(remaining)
				var done <-chan struct{}
				if ctx.GinContext != nil && ctx.GinContext.Request != nil {
					done = ctx.GinContext.Request.Context().Done()
				}
				select {
				case <-changed:
					timer.Stop()
					ctx.LimitExcluded = nil // capacity moved: a previously lost race may now win
					continue
				case <-timer.C:
				case <-done:
					timer.Stop()
					waited = clock.Now().Sub(ctx.limitStart)
					return candidates, nil, &LimitError{RuleUUID: rule.UUID, Reasons: reasons, RetryAfter: retry, Waited: waited}
				}
				waited = clock.Now().Sub(ctx.limitStart)
			}
		}

		logrus.WithContext(reqCtx).WithFields(logrus.Fields{
			"stage": "routing_limit_rejected", "rule_uuid": rule.UUID, "policy": policy,
			"reasons": reasons.String(), "waited_ms": waited.Milliseconds(), "candidates": len(candidates),
		}).Warnf("[limits] rejecting request: all %d candidates at limit (%s), waited %s", len(candidates), reasons, waited.Round(time.Millisecond))
		return candidates, nil, &LimitError{RuleUUID: rule.UUID, Reasons: reasons, RetryAfter: retry, Waited: waited}
	}
}

func (s *LimitStage) logDiverted(ctx *SelectionContext, c limitClass, total int, kept []*loadbalance.Service) {
	if len(kept) == total {
		return
	}
	ids := make([]string, 0, len(c.blocked))
	for _, b := range c.blocked {
		ids = append(ids, b.ServiceID())
	}
	logrus.WithContext(selectionLogContext(ctx)).WithFields(logrus.Fields{
		"stage": "routing_limit_diverted", "rule_uuid": selectionRuleUUID(ctx),
		"blocked": ids, "hot": len(c.hot), "remaining": len(kept),
	}).Infof("[limits] steering away from %d blocked / %d hot of %d candidates", len(c.blocked), len(c.hot), total)
}
