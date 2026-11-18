package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// OpenCode's gateway serves two products off one host and one API key:
// pay-as-you-go Zen (a prepaid balance) under /zen/v1, and the Go
// subscription under /zen/go/v1. Only the subscription publishes usage —
// GET /zen/go/v1/usage — and it answers in percentages, one per limit the
// plan enforces: a rolling window, the calendar week, and the billing month.
// The Zen balance has no public endpoint (opencode issue #10448), so a
// balance-only key reads as unreadable rather than as an error.
// See .design/opencode-quota.md.
const (
	opencodeAPIHost   = "https://opencode.ai"
	opencodeUsagePath = "/zen/go/v1/usage"

	// Upstream reports the rolling window's percentage and reset time but not
	// its length, and the length is what orders windows against other
	// providers'. Five hours is the figure the gateway itself names when the
	// window is spent (GoUsageLimitError carries limitName "5 hour"), so it is
	// the plan's documented period rather than a number invented here.
	opencodeRollingWindowMinutes = 5 * 60
	// The billing month runs from the subscription date, not the 1st, so its
	// length varies; 30 days is the nominal period, and ResetsAt carries the
	// real boundary.
	opencodeMonthlyWindowMinutes = 30 * 24 * 60
)

// OpenCodeFetcher retrieves OpenCode Zen / Go quota data.
// Uses: GET https://opencode.ai/zen/go/v1/usage (Zen API key as Bearer).
type OpenCodeFetcher struct{}

func NewOpenCodeFetcher() *OpenCodeFetcher {
	return &OpenCodeFetcher{}
}

func (f *OpenCodeFetcher) Name() string                     { return "opencode" }
func (f *OpenCodeFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeOpenCode }
func (f *OpenCodeFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *OpenCodeFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

// opencodeUsageResponse from GET /zen/go/v1/usage.
//
// Each entry is a percentage and a reset time; upstream never reports the
// dollar amounts behind them, so the plan's limits stay invisible to us.
type opencodeUsageResponse struct {
	Usage struct {
		Rolling *opencodeUsageWindow `json:"rolling"`
		Weekly  *opencodeUsageWindow `json:"weekly"`
		Monthly *opencodeUsageWindow `json:"monthly"`
	} `json:"usage"`
}

type opencodeUsageWindow struct {
	Status   string  `json:"status"`   // "ok" | "rate-limited"
	Percent  float64 `json:"percent"`  // 0-100, floored upstream
	ResetsAt string  `json:"resetsAt"` // RFC3339
}

// opencodeError is the gateway's error envelope, shared by the inference and
// usage endpoints. The type field is what separates a rejected key from a key
// that is simply not on the Go plan.
type opencodeError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (f *OpenCodeFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	// A provider is configured with an inference base — "…/zen/v1" for
	// pay-as-you-go, "…/zen/go/v1" for the subscription — while the usage
	// endpoint is addressed from the host root. Longest suffix first, since
	// "/zen/go/v1" also ends with "/v1".
	url := apiRoot(provider.APIBase, opencodeAPIHost, "/zen/go/v1", "/zen/v1", "/v1") + opencodeUsagePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")

	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 403 EntitlementError is not a failure: it is a working key that
		// happens to bill against the Zen balance, and the balance has no
		// public endpoint. Saying so beats reporting an error the user cannot
		// act on — and beats reporting 0% usage, which would be a lie.
		if resp.StatusCode == http.StatusForbidden && opencodeErrorType(body) == "EntitlementError" {
			return unreadableUsage(provider, quota.ProviderTypeOpenCode,
				"no OpenCode Go subscription; the Zen balance has no usage API"), nil
		}
		return nil, fmt.Errorf("unexpected status code: %d%s", resp.StatusCode, errorDetail(body))
	}

	var apiResp opencodeUsageResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeOpenCode,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  json.RawMessage(body),
	}

	// The gateway checks weekly, then monthly, then rolling, and the first one
	// that is spent rejects the request; all three are therefore account-level
	// gates, and the tightest of them is what binds.
	addOpenCodeWindow(usage, "rolling", apiResp.Usage.Rolling,
		quota.WindowTypeSession, opencodeRollingWindowMinutes, "5h limit")
	addOpenCodeWindow(usage, "weekly", apiResp.Usage.Weekly,
		quota.WindowTypeWeekly, 7*24*60, "Weekly limit")
	addOpenCodeWindow(usage, "monthly", apiResp.Usage.Monthly,
		quota.WindowTypeMonthly, opencodeMonthlyWindowMinutes, "Monthly limit")

	if len(usage.Windows) == 0 {
		usage.MarkUnreadable("usage response carried no windows", now)
	}
	return usage, nil
}

// addOpenCodeWindow appends one plan limit. Upstream gives a percentage and
// nothing else, so the window is expressed on the 0-100 scale rather than
// pretending to know the dollar cap behind it.
func addOpenCodeWindow(usage *quota.ProviderUsage, key string, w *opencodeUsageWindow,
	windowType quota.WindowType, windowMinutes int, label string) {
	if w == nil {
		return
	}

	window := &quota.UsageWindow{
		Type:          windowType,
		Kind:          quota.WindowKindLimit, // recovers on its own; see Pct(WindowKindLimit)
		Used:          w.Percent,
		Limit:         100,
		UsedPercent:   w.Percent,
		Unit:          quota.UsageUnitPercent,
		WindowMinutes: windowMinutes,
		Label:         label,
		Description:   fmt.Sprintf("%.0f%% of the Go %s used", w.Percent, strings.ToLower(label)),
	}
	if resetsAt, err := time.Parse(time.RFC3339, w.ResetsAt); err == nil {
		window.ResetsAt = &resetsAt
	}
	// "rate-limited" is the gateway's own verdict on this window; it is the
	// only signal that says requests are being refused right now, since a
	// floored 100% can also mean "very nearly spent".
	if w.Status != "" {
		limitReached := w.Status == "rate-limited"
		allowed := !limitReached
		window.LimitReached = &limitReached
		window.Allowed = &allowed
	}

	usage.AddWindow(key, window)
}

// opencodeErrorType reads the error type out of the gateway's envelope.
func opencodeErrorType(body []byte) string {
	var envelope opencodeError
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Error.Type
}
