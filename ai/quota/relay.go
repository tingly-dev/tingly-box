package quota

import (
	"cmp"
	"fmt"
	"net/url"
	"strings"
)

// A tingly-box can be the upstream of another: an edge points a provider at a
// central box's /tingly/<scenario>. The central box relays its quota as one
// ProviderUsage, which the edge's tingly_box fetcher stores as is. Display
// only. See .design/quota-relay.md.

// RelayUsage merges upstream providers into one ProviderUsage, labelling each
// window with its provider's name and keeping only its percentage and reset
// time — no account identity, absolute amount or balance.
func RelayUsage(upstreams []*ProviderUsage) *ProviderUsage {
	out := &ProviderUsage{ProviderType: ProviderTypeTinglyBox}
	n := 0
	for _, up := range upstreams {
		var windows []*UsageWindow
		for _, w := range up.Windows {
			if w == nil || !w.Countable() {
				continue
			}
			used := min(max(w.Percent(), 0), 100)
			left := 100 - used
			windows = append(windows, &UsageWindow{
				Key: w.Key, Label: w.Label, Type: w.Type, Kind: w.Kind,
				Used: used, Limit: 100, UsedPercent: used, Available: &left, Unit: UsageUnitPercent,
				ResetsAt: w.ResetsAt, WindowMinutes: w.WindowMinutes,
			})
		}
		if len(windows) == 0 {
			continue
		}
		n++
		for _, w := range windows {
			name := cmp.Or(up.ProviderName, fmt.Sprintf("upstream %d", n))
			w.Label = name + " · " + cmp.Or(w.Label, string(w.Type))
			w.Key = fmt.Sprintf("upstream_%d/%s", n, w.Key)
		}
		out.Windows = append(out.Windows, windows...)
		if out.FetchedAt.IsZero() || up.FetchedAt.Before(out.FetchedAt) {
			out.FetchedAt = up.FetchedAt // the oldest reading dates the relay
		}
	}
	return out
}

// GatewayQuotaURL maps a provider's API base to the tingly-box quota endpoint
// behind it, if the base is a tingly-box route (/tingly/<scenario>, possibly
// behind a proxy prefix and followed by /v1). The route shape, not the host,
// identifies a tingly-box: an edge reaches its central box on any host.
func GatewayQuotaURL(apiBase string) (string, bool) {
	base := strings.TrimSpace(apiBase)
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", false
	}
	prefix, rest, ok := strings.Cut(strings.TrimSuffix(u.Path, "/")+"/", "/tingly/")
	scenario, _, _ := strings.Cut(rest, "/")
	if !ok || scenario == "" {
		return "", false
	}
	return fmt.Sprintf("%s://%s%s/tingly/%s/quota", u.Scheme, u.Host, prefix, scenario), true
}
