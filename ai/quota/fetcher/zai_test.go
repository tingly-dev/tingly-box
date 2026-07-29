package fetcher

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func zaiUsage(t *testing.T, response string) *quota.ProviderUsage {
	t.Helper()

	var apiResp zaiQuotaResponse
	if err := json.Unmarshal([]byte(response), &apiResp); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	usage, err := buildZaiProviderUsage(
		&ai.Provider{UUID: "test-uuid", Name: "Z.ai"},
		quota.ProviderTypeZai, response, &apiResp,
	)
	if err != nil {
		t.Fatalf("buildZaiProviderUsage() error: %v", err)
	}
	return usage
}

func findBreakdown(t *testing.T, usage *quota.ProviderUsage, key string) *quota.UsageBreakdown {
	t.Helper()
	for _, bd := range usage.Breakdowns {
		if bd.Key == key {
			return bd
		}
	}
	t.Fatalf("breakdown %q not found", key)
	return nil
}

func TestZai_ModelShareIsNotExhaustion(t *testing.T) {
	// The weekly allowance is barely touched (32k of 100k), but one model
	// accounts for 85% of that consumption. Reporting the share as the
	// model's usage renders it red and reads as nearly exhausted.
	usage := zaiUsage(t, `{"code":0,"success":true,"data":{"level":"pro","limits":[
		{"type":"TOKENS_LIMIT","unit":6,"number":1,"usage":100000,"currentValue":32000,
		 "usageDetails":[{"modelCode":"glm-4.6","usage":27200},{"modelCode":"glm-4.5","usage":4800}]}
	]}}`)

	checkInvariants(t, usage)

	if pct, ok := usage.Pct(); !ok || pct != 32 {
		t.Fatalf("Pct() = %v, %v; want 32, true", pct, ok)
	}

	glm46 := findBreakdown(t, usage, "glm-4.6").Windows[0]
	if math.Abs(glm46.UsedPercent-27.2) > 1e-9 {
		t.Errorf("glm-4.6 UsedPercent = %v; want 27.2 (share of the allowance, not of usage)",
			glm46.UsedPercent)
	}
	if glm46.UsedPercent >= 80 {
		t.Error("a model eating 85% of a 32%-used allowance must not read as nearly exhausted")
	}
}

func TestZai_ModelShareStillVisibleInDescription(t *testing.T) {
	usage := zaiUsage(t, `{"code":0,"success":true,"data":{"level":"pro","limits":[
		{"type":"TOKENS_LIMIT","unit":6,"number":1,"usage":100000,"currentValue":32000,
		 "usageDetails":[{"modelCode":"glm-4.6","usage":27200}]}
	]}}`)

	got := findBreakdown(t, usage, "glm-4.6").Windows[0].Description
	if want := "27200 / 100000 · 85% of usage"; got != want {
		t.Errorf("Description = %q; want %q", got, want)
	}
}

func TestZai_FeatureLimitDoesNotGateTheAccount(t *testing.T) {
	// The MCP allowance is spent, but ordinary requests do not touch it.
	usage := zaiUsage(t, `{"code":0,"success":true,"data":{"level":"pro","limits":[
		{"type":"TOKENS_LIMIT","unit":6,"number":1,"usage":100000,"currentValue":20000},
		{"type":"TIME_LIMIT","unit":3,"number":5,"usage":100,"currentValue":100}
	]}}`)

	checkInvariants(t, usage)

	if pct, ok := usage.Pct(); !ok || pct != 20 {
		t.Fatalf("Pct() = %v, %v; want 20, true — the MCP limit must not gate the account", pct, ok)
	}

	mcp := findBreakdown(t, usage, "mcp")
	if mcp.Group != "feature" {
		t.Errorf("mcp breakdown Group = %q; want feature", mcp.Group)
	}
	if got := mcp.Windows[0].UsedPercent; got != 100 {
		t.Errorf("mcp UsedPercent = %v; want 100 — it is still reported, just not account-wide", got)
	}
}

func TestZai_WindowsCarryTheirPeriod(t *testing.T) {
	usage := zaiUsage(t, `{"code":0,"success":true,"data":{"level":"pro","limits":[
		{"type":"TOKENS_LIMIT","unit":3,"number":5,"usage":1000,"currentValue":100},
		{"type":"TOKENS_LIMIT","unit":6,"number":1,"usage":100000,"currentValue":20000},
		{"type":"TOKENS_LIMIT","unit":5,"number":1,"usage":900000,"currentValue":90000}
	]}}`)

	checkInvariants(t, usage)

	want := map[string]int{
		"TOKENS_LIMIT_3_5": 5 * 60,
		"TOKENS_LIMIT_6_1": 7 * 24 * 60,
		"TOKENS_LIMIT_5_1": 30 * 24 * 60,
	}
	for key, minutes := range want {
		if got := findWindow(t, usage, key).WindowMinutes; got != minutes {
			t.Errorf("%s WindowMinutes = %d; want %d", key, got, minutes)
		}
	}
}

func TestZai_PercentOnlyLimitLeavesModelDetailUnknown(t *testing.T) {
	// Upstream gave a percentage but no totals, so a model's raw token count
	// has nothing to be measured against.
	usage := zaiUsage(t, `{"code":0,"success":true,"data":{"level":"pro","limits":[
		{"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":40,
		 "usageDetails":[{"modelCode":"glm-4.6","usage":27200}]}
	]}}`)

	checkInvariants(t, usage)

	if pct, ok := usage.Pct(); !ok || pct != 40 {
		t.Fatalf("Pct() = %v, %v; want 40, true", pct, ok)
	}
	detail := findBreakdown(t, usage, "glm-4.6").Windows[0]
	if !detail.Unknown {
		t.Error("model detail should be unknown when the parent limit reported no totals")
	}
}

func TestZai_FeatureLimitKeepsItsModelDetail(t *testing.T) {
	// Scoping the MCP limit out of the account windows must not drop the
	// per-model rows that hang off it.
	usage := zaiUsage(t, `{"code":0,"success":true,"data":{"level":"pro","limits":[
		{"type":"TIME_LIMIT","unit":3,"number":5,"usage":100,"currentValue":50,
		 "usageDetails":[{"modelCode":"glm-4.6","usage":30}]}
	]}}`)

	checkInvariants(t, usage)

	findBreakdown(t, usage, "mcp")
	if got := findBreakdown(t, usage, "glm-4.6").Windows[0].Used; got != 30 {
		t.Errorf("glm-4.6 Used = %v; want 30", got)
	}
}
