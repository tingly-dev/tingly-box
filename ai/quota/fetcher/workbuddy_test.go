package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func workbuddyTestProvider(issuer ai.Issuer) *ai.Provider {
	return &ai.Provider{
		UUID:     "workbuddy-uuid",
		Name:     "WorkBuddy",
		Enabled:  true,
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			Issuer:      issuer,
			AccessToken: "wb-access",
			ExtraFields: map[string]interface{}{oauth.WorkBuddyMetaUserID: "u-42"},
		},
	}
}

// workbuddyBilling fakes the billing meter: routes maps a path to the
// envelope it answers; any other path is a 404. Every request is checked for
// the desktop client's identification.
func workbuddyBilling(t *testing.T, domain string, routes map[string]string) (*httptest.Server, map[string]map[string]any) {
	t.Helper()
	bodies := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("%s: method = %s", r.URL.Path, r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer wb-access" || r.Header.Get("X-User-Id") != "u-42" ||
			r.Header.Get("User-Agent") != ai.WorkBuddyUserAgent || r.Header.Get("X-Domain") != domain ||
			r.Header.Get("X-IDE-Type") != "WorkBuddy" {
			t.Errorf("%s: identification headers = %v", r.URL.Path, r.Header)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies[r.URL.Path] = body
		response, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return server, bodies
}

func TestWorkBuddyFetcherCreditsWithReset(t *testing.T) {
	t.Parallel()

	server, bodies := workbuddyBilling(t, "copilot.tencent.com", map[string]string{
		workbuddySummaryPath: `{"code":0,"msg":"ok","data":{
			"IsPaidUser": true,
			"SubscriptionPackageName": "Pro",
			"Packages": [
				{"PackageCode":"TCACA_code_002_AkiJS3ZHF5","CapacityUnit":"credits","CycleTotalCapacity":"1000","CycleRemainCapacity":"750.5","CycleUsedCapacity":"249.5","CycleFrozenCapacity":"3"},
				{"PackageCode":"TCACA_code_001_PqouKr6QWV","CapacityUnit":"credits","CycleTotalCapacity":"50","CycleRemainCapacity":"50","CycleUsedCapacity":"0"},
				{"PackageCode":"other","CapacityUnit":"tokens","CycleTotalCapacity":"99999"}
			]}}`,
		workbuddyPaidPath: `{"code":0,"data":{"Accounts":[
			{"PackageCode":"TCACA_code_002_AkiJS3ZHF5","CapacityUnit":"credits","CycleEndTime":"2026-10-31 23:59:59"},
			{"PackageCode":"TCACA_code_002_AkiJS3ZHF5","CapacityUnit":"credits","CycleEndTime":"2026-09-30 23:59:59"},
			{"PackageCode":"not-counted","CapacityUnit":"credits","CycleEndTime":"2026-10-20 23:59:59"}]}}`,
		// The daily slice ends tonight; it is not the plan's reset.
		workbuddyFreePath: `{"code":0,"data":{"Accounts":[
			{"PackageCode":"TCACA_code_001_PqouKr6QWV","CapacityType":4,"CapacityUnit":"credits","CycleEndTime":"2026-10-17 23:59:59"}]}}`,
	})

	fetcher := &WorkBuddyFetcher{baseURL: server.URL, now: func() time.Time { return time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC) }}
	provider := workbuddyTestProvider(ai.IssuerWorkBuddyCN)
	if err := fetcher.Validate(provider); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if usage.ProviderType != quota.ProviderTypeWorkBuddy || usage.Account == nil || usage.Account.Tier != "Pro" || usage.Account.ID != "u-42" {
		t.Errorf("usage = %+v account = %+v", usage, usage.Account)
	}
	if len(usage.Windows) != 1 {
		t.Fatalf("windows = %d, want one summed Credits meter", len(usage.Windows))
	}
	w := usage.Windows[0]
	if w.Used != 249.5 || w.Limit != 1050 || w.Available == nil || *w.Available != 800.5 || w.Unit != quota.UsageUnitCredits {
		t.Errorf("values = used %v limit %v available %v unit %q", w.Used, w.Limit, w.Available, w.Unit)
	}
	if w.Description != "800.5 / 1050 credits left, 3 reserved" {
		t.Errorf("description = %q", w.Description)
	}
	if w.Type != quota.WindowTypeMonthly || w.Kind != quota.WindowKindLimit {
		t.Errorf("type/kind = %q/%q, want a monthly limit", w.Type, w.Kind)
	}
	// 2026-10-31 23:59:59 CST, plus the second the dashboard adds.
	wantReset := time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC)
	if w.ResetsAt == nil || !w.ResetsAt.Equal(wantReset) {
		t.Errorf("reset = %v, want %v", w.ResetsAt, wantReset)
	}
	free := bodies[workbuddyFreePath]
	if free["SlicePeriodStartTime"] != "2026-10-17 00:00:00" || free["PackageCodes"] == nil {
		t.Errorf("free listing body = %v", free)
	}
}

// A listing that fails keeps the balance; the credits just have no reset.
func TestWorkBuddyFetcherListingFailureKeepsBalance(t *testing.T) {
	t.Parallel()

	server, _ := workbuddyBilling(t, "www.workbuddy.ai", map[string]string{
		workbuddySummaryPath: `{"code":0,"data":{"IsPaidUser":false,"Packages":[{"PackageCode":"gift","CycleTotalCapacity":200,"CycleRemainCapacity":"150"}]}}`,
		workbuddyPaidPath:    `{"code":10001,"msg":"PackageCodes required"}`,
	})
	usage, err := (&WorkBuddyFetcher{baseURL: server.URL}).Fetch(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddy))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	w := usage.Windows[0]
	if w.Type != quota.WindowTypeBalance || w.ResetsAt != nil || w.Used != 50 || w.Limit != 200 {
		t.Errorf("window = %+v, want a 50/200 balance with no reset", w)
	}
	if usage.Account.Tier != "Free" {
		t.Errorf("tier = %q, want Free", usage.Account.Tier)
	}
}

// An enterprise account's summary has no packages; the enterprise meter
// carries its credits.
func TestWorkBuddyFetcherEnterprise(t *testing.T) {
	t.Parallel()

	server, _ := workbuddyBilling(t, "copilot.tencent.com", map[string]string{
		workbuddySummaryPath:    `{"code":0,"data":{"Packages":[]}}`,
		workbuddyEnterprisePath: `{"code":0,"data":{"limitNum":"5000","credit":"1250","cycleStartTime":"2026-10-01 00:00:00","cycleResetTime":"2026-11-01 00:00:00"}}`,
	})
	usage, err := (&WorkBuddyFetcher{baseURL: server.URL}).Fetch(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddyCN))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	w := usage.Windows[0]
	if w.Used != 1250 || w.Limit != 5000 || w.Type != quota.WindowTypeMonthly || w.ResetsAt == nil {
		t.Errorf("window = %+v", w)
	}
	if usage.Account.Tier != "Enterprise" {
		t.Errorf("tier = %q, want Enterprise", usage.Account.Tier)
	}
}

func TestWorkBuddyFetcherRejections(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"in-band code": {http.StatusOK, `{"code":401,"msg":"login expired"}`, "login expired"},
		"http status":  {http.StatusUnauthorized, `{"code":401,"msg":"unauthorized"}`, "401"},
	}
	for name, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := (&WorkBuddyFetcher{baseURL: server.URL}).Fetch(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddy))
		server.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
	if err := NewWorkBuddyFetcher().Validate(kimiCodeTestProvider()); err == nil {
		t.Error("a non-WorkBuddy login must not validate")
	}
}
