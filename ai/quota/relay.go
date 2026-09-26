package quota

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// A tingly-box can be the upstream of another tingly-box: an edge install
// points a provider at a central box's /tingly/<scenario> URL. The edge holds
// a model credential, not the vendor keys, so the central box relays its own
// quota as one ProviderUsage that the edge's tingly_box fetcher stores as is.
// Display only: nothing on the edge routes on it. See .design/quota-relay.md.

// RelayUsage merges the quota of the upstream providers behind a scenario
// into one ProviderUsage. Providers are anonymised as "upstream N" in the
// order given; account, cost, breakdowns and raw responses are dropped, and
// a provider with no windows (unreadable, unsupported) is left out.
//
// With percentOnly — the caller holds a sharing key — each window keeps only
// its percentage and reset time, and a window whose only figure is an
// absolute amount (a balance without a cap) is dropped.
func RelayUsage(upstreams []*ProviderUsage, percentOnly bool) *ProviderUsage {
	out := &ProviderUsage{ProviderType: ProviderTypeTinglyBox}
	n := 0
	for _, up := range upstreams {
		if up == nil {
			continue
		}
		var relayed []*UsageWindow
		for _, w := range up.Windows {
			if w == nil {
				continue
			}
			if percentOnly {
				w = percentOnlyWindow(w)
			} else {
				clone := *w
				w = &clone
			}
			if w != nil {
				relayed = append(relayed, w)
			}
		}
		if len(relayed) == 0 {
			continue
		}
		n++
		for _, w := range relayed {
			w.Label = fmt.Sprintf("upstream %d · %s", n, windowLabel(w))
			w.Key = fmt.Sprintf("upstream_%d/%s", n, w.Key)
		}
		out.Windows = append(out.Windows, relayed...)
		// The oldest reading is the honest age of the whole relay.
		if out.FetchedAt.IsZero() || up.FetchedAt.Before(out.FetchedAt) {
			out.FetchedAt = up.FetchedAt
		}
	}
	return out
}

func windowLabel(w *UsageWindow) string {
	if w.Label != "" {
		return w.Label
	}
	return string(w.Type)
}

// percentOnlyWindow keeps what answers "how much is left, when does it come
// back" and nothing that measures the account.
func percentOnlyWindow(w *UsageWindow) *UsageWindow {
	out := &UsageWindow{
		Key:           w.Key,
		Label:         w.Label,
		Type:          w.Type,
		Kind:          w.Kind,
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

// GatewayQuotaURL derives the quota endpoint of a tingly-box from a
// provider's API base, and reports whether the base addresses one at all. A
// tingly-box is recognised by its route shape — /tingly/<scenario>, optionally
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
