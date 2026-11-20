package fetcher

import (
	"net/http"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai/quota"
)

func unifiedHeaders(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set("anthropic-ratelimit-unified-"+kv[i], kv[i+1])
	}
	return h
}

func TestParseAnthropicUnifiedHeaders(t *testing.T) {
	got, ok := ParseAnthropicUnifiedHeaders(unifiedHeaders(
		"status", "allowed_warning",
		"5h-utilization", "0.42", "5h-reset", "1786147200", "5h-status", "allowed",
		"7d-utilization", "0.9", "7d-reset", "1786600000", "7d-status", "allowed_warning",
	))
	if !ok {
		t.Fatal("expected a usable reading")
	}
	if got.Status != "allowed_warning" {
		t.Errorf("status = %q", got.Status)
	}
	if got.FiveHour == nil || got.FiveHour.UsedPercent < 41.99 || got.FiveHour.UsedPercent > 42.01 {
		t.Fatalf("5h = %+v, want 42%%", got.FiveHour)
	}
	if want := time.Unix(1786147200, 0).UTC(); !got.FiveHour.ResetsAt.Equal(want) {
		t.Errorf("5h reset = %v, want %v", got.FiveHour.ResetsAt, want)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercent < 89.99 || got.SevenDay.UsedPercent > 90.01 || got.SevenDay.Status != "allowed_warning" {
		t.Errorf("7d = %+v", got.SevenDay)
	}
}

func TestParseAnthropicUnifiedHeaders_NothingUsable(t *testing.T) {
	// Only the overall status: not a window reading, must not invent 0%.
	if _, ok := ParseAnthropicUnifiedHeaders(unifiedHeaders("status", "allowed")); ok {
		t.Error("status alone is not a window reading")
	}
	if _, ok := ParseAnthropicUnifiedHeaders(unifiedHeaders("5h-utilization", "garbage")); ok {
		t.Error("an unparsable utilization is not a reading")
	}
	if _, ok := ParseAnthropicUnifiedHeaders(http.Header{}); ok {
		t.Error("no headers is not a reading")
	}
}

func TestParseAnthropicUnifiedHeaders_FractionBoundary(t *testing.T) {
	got, _ := ParseAnthropicUnifiedHeaders(unifiedHeaders("5h-utilization", "1", "7d-utilization", "0"))
	if got.FiveHour.UsedPercent != 100 {
		t.Errorf("1 is a full window, got %v%%", got.FiveHour.UsedPercent)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercent != 0 {
		t.Errorf("0 is a real 0%%, got %+v", got.SevenDay)
	}
}

func storedClaudeUsage() *quota.ProviderUsage {
	usage := &quota.ProviderUsage{ProviderUUID: "p", ProviderType: quota.ProviderTypeAnthropic, LastError: "stale"}
	reset := "2026-07-29T18:00:00.000000Z"
	p10, p50 := 10.0, 50.0
	usage.AddWindow("five_hour", anthropicPercentWindow(quota.WindowTypeSession, 300, "5-Hour Window", &p10, &reset))
	usage.AddWindow("seven_day", anthropicPercentWindow(quota.WindowTypeWeekly, 7*24*60, "7-Day Window", &p50, &reset))
	addAnthropicExtraUsage(usage, &p10, 12.5, 100, "USD")
	usage.AddBreakdown("fable", "Fable", "model", anthropicPercentWindow(quota.WindowTypeWeekly, 7*24*60, "Fable (weekly)", &p50, &reset))
	return usage
}

func TestAnthropicUnified_ApplyReplacesWindowsAndKeepsTheRest(t *testing.T) {
	usage := storedClaudeUsage()
	reading, _ := ParseAnthropicUnifiedHeaders(unifiedHeaders(
		"status", "allowed", "5h-utilization", "0.30", "5h-reset", "1786147200", "7d-utilization", "0.55", "7d-reset", "1786600000"))
	now := time.Unix(1786000000, 0)

	if !reading.Apply(usage, now) {
		t.Fatal("a moved percentage must report a change")
	}
	five := findWindow(t, usage, "five_hour")
	if five.UsedPercent < 29.99 || five.UsedPercent > 30.01 || five.ResetsAt == nil || five.ResetsAt.Unix() != 1786147200 {
		t.Errorf("five_hour = %+v", five)
	}
	if five.LimitReached == nil || *five.LimitReached {
		t.Errorf("an allowed window is not limit-reached: %+v", five.LimitReached)
	}
	if n := countWindows(usage, "five_hour"); n != 1 {
		t.Errorf("five_hour appears %d times", n)
	}
	// Untouched by a header reading: the spend block and scoped limits.
	findWindow(t, usage, "extra_usage")
	if len(usage.Breakdowns) != 1 {
		t.Errorf("breakdowns = %d, want the polled one kept", len(usage.Breakdowns))
	}
	if usage.FetchedAt != now || usage.LastError != "" {
		t.Errorf("a fresh reading clears the stale error: fetched=%v err=%q", usage.FetchedAt, usage.LastError)
	}

	// The same reading again changes nothing worth a history row.
	if reading.Apply(usage, now.Add(time.Minute)) {
		t.Error("an identical reading must not report a change")
	}
}

func TestAnthropicUnified_RejectedMarksTheWindow(t *testing.T) {
	usage := storedClaudeUsage()
	reading, _ := ParseAnthropicUnifiedHeaders(unifiedHeaders(
		"status", "rejected", "5h-utilization", "1", "5h-reset", "1786147200", "5h-status", "rejected", "7d-utilization", "0.5"))
	if !reading.Apply(usage, time.Unix(1786000000, 0)) {
		t.Fatal("rejection is a change")
	}
	five := findWindow(t, usage, "five_hour")
	if five.LimitReached == nil || !*five.LimitReached || five.Allowed == nil || *five.Allowed {
		t.Errorf("rejected window must say limit reached / not allowed: %+v", five)
	}
}

func countWindows(u *quota.ProviderUsage, key string) int {
	n := 0
	for _, w := range u.Windows {
		if w.Key == key {
			n++
		}
	}
	return n
}
