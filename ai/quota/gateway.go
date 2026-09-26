package quota

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// A tingly-box gateway can itself be the upstream of another tingly-box: an
// edge install points a provider at a central box's /tingly/<scenario> URL.
// The edge cannot read the central box's provider quota — it holds a model
// credential, not the vendor keys — so the central box serves a projection of
// it: per model the caller can reach, never per provider. The edge's
// tingly_box fetcher turns that back into a ProviderUsage.
// See .design/quota-relay.md.

// GatewayQuota is what GET /tingly/<scenario>/quota returns: the quota behind
// every model the calling credential can reach on that scenario.
type GatewayQuota struct {
	Models []ModelQuota `json:"models"`
}

// ModelQuota is one model's quota as the gateway discloses it. It names the
// model the caller asked for, never the provider or account behind it.
type ModelQuota struct {
	Model string `json:"model"`
	// Windows of the backing service with the most headroom — the capacity a
	// request for this model can still reach.
	Windows    []*UsageWindow `json:"windows,omitempty"`
	RecoversAt *time.Time     `json:"recovers_at,omitempty"`
	FetchedAt  *time.Time     `json:"fetched_at,omitempty"`
	// Unreadable is set when no backing service has readable quota. The
	// reason stays on the gateway: vendor error text can carry URLs and key
	// fragments.
	Unreadable bool `json:"unreadable,omitempty"`
}

// ServiceQuota pairs a service's model with its provider's stored usage, the
// input to ProjectModelQuota.
type ServiceQuota struct {
	Model string
	Usage *ProviderUsage
}

// ProjectModelQuota reduces the services behind one model to what a caller of
// that model gets to see. The service with the most headroom speaks for the
// model — routing sends a request wherever capacity remains, so the best
// service is the honest answer to "how much is left", and averaging would
// understate an exhausted pool (quota-semantics.md §2.2).
//
// With redact, the caller holds a sharing key: windows are reduced to
// percentages and reset times, and anything without a usage figure is
// dropped, so no absolute amount or balance leaves the gateway.
func ProjectModelQuota(model string, services []ServiceQuota, redact bool) ModelQuota {
	out := ModelQuota{Model: model}

	var best *ProviderUsage
	bestPct, bestKnown := 0.0, false
	for _, svc := range services {
		// Only Windows, FetchedAt and RecoversAt are read off the view; the
		// account, cost and raw response it still carries never reach out.
		view := svc.Usage.ForModel(svc.Model)
		if view == nil || len(view.Windows) == 0 {
			continue
		}
		pct, ok := view.Pct()
		switch {
		case best == nil,
			ok && !bestKnown,
			ok && bestKnown && pct < bestPct:
			best, bestPct, bestKnown = view, pct, ok
		}
	}
	if best == nil {
		out.Unreadable = true
		return out
	}

	fetchedAt := best.FetchedAt
	out.FetchedAt = &fetchedAt
	out.RecoversAt = best.RecoversAt()
	for _, w := range best.Windows {
		if w == nil {
			continue
		}
		if redact {
			w = redactWindow(w)
			if w == nil {
				continue
			}
		} else {
			clone := *w
			w = &clone
		}
		out.Windows = append(out.Windows, w)
	}
	if len(out.Windows) == 0 {
		out.Unreadable = true
		out.RecoversAt = nil
	}
	return out
}

// modelBreakdown finds the breakdown scoped to model, if any.
func (p *ProviderUsage) modelBreakdown(model string) *UsageBreakdown {
	if p == nil || model == "" {
		return nil
	}
	for _, bd := range p.Breakdowns {
		if bd != nil && bd.Group == BreakdownGroupModel && bd.Key == model {
			return bd
		}
	}
	return nil
}

// BreakdownGroupModel is the breakdown group for per-model quota.
const BreakdownGroupModel = "model"

// redactWindow keeps what answers "how much is left, when does it come back"
// and nothing that measures the account: a countable window becomes a
// percentage of 100, an uncapped one keeps its flag, and a window whose only
// figure is an absolute amount (a balance without a cap) is dropped.
func redactWindow(w *UsageWindow) *UsageWindow {
	out := &UsageWindow{
		Key:           w.Key,
		Type:          w.Type,
		Kind:          w.Kind,
		Label:         w.Label,
		ResetsAt:      w.ResetsAt,
		WindowMinutes: w.WindowMinutes,
		Unit:          UsageUnitPercent,
		Allowed:       w.Allowed,
		LimitReached:  w.LimitReached,
	}
	switch {
	case w.Countable():
		used := min(max(w.Percent(), 0), 100)
		available := 100 - used
		out.Used, out.Limit, out.UsedPercent, out.Available = used, 100, used, &available
	case w.Unlimited:
		out.Unlimited = true
	default:
		return nil
	}
	return out
}

// ForModel returns the usage as a request for model sees it. A gateway
// serves many models from one provider entry and they run out independently,
// so its view is the model's own breakdown (no windows when the gateway does
// not report the model); every other provider type is returned as is. Surfaces
// that know the model — service_quota routing, the status line — read this
// rather than the whole gateway, whose account-level windows are one per model.
func (p *ProviderUsage) ForModel(model string) *ProviderUsage {
	if p == nil || p.ProviderType != ProviderTypeTinglyBox {
		return p
	}
	view := *p
	view.Windows = nil
	if bd := p.modelBreakdown(model); bd != nil {
		view.Windows = bd.Windows
	}
	view.Breakdowns = nil
	return &view
}

// GatewayUsage maps a gateway's reply back into a ProviderUsage. Each model
// becomes a breakdown carrying its full windows (what ForModel and a
// further gateway read), and its binding window is lifted into Windows,
// labelled with the model, so the provider card shows one bar per model.
func GatewayUsage(gq *GatewayQuota) *ProviderUsage {
	usage := &ProviderUsage{}
	for _, mq := range gq.Models {
		if mq.Model == "" {
			continue
		}
		if mq.Unreadable || len(mq.Windows) == 0 {
			usage.AddBreakdown(mq.Model, mq.Model, BreakdownGroupModel, &UsageWindow{
				Type: WindowTypeCustom, Label: mq.Model, Unknown: true,
			})
			continue
		}
		usage.AddBreakdown(mq.Model, mq.Model, BreakdownGroupModel, mq.Windows...)
		binding := (&ProviderUsage{Windows: mq.Windows}).Tightest()
		if binding == nil {
			binding = mq.Windows[0]
		}
		lifted := *binding
		lifted.Label = gatewayWindowLabel(mq.Model, binding)
		usage.AddWindow(mq.Model+"/"+binding.Key, &lifted)
	}
	usage.NormalizeWindows()
	return usage
}

func gatewayWindowLabel(model string, w *UsageWindow) string {
	if w.Label == "" {
		return model
	}
	return model + " · " + w.Label
}

// GatewayQuotaURL derives the quota endpoint of a tingly-box gateway from a
// provider's API base, and reports whether the base addresses one at all. A
// gateway is recognised by its route shape — /tingly/<scenario>, optionally
// behind a reverse-proxy prefix and followed by /v1 — rather than by host,
// because an edge reaches the central box on any host, local or remote.
func GatewayQuotaURL(apiBase string) (string, bool) {
	trimmed := strings.TrimSpace(apiBase)
	if trimmed == "" {
		return "", false
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	i := slices.Index(segments, "tingly")
	if i < 0 || i+1 >= len(segments) || segments[i+1] == "" {
		return "", false
	}
	prefix := strings.Join(segments[:i+2], "/")
	return fmt.Sprintf("%s://%s/%s/quota", parsed.Scheme, parsed.Host, prefix), true
}
