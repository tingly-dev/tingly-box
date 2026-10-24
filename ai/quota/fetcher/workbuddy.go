package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// WorkBuddy's billing meter, on the same origin as the rest of the API for
// both regions and with no /v2 prefix. See .design/workbuddy-oauth.md#quota.
const (
	workbuddySummaryPath    = "/billing/meter/get-user-resource-summary"
	workbuddyEnterprisePath = "/billing/meter/get-enterprise-user-usage"
	workbuddyPaidPath       = "/billing/meter/get-user-resource-paid-packages"
	workbuddyFreePath       = "/billing/meter/get-user-resource-free-packages"

	// workbuddyResetBudget bounds the optional package listings that only
	// supply the reset time, so a slow listing never holds up the balance.
	workbuddyResetBudget = 5 * time.Second
)

// The package listings refuse a request without the package codes to list
// (code 10001 "PackageCodes required"). These are the codes the WorkBuddy
// desktop client sends; a package added later is not considered for the reset.
var (
	workbuddyPaidPackageCodes = []string{
		"TCACA_code_002_AkiJS3ZHF5", "TCACA_code_005_maRGyrHhw1", "TCACA_code_003_FAnt7lcmRT",
		"TCACA_code_023_4xbGhMrE6q", "TCACA_code_026_BaESVICNoi", "TCACA_code_027_0FCGVA6vSa",
		"TCACA_code_009_0XmEQc2xOf", "TCACA_code_038_OhvqZtiPKr", "TCACA_code_036_lupO5WgNdG",
	}
	workbuddyFreePackageCodes = []string{
		workbuddyDailyPackageCode, "TCACA_code_008_cfWoLwvjU4", "TCACA_code_035_ArVxJcGDsm",
		"TCACA_code_006_DbXS0lrypC", "TCACA_code_039_KRcQj7wUat", "TCACA_code_040_mi9rCYg46x",
		"TCACA_code_007_nzdH5h4Nl0", "TCACA_code_028_NtpWi0jzXs", "TCACA_code_037_WxOD3MpI2o",
		"TCACA_code_029_6wCGEWquYy", "TCACA_code_030_BjSt89qTvr",
	}
)

// The daily slice package (and any package of CapacityType 4) refills every
// midnight; its cycle end is not when the plan's credits reset.
const (
	workbuddyDailyPackageCode = "TCACA_code_001_PqouKr6QWV"
	workbuddySliceCapacity    = "4"
)

// workbuddyCycleLayout is how the billing API writes times: zoneless, in
// China Standard Time for both regions.
const workbuddyCycleLayout = "2006-01-02 15:04:05"

var workbuddyCycleZone = time.FixedZone("CST", 8*60*60)

// WorkBuddyFetcher reads the credits of a WorkBuddy / CodeBuddy login, for
// both regions: the provider's issuer picks the origin (ai.WorkBuddySiteFor).
type WorkBuddyFetcher struct {
	baseURL string // empty means the issuer's origin; overridden by tests only
	now     func() time.Time
}

func NewWorkBuddyFetcher() *WorkBuddyFetcher {
	return &WorkBuddyFetcher{}
}

func (f *WorkBuddyFetcher) Name() string                     { return "workbuddy" }
func (f *WorkBuddyFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeWorkBuddy }
func (f *WorkBuddyFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeOAuth }

func (f *WorkBuddyFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.AuthType != ai.AuthTypeOAuth || !ai.IsWorkBuddyIssuer(provider.OAuthIssuer()) {
		return fmt.Errorf("WorkBuddy credits require a WorkBuddy login")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no access token available")
	}
	if provider.IsOAuthExpired() {
		return fmt.Errorf("OAuth token is expired")
	}
	return nil
}

// workbuddyAmount reads a capacity, sent as a decimal string ("438.88") or
// now and then a number.
type workbuddyAmount struct {
	Value float64
	Valid bool
}

func (a *workbuddyAmount) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}
	text := string(data)
	if data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		if strings.TrimSpace(text) == "" {
			return nil
		}
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return fmt.Errorf("capacity %s: %w", data, err)
	}
	a.Value, a.Valid = value, true
	return nil
}

type workbuddyPackage struct {
	PackageCode         string          `json:"PackageCode"`
	CapacityUnit        string          `json:"CapacityUnit"`
	CycleTotalCapacity  workbuddyAmount `json:"CycleTotalCapacity"`
	CycleRemainCapacity workbuddyAmount `json:"CycleRemainCapacity"`
	CycleUsedCapacity   workbuddyAmount `json:"CycleUsedCapacity"`
	CycleFrozenCapacity workbuddyAmount `json:"CycleFrozenCapacity"`
}

type workbuddySummary struct {
	Packages                []workbuddyPackage `json:"Packages"`
	SubscriptionPackageName string             `json:"SubscriptionPackageName"`
	IsPaidUser              bool               `json:"IsPaidUser"`
}

