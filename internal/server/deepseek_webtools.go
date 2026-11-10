package server

import (
	"context"
	"net/url"
	"sort"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/db"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/webtools/deepseek"
	"go.opentelemetry.io/otel/trace"
)

func resolveDeepSeekProvider(providers []*typ.Provider) *typ.Provider {
	// Deterministic when more than one enabled DeepSeek credential exists.
	candidates := append([]*typ.Provider(nil), providers...)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i] == nil {
			return false
		}
		if candidates[j] == nil {
			return true
		}
		return candidates[i].UUID < candidates[j].UUID
	})
	for _, p := range candidates {
		if p == nil || !p.Enabled || p.Token == "" || (p.AuthType != "" && p.AuthType != typ.AuthTypeAPIKey) {
			continue
		}
		base := p.APIBaseOpenAI
		if base == "" {
			base = p.APIBase
		}
		u, err := url.Parse(base)
		// Only first-party credentials, never a similarly named third-party vendor.
		if err == nil && u.Scheme == "https" && u.Hostname() == "api.deepseek.com" && u.User == nil {
			return p
		}
	}
	return nil
}

func (s *Server) newDeepSeekWebtools() *deepseek.Service {
	return &deepseek.Service{
		Resolve: func() *typ.Provider {
			if s.config == nil {
				return nil
			}
			return resolveDeepSeekProvider(s.config.ListProviders())
		},
		Record: func(ctx context.Context, p *typ.Provider, op string, billable bool, latency time.Duration, callErr error) {
			store := s.UsageStore()
			if store == nil {
				return
			}
			record := &db.UsageRecord{ProviderUUID: p.UUID, ProviderName: p.Name, Model: "deepseek-tools/" + op, Scenario: "webtools", EventType: "requests", UnitPrice: deepseek.PriceCNY, Currency: "CNY", LatencyMs: int(latency.Milliseconds())}
			if span := trace.SpanContextFromContext(ctx); span.HasTraceID() {
				record.TraceID = span.TraceID().String()
			}
			identity := coretool.GetUsageIdentity(ctx)
			record.UserID, record.TeamID, record.RuleUUID = identity.UserID, identity.TeamID, identity.RuleUUID
			if identity.Scenario != "" {
				record.Scenario = identity.Scenario
			}
			if billable {
				record.BillableRequests = 1
				record.RequestCostCNY = deepseek.PriceCNY
			}
			if callErr != nil {
				record.Status = "error"
				record.ErrorCode = "deepseek_webtool_error"
			}
			if err := store.RecordUsage(record); err != nil {
				logrus.WithError(err).Error("failed to record DeepSeek web tool usage")
			}
		},
	}
}
