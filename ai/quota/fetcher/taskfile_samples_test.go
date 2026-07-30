package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func serve(t *testing.T, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

// want describes what a sample should normalize to. A zero pct with wantOK
// false means the provider's usage is unknown, which is not a usage of zero.
type want struct {
	pct      float64
	ok       bool
	tightest string // Tightest().Key, empty when usage is unknown
	windows  int    // account-level windows
}

func check(t *testing.T, name string, usage *quota.ProviderUsage, w want) {
	t.Helper()
	usage.NormalizeWindows()
	checkInvariants(t, usage)

	pct, ok := usage.Pct()
	if ok != w.ok || (ok && pct != w.pct) {
		t.Errorf("%s: Pct() = %v, %v; want %v, %v", name, pct, ok, w.pct, w.ok)
	}
	got := ""
	if tight := usage.Tightest(); tight != nil {
		got = tight.Key
	}
	if got != w.tightest {
		t.Errorf("%s: Tightest() = %q; want %q", name, got, w.tightest)
	}
	if len(usage.Windows) != w.windows {
		t.Errorf("%s: %d account windows; want %d", name, len(usage.Windows), w.windows)
	}
}

func oauthProvider(name string) *ai.Provider {
	return &ai.Provider{UUID: "u-" + name, Name: name, AuthType: ai.AuthTypeOAuth,
		Token: "t", OAuthDetail: &ai.OAuthDetail{AccessToken: "t"}}
}

// TestTaskfileSamples pins what the payloads documented in
// build/Taskfile.quota.yml normalize to. Those are the responses the project
// captures from each upstream, so they are the closest thing to production
// input a test can hold — and they cover the shapes hand-written fixtures miss:
// a null utilization, a free plan whose "primary" window is a week long, a plan
// that mixes a coding model with speech and image quotas.
func TestTaskfileSamples(t *testing.T) {
	// ── anthropic (build/Taskfile.quota.yml) ──
	s := serve(t, `{"five_hour":{"utilization":7.0,"resets_at":"2026-04-08T05:00:00.096458+00:00"},
	"seven_day":{"utilization":1.0,"resets_at":"2026-04-14T15:00:01.096478+00:00"},
	"seven_day_sonnet":{"utilization":1.0,"resets_at":"2026-04-14T15:00:01.096485+00:00"},
	"extra_usage":{"is_enabled":true,"monthly_limit":7500,"used_credits":0.0,"utilization":null}}`)
	u, err := (&AnthropicFetcher{baseURL: s.URL}).Fetch(context.Background(), oauthProvider("Claude"))
	if err != nil {
		t.Fatal(err)
	}
	// The add-on is enabled but upstream will not say how much is used, so it
	// does not contribute — and does not report 0%.
	check(t, "anthropic", u, want{pct: 7, ok: true, tightest: "five_hour", windows: 3})

	// ── gemini ──
	s = serve(t, `{"buckets":[{"modelId":"gemini-2.5-pro","remainingFraction":0.75,"resetTime":"2026-04-03T00:00:00Z"},
	{"modelId":"gemini-2.5-flash","remainingFraction":0.90,"resetTime":"2026-04-03T00:00:00Z"}]}`)
	u, err = (&GeminiFetcher{baseURL: s.URL}).Fetch(context.Background(), oauthProvider("Gemini"))
	if err != nil {
		t.Fatal(err)
	}
	// pro at 25% and flash at 10%: the account figure is pro, not the mean.
	check(t, "gemini", u, want{pct: 25, ok: true, tightest: "tightest", windows: 1})

	// ── openrouter (free tier, limit null) ──
	s = serve(t, `{"data":{"label":"sk-or-v1-c1a...5a2","limit":null,"limit_remaining":null,"usage":0,
	"usage_daily":0,"usage_weekly":0,"usage_monthly":0,"is_free_tier":true,"creator_user_id":"user_xxx"}}`)
	u, err = (&OpenRouterFetcher{}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "OpenRouter", Token: "k", APIBase: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	// A free key has no cap at all, so there is no percentage to report.
	check(t, "openrouter", u, want{ok: false, windows: 1})

	// ── codex (fresh) ──
	codexFresh := `{"user_id":"user-j5z","account_id":"user-j5z","plan_type":"prolite",
	"rate_limit":{"allowed":true,"limit_reached":false,
	  "primary_window":{"used_percent":0,"limit_window_seconds":18000,"reset_at":1777051105},
	  "secondary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":1777637905}},
	"code_review_rate_limit":null,
	"additional_rate_limits":[{"limit_name":"GPT-5.3-Codex-Spark","metered_feature":"codex_bengalfox",
	  "rate_limit":{"allowed":true,"limit_reached":false,
	   "primary_window":{"used_percent":0,"limit_window_seconds":18000,"reset_at":1777051105}}}],
	"credits":{"has_credits":false,"unlimited":false,"balance":"0"},
	"spend_control":{"reached":false},"rate_limit_reset_credits":null}`
	s = serve(t, codexFresh)
	u, err = (&CodexFetcher{baseURL: s.URL}).Fetch(context.Background(), oauthProvider("Codex"))
	if err != nil {
		t.Fatal(err)
	}
	check(t, "codex fresh", u, want{pct: 0, ok: true, tightest: "current", windows: 2})

	// ── codex (rate limited) ──
	codexLimited := `{"user_id":"user-I9Ki","plan_type":"free",
	"rate_limit":{"allowed":false,"limit_reached":true,
	  "primary_window":{"used_percent":100,"limit_window_seconds":604800,"reset_at":1783148508},
	  "secondary_window":{"used_percent":87,"limit_window_seconds":604800,"reset_at":1783392195}},
	"additional_rate_limits":[{"limit_name":"GPT-5.3-Codex-Spark","metered_feature":"codex_bengalfox",
	  "rate_limit":{"allowed":true,"limit_reached":false,
	   "primary_window":{"used_percent":0,"limit_window_seconds":18000,"reset_at":1783165565}}}],
	"credits":{"has_credits":false,"unlimited":false,"balance":"0"},
	"rate_limit_reset_credits":{"available_count":4}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/wham/rate-limit-reset-credits" {
			_, _ = w.Write([]byte(`{"credits":[]}`))
			return
		}
		_, _ = w.Write([]byte(codexLimited))
	}))
	defer srv.Close()
	u, err = (&CodexFetcher{baseURL: srv.URL}).Fetch(context.Background(), oauthProvider("Codex"))
	if err != nil {
		t.Fatal(err)
	}
	// Spark is untouched but scoped to its own model, so it neither rescues nor
	// gates the account. The spent primary window does.
	check(t, "codex limited", u, want{pct: 100, ok: true, tightest: "current", windows: 2})
	if got := findWindow(t, u, "current").Type; got != quota.WindowTypeWeekly {
		t.Errorf("codex limited: primary window type = %q; upstream reports 604800s for it", got)
	}

	// ── kimi-code ──
	s = serve(t, `{"user":{"userId":"user-example","membership":{"level":"LEVEL_ADVANCED"}},
	"usage":{"limit":"100","used":"6","remaining":"94","resetTime":"2026-07-27T15:32:57.102261Z"},
	"limits":[{"window":{"duration":300,"timeUnit":"TIME_UNIT_MINUTE"},
	  "detail":{"limit":"100","remaining":"100","resetTime":"2026-07-23T05:32:57.102261Z"}}]}`)
	u, err = (&KimiCodeFetcher{baseURL: s.URL}).Fetch(context.Background(), oauthProvider("Kimi Code"))
	if err != nil {
		t.Fatal(err)
	}
	check(t, "kimi-code", u, want{pct: 6, ok: true, tightest: "weekly", windows: 2})

	// ── kimi-k2 ──
	s = serve(t, `{"consumed":150,"remaining":350}`)
	u, err = (&KimiK2Fetcher{baseURL: s.URL}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "Kimi K2", Token: "k"})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "kimi-k2", u, want{pct: 30, ok: true, tightest: "credits", windows: 1})
	if u.RecoversAt() != nil {
		t.Error("kimi-k2: a credit balance does not come back by waiting")
	}

	// ── minimax (real capture) ──
	// Counts are 0/0 and the remaining percentages carry the whole story.
	// Each model reports its own interval length — 5h for general, 24h for
	// video — alongside the shared week.
	s = serve(t, `{"model_remains":[
	 {"start_time":1785376800000,"end_time":1785394800000,"remains_time":14240499,
	  "current_interval_total_count":0,"current_interval_usage_count":0,"model_name":"general",
	  "current_weekly_total_count":0,"current_weekly_usage_count":0,
	  "weekly_start_time":1785081600000,"weekly_end_time":1785686400000,
	  "current_interval_status":1,"current_interval_remaining_percent":100,
	  "current_weekly_status":3,"current_weekly_remaining_percent":100},
	 {"start_time":1785340800000,"end_time":1785427200000,"remains_time":46640499,
	  "current_interval_total_count":0,"current_interval_usage_count":0,"model_name":"video",
	  "current_weekly_total_count":0,"current_weekly_usage_count":0,
	  "weekly_start_time":1785081600000,"weekly_end_time":1785686400000,
	  "current_interval_status":3,"current_interval_remaining_percent":100,
	  "current_weekly_status":3,"current_weekly_remaining_percent":100}],
	"base_resp":{"status_code":0,"status_msg":"success"}}`)
	u, err = fetchMiniMaxQuota(context.Background(),
		&ai.Provider{UUID: "u", Name: "MiniMax", Token: "k"}, s.URL, quota.ProviderTypeMiniMax)
	if err != nil {
		t.Fatal(err)
	}
	check(t, "minimax", u, want{pct: 0, ok: true, tightest: "interval", windows: 2})
	if got := findWindow(t, u, "interval").WindowMinutes; got != 5*60 {
		t.Errorf("minimax: interval = %d min; want 300, general's own length", got)
	}

	// ── zai (string unit form) ──
	// This form names its unit ("tokens") rather than coding a period, so the
	// window stays unsized and typed custom instead of being given a length we
	// would have had to invent.
	zaiU := zaiUsage(t, `{"code":0,"data":{"planName":"Pro","limits":[
	 {"type":"TOKENS_LIMIT","used":50000,"total":200000,"unit":"tokens","nextResetTime":1743696000000},
	 {"type":"TIME_LIMIT","used":30,"total":120,"unit":"minutes","nextResetTime":1743696000000}]}}`)
	check(t, "zai", zaiU, want{pct: 25, ok: true, tightest: "TOKENS_LIMIT", windows: 1})
	findBreakdown(t, zaiU, "mcp")

	// ── glm (real captured form) ──
	glmU := zaiUsage(t, `{"code":200,"msg":"ok","success":true,"data":{"level":"max","limits":[
	 {"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":0},
	 {"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":100,"nextResetTime":1782717116981},
	 {"type":"TIME_LIMIT","unit":5,"number":1,"usage":4000,"currentValue":63,"remaining":3937,
	  "percentage":1,"nextResetTime":1784704316998,
	  "usageDetails":[{"modelCode":"search-prime","usage":71},{"modelCode":"web-reader","usage":5},
	   {"modelCode":"zread","usage":0}]}]}}`)
	// The weekly token allowance is spent. The monthly MCP limit is scoped to
	// that feature, so it neither gates the account nor softens the verdict,
	// and its per-model detail survives being moved out of the account windows.
	check(t, "glm", glmU, want{pct: 100, ok: true, tightest: "TOKENS_LIMIT_6_1", windows: 2})
	findBreakdown(t, glmU, "mcp")
	findBreakdown(t, glmU, "search-prime")

	// ── openai 404 / copilot ──
	s = serve(t, "")
	srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv404.Close()
	u, err = (&OpenAIFetcher{}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "OpenAI", Token: "k", APIBase: srv404.URL})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "openai 404", u, want{ok: false, windows: 0})

	u, _ = (&CopilotFetcher{}).Fetch(context.Background(), oauthProvider("Copilot"))
	check(t, "copilot", u, want{ok: false, windows: 0})
	if u.LastError == "" {
		t.Error("copilot: the reason should be recorded even though no window is")
	}
}