// workbuddyEnterpriseUsage is the meter of an enterprise (team) account,
// whose resource summary lists no packages at all.
type workbuddyEnterpriseUsage struct {
	LimitNum       workbuddyAmount `json:"limitNum"`
	Credit         workbuddyAmount `json:"credit"`
	CycleStartTime string          `json:"cycleStartTime"`
	CycleResetTime string          `json:"cycleResetTime"`
}

type workbuddyPackageAccount struct {
	PackageCode  string          `json:"PackageCode"`
	CapacityUnit string          `json:"CapacityUnit"`
	CapacityType json.RawMessage `json:"CapacityType"`
	CycleEndTime string          `json:"CycleEndTime"`
}

func (f *WorkBuddyFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	site, ok := ai.WorkBuddySiteFor(provider.OAuthIssuer())
	if !ok {
		return nil, fmt.Errorf("provider is not a WorkBuddy login")
	}
	if f.baseURL != "" {
		site.Origin = strings.TrimRight(f.baseURL, "/")
	}
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)
	now := time.Now()
	if f.now != nil {
		now = f.now()
	}

	var summary workbuddySummary
	raw, err := f.meter(ctx, client, site, provider, workbuddySummaryPath, map[string]any{}, &summary)
	if err != nil {
		return nil, err
	}

	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeWorkBuddy,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  raw,
	}
	plan := summary.SubscriptionPackageName
	if plan == "" {
		plan = "Free"
		if summary.IsPaidUser {
			plan = "Pro"
		}
	}

	enterprise := provider.OAuthDetail.GetExtraFieldString(oauth.WorkBuddyMetaDomain) != ""
	window, counted := summaryWindow(summary.Packages)
	if window == nil {
		// An enterprise account's summary has no packages; its credits are
		// the enterprise meter's. A personal account's refusal of that meter
		// ("uid or enterpriseID is empty") leaves the summary's answer.
		var ent workbuddyEnterpriseUsage
		if _, err := f.meter(ctx, client, site, provider, workbuddyEnterprisePath, map[string]any{}, &ent); err == nil {
			window = enterpriseWindow(ent)
			if window != nil {
				enterprise = true
			}
			if window != nil && summary.SubscriptionPackageName == "" {
				plan = "Enterprise"
			}
		}
	} else if reset := f.nextReset(ctx, client, site, provider, counted, now); reset != nil {
		window.Type = quota.WindowTypeMonthly
		window.Kind = quota.WindowKindLimit
		window.ResetsAt = reset
	}
	usage.Account = &quota.UsageAccount{
		ID:   provider.OAuthDetail.GetExtraFieldString(oauth.WorkBuddyMetaUserID),
		Tier: plan,
	}
	if window == nil {
		return unreadableUsage(provider, quota.ProviderTypeWorkBuddy, "account reports no credits"), nil
	}
	usage.AddWindow("credits", window)
	if !enterprise {
		f.addCheckin(ctx, client, site, provider, usage)
	}
	return usage, nil
}

// summaryWindow sums the account's credit packages into the one meter
// WorkBuddy's own usage page shows. counted holds the codes it summed, so
// the reset lookup only considers those packages.
func summaryWindow(packages []workbuddyPackage) (*quota.UsageWindow, map[string]bool) {
	var total, remaining, used, frozen float64
	usedKnown := true
	counted := make(map[string]bool)
	for _, p := range packages {
		if p.CapacityUnit != "" && p.CapacityUnit != "credits" {
			continue
		}
		if !p.CycleTotalCapacity.Valid {
			continue
		}
		counted[p.PackageCode] = true
		total += p.CycleTotalCapacity.Value
		remaining += p.CycleRemainCapacity.Value
		frozen += p.CycleFrozenCapacity.Value
		if p.CycleUsedCapacity.Valid {
			used += p.CycleUsedCapacity.Value
		} else {
			usedKnown = false
		}
	}
	if total <= 0 {
		return nil, nil
	}
	if !usedKnown {
		used = max(0, total-remaining)
	}
	description := fmt.Sprintf("%s / %s credits left", formatCredits(remaining), formatCredits(total))
	if frozen > 0 {
		// Held by requests still running: not spendable yet, not spent yet.
		description += fmt.Sprintf(", %s reserved", formatCredits(frozen))
	}
	available := remaining
	return &quota.UsageWindow{
		// Without a known cycle end the credits are shown as a balance; a
		// reset found in the package listings makes them a monthly limit.
		Type:        quota.WindowTypeBalance,
		Kind:        quota.WindowKindResource,
		Used:        used,
		Limit:       total,
		Available:   &available,
		Unit:        quota.UsageUnitCredits,
		Label:       "Credits",
		Description: description,
	}, counted
}

