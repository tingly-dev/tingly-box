package fetcher

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai/quota"
)

// Claude subscription responses carry the account's rate-limit state in
// anthropic-ratelimit-unified-* headers; Claude Code's own quota check is a
// one-token request whose only purpose is to read them. Reading them from the
// responses we already proxy keeps the stored quota fresh without a poll and
// shows an exhausted window the moment it is rejected.
//
//	anthropic-ratelimit-unified-status        allowed | allowed_warning | rejected
//	anthropic-ratelimit-unified-5h-utilization  0..1 fraction (the poll reports percent)
//	anthropic-ratelimit-unified-5h-reset        unix seconds
//	anthropic-ratelimit-unified-5h-status       per-window status (2.1.328)
//	…-7d-…                                      the weekly window

// AnthropicUnifiedWindow is one rolling window read from response headers.
type AnthropicUnifiedWindow struct {
	UsedPercent float64   // 0-100; the header is a 0-1 fraction
	ResetsAt    time.Time // zero when absent
	Status      string    // per-window status, may be empty
}

// AnthropicUnified is the rate-limit state of one response.
type AnthropicUnified struct {
	Status   string // overall: allowed, allowed_warning, rejected
	FiveHour *AnthropicUnifiedWindow
	SevenDay *AnthropicUnifiedWindow
}

const anthropicUnifiedPrefix = "anthropic-ratelimit-unified-"

// ParseAnthropicUnifiedHeaders reads the unified rate-limit headers. ok is
// false when the response carries no window we can use.
func ParseAnthropicUnifiedHeaders(h http.Header) (*AnthropicUnified, bool) {
	u := &AnthropicUnified{Status: strings.TrimSpace(h.Get(anthropicUnifiedPrefix + "status"))}
	u.FiveHour = parseUnifiedWindow(h, "5h")
	u.SevenDay = parseUnifiedWindow(h, "7d")
	return u, u.FiveHour != nil || u.SevenDay != nil
}

func parseUnifiedWindow(h http.Header, name string) *AnthropicUnifiedWindow {
	raw := strings.TrimSpace(h.Get(anthropicUnifiedPrefix + name + "-utilization"))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return nil
	}
	w := &AnthropicUnifiedWindow{
		// The headers are fractions ("0.42"); a value above 1 can only be a
		// percentage already.
		UsedPercent: v,
		Status:      strings.TrimSpace(h.Get(anthropicUnifiedPrefix + name + "-status")),
	}
	if v <= 1 {
		w.UsedPercent = v * 100
	}
	if reset, err := strconv.ParseInt(strings.TrimSpace(h.Get(anthropicUnifiedPrefix+name+"-reset")), 10, 64); err == nil && reset > 0 {
		w.ResetsAt = time.Unix(reset, 0).UTC()
	}
	return w
}

// Apply merges the header reading into a stored usage snapshot, replacing the
// five_hour / seven_day windows it names and leaving everything else (spend,
// scoped breakdowns) as the last poll saw it. It reports whether anything
// changed in a way worth storing: a moved percentage, a new reset time, or a
// changed limit status.
func (u *AnthropicUnified) Apply(usage *quota.ProviderUsage, now time.Time) bool {
	changed := false
	for _, spec := range []struct {
		key     string
		w       *AnthropicUnifiedWindow
		typ     quota.WindowType
		minutes int
		label   string
	}{
		{"five_hour", u.FiveHour, quota.WindowTypeSession, 300, "5-Hour Window"},
		{"seven_day", u.SevenDay, quota.WindowTypeWeekly, 7 * 24 * 60, "7-Day Window"},
	} {
		if spec.w == nil {
			continue
		}
		pct := spec.w.UsedPercent
		var resets *string
		if !spec.w.ResetsAt.IsZero() {
			s := spec.w.ResetsAt.Format(time.RFC3339Nano)
			resets = &s
		}
		next := anthropicPercentWindow(spec.typ, spec.minutes, spec.label, &pct, resets)
		next.Key = spec.key
		rejected := u.Status == "rejected" || spec.w.Status == "rejected"
		next.LimitReached = &rejected
		allowed := !rejected
		next.Allowed = &allowed

		// Drop the stale window and add the fresh one through AddWindow so it
		// gets the same derived semantics as a polled window.
		kept := usage.Windows[:0]
		found := false
		for _, old := range usage.Windows {
			if old != nil && old.Key == spec.key {
				found = true
				if old.UsedPercent != next.UsedPercent || !sameTime(old.ResetsAt, next.ResetsAt) ||
					boolPtrValue(old.LimitReached) != rejected {
					changed = true
				}
				continue
			}
			kept = append(kept, old)
		}
		usage.Windows = kept
		usage.AddWindow(spec.key, next)
		if !found {
			changed = true
		}
	}
	if changed {
		usage.FetchedAt = now
		usage.LastError = ""
		usage.LastErrorAt = nil
	}
	return changed
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func boolPtrValue(b *bool) bool { return b != nil && *b }