func enterpriseWindow(u workbuddyEnterpriseUsage) *quota.UsageWindow {
	if !u.LimitNum.Valid || u.LimitNum.Value <= 0 {
		return nil
	}
	limit, used := u.LimitNum.Value, u.Credit.Value
	available := max(0, limit-used)
	window := &quota.UsageWindow{
		Type:        quota.WindowTypeBalance,
		Kind:        quota.WindowKindResource,
		Used:        used,
		Limit:       limit,
		Available:   &available,
		Unit:        quota.UsageUnitCredits,
		Label:       "Credits",
		Description: fmt.Sprintf("%s / %s credits used", formatCredits(used), formatCredits(limit)),
	}
	if reset := parseWorkBuddyTime(u.CycleResetTime); reset != nil {
		window.Type = quota.WindowTypeMonthly
		window.Kind = quota.WindowKindLimit
		window.ResetsAt = reset
		if start := parseWorkBuddyTime(u.CycleStartTime); start != nil && reset.After(*start) {
			window.WindowMinutes = int(reset.Sub(*start).Minutes())
			window.Type = windowTypeForMinutes(window.WindowMinutes)
		}
	}
	return window
}

// nextReset is the earliest future cycle end among the counted packages, from
// the paid and free listings. It is optional: a listing that fails or runs
// past the budget leaves the reset unknown and the balance as it is.
func (f *WorkBuddyFetcher) nextReset(ctx context.Context, client *http.Client, site ai.WorkBuddySite, provider *ai.Provider, counted map[string]bool, now time.Time) *time.Time {
	ctx, cancel := context.WithTimeout(ctx, workbuddyResetBudget)
	defer cancel()

	day := now.In(workbuddyCycleZone).Format("2006-01-02")
	listings := []struct {
		path string
		body map[string]any
	}{
		{workbuddyPaidPath, map[string]any{"PackageCodes": workbuddyPaidPackageCodes, "NeedRenewInfo": true}},
		{workbuddyFreePath, map[string]any{"PackageCodes": workbuddyFreePackageCodes,
			"SlicePeriodStartTime": day + " 00:00:00", "SlicePeriodEndTime": day + " 23:59:59"}},
	}
	var earliest *time.Time
	for _, l := range listings {
		l.body["PageNumber"], l.body["PageSize"], l.body["Status"] = 1, 200, []int{0, 3}
		var page struct {
			Accounts []workbuddyPackageAccount `json:"Accounts"`
		}
		if _, err := f.meter(ctx, client, site, provider, l.path, l.body, &page); err != nil {
			continue
		}
		for _, a := range page.Accounts {
			if !counted[a.PackageCode] || a.PackageCode == workbuddyDailyPackageCode ||
				strings.Trim(string(a.CapacityType), `"`) == workbuddySliceCapacity ||
				(a.CapacityUnit != "" && a.CapacityUnit != "credits") {
				continue
			}
			end := parseWorkBuddyTime(a.CycleEndTime)
			if end == nil {
				continue
			}
			// The end is inclusive; the dashboard resets one second after it.
			reset := end.Add(time.Second)
			if reset.After(now) && (earliest == nil || reset.Before(*earliest)) {
				earliest = &reset
			}
		}
	}
	return earliest
}

// workbuddyReply is one answer of the billing API, decoded no further than its
// {code, msg, data} envelope.
type workbuddyReply struct {
	Status int
	Code   string
	Msg    string
	Data   json.RawMessage
	Raw    json.RawMessage
}

// post sends a request to the billing API as the desktop client does. Only a
// request that could not be made or answered is an error; what the answer says
// is the caller's to read, because a refusal can also be a result (a repeat
// check-in answers HTTP 400 / code 10001).
func (f *WorkBuddyFetcher) post(ctx context.Context, client *http.Client, site ai.WorkBuddySite, provider *ai.Provider, path string, body any) (*workbuddyReply, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, site.Origin+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for k, v := range site.ClientHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("X-User-Id", provider.OAuthDetail.GetExtraFieldString(oauth.WorkBuddyMetaUserID))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var env struct {
		Code json.Number     `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &env)
	return &workbuddyReply{Status: resp.StatusCode, Code: env.Code.String(), Msg: env.Msg, Data: env.Data, Raw: raw}, nil
}

// meter POSTs to the billing API and decodes the data of its envelope into
// out. It returns the raw body.
func (f *WorkBuddyFetcher) meter(ctx context.Context, client *http.Client, site ai.WorkBuddySite, provider *ai.Provider, path string, body any, out any) (json.RawMessage, error) {
	reply, err := f.post(ctx, client, site, provider, path, body)
	if err != nil {
		return nil, err
	}
	if reply.Status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d%s", reply.Status, errorDetail(reply.Raw))
	}
	if reply.Code != "" && reply.Code != "0" {
		return nil, fmt.Errorf("billing API error %s: %s", reply.Code, reply.Msg)
	}
	if len(reply.Data) == 0 || string(reply.Data) == "null" {
		return nil, fmt.Errorf("billing API returned no data")
	}
	if err := json.Unmarshal(reply.Data, out); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return reply.Raw, nil
}

func parseWorkBuddyTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if t, err := time.ParseInLocation(workbuddyCycleLayout, value, workbuddyCycleZone); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return &t
	}
	return nil
}

// formatCredits writes a count whole, else to two places.
func formatCredits(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}
