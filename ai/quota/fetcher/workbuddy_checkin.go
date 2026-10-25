package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// WorkBuddy's daily check-in: 100 credits a day, plus a bonus for a streak.
// The two paths are the desktop client's own (a third-party tool that drives
// the desktop app calls the same two); they sit under /v2 on the account's
// origin, unlike the credit meter. See .design/provider-rewards.md.
const (
	workbuddyCheckinStatusPath = "/v2/billing/meter/checkin-activity-status"
	workbuddyCheckinPath       = "/v2/billing/meter/daily-checkin"

	// workbuddyCheckinID names the reward in UsageRedeem.ID.
	workbuddyCheckinID = "daily-checkin"

	// workbuddyAlreadyDone is the code a repeat check-in answers (with HTTP 400).
	workbuddyAlreadyDone = "10001"
)

// workbuddyCheckin is what the status answer means for the check-in. A state
// the answer does not clearly give is neither: nothing is claimed on a guess.
type workbuddyCheckin struct {
	Claimed   bool
	Claimable bool
}

// Field spellings are not documented, so the status is read from any of the
// names below, in the answer or one object down.
var (
	workbuddyClaimedKeys   = []string{"checkedin", "ischeckedin", "todaycheckedin", "hascheckedin", "signed", "issigned", "todaysigned", "claimed", "isclaimed", "todayclaimed"}
	workbuddyClaimableKeys = []string{"claimable", "canclaim", "cancheckin", "canbeclaimed"}
	workbuddyStateKeys     = []string{"status", "checkinstatus", "state", "claimstatus"}
)

func readWorkBuddyCheckin(data json.RawMessage) (workbuddyCheckin, bool) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return workbuddyCheckin{}, false
	}
	fields := map[string]any{}
	var collect func(m map[string]any, depth int)
	collect = func(m map[string]any, depth int) {
		for k, v := range m {
			key := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(k))
			if sub, ok := v.(map[string]any); ok {
				if depth < 1 {
					collect(sub, depth+1)
				}
				continue
			}
			if _, seen := fields[key]; !seen {
				fields[key] = v
			}
		}
	}
	collect(root, 0)

	// Each source gives a yes, a no, or nothing; the first that speaks decides.
	claimed, claimable := (*bool)(nil), (*bool)(nil)
	for _, key := range workbuddyClaimedKeys {
		if v, ok := fields[key].(bool); ok {
			claimed = &v
			break
		}
	}
	for _, key := range workbuddyClaimableKeys {
		if v, ok := fields[key].(bool); ok {
			claimable = &v
			break
		}
	}
	for _, key := range workbuddyStateKeys {
		state, ok := fields[key].(string)
		if !ok {
			continue
		}
		yes, no := true, false
		switch strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(state)) {
		case "CLAIMED", "CHECKED_IN", "SIGNED", "DONE", "COMPLETED":
			claimed = &yes
		case "CLAIMABLE", "UNCLAIMED", "AVAILABLE", "PENDING", "NOT_CHECKED_IN", "UNSIGNED":
			claimed, claimable = &no, &yes
		}
		break
	}
	switch {
	case claimed != nil && *claimed:
		return workbuddyCheckin{Claimed: true}, true
	case claimable != nil:
		return workbuddyCheckin{Claimable: *claimable}, true
	case claimed != nil: // explicitly not checked in
		return workbuddyCheckin{Claimable: true}, true
	}
	return workbuddyCheckin{}, false
}

func (f *WorkBuddyFetcher) checkinStatus(ctx context.Context, client *http.Client, site ai.WorkBuddySite, provider *ai.Provider) (workbuddyCheckin, error) {
	var raw json.RawMessage
	if _, err := f.meter(ctx, client, site, provider, workbuddyCheckinStatusPath, map[string]any{}, &raw); err != nil {
		return workbuddyCheckin{}, err
	}
	st, known := readWorkBuddyCheckin(raw)
	if !known {
		return workbuddyCheckin{}, fmt.Errorf("check-in status not recognised")
	}
	return st, nil
}

// addCheckin offers today's check-in when it can be taken. A day already
// checked in shows nothing (the credits are what the user sees), and a status
// that cannot be read is logged and never drops the credits.
func (f *WorkBuddyFetcher) addCheckin(ctx context.Context, client *http.Client, site ai.WorkBuddySite, provider *ai.Provider, usage *quota.ProviderUsage) {
	st, err := f.checkinStatus(ctx, client, site, provider)
	if err != nil {
		logrus.WithError(err).WithField("provider_uuid", provider.UUID).Warn("workbuddy check-in status not read")
		return
	}
	if !st.Claimable {
		return
	}
	bd := usage.AddBreakdown("reward-"+workbuddyCheckinID, "Daily check-in", "resource", &quota.UsageWindow{
		Type:        quota.WindowTypeBalance,
		Used:        0,
		Limit:       1,
		Unit:        quota.UsageUnitCredits,
		Label:       "available",
		Description: "100 credits",
	})
	// Idempotent, and a day not taken is lost: safe to take unasked.
	bd.Redeem = &quota.UsageRedeem{ID: workbuddyCheckinID, Scope: "daily", Auto: true}
}

// Redeem takes today's check-in. Enterprise accounts are never touched, and
// nothing is sent unless the status clearly says it can be taken. The claim is
// sent once; a second status read confirms it, and a claim that cannot be
// confirmed is unconfirmed, never retried.
func (f *WorkBuddyFetcher) Redeem(ctx context.Context, provider *ai.Provider, rewardID string) (string, error) {
	if rewardID != workbuddyCheckinID {
		return "", quota.ErrRewardNotFound
	}
	if err := f.Validate(provider); err != nil {
		return "", err
	}
	if provider.OAuthDetail.GetExtraFieldString(oauth.WorkBuddyMetaDomain) != "" {
		return "", quota.ErrRedeemUnsupported
	}
	site, _ := ai.WorkBuddySiteFor(provider.OAuthIssuer())
	if f.baseURL != "" {
		site.Origin = strings.TrimRight(f.baseURL, "/")
	}
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	st, err := f.checkinStatus(ctx, client, site, provider)
	if err != nil {
		return "", fmt.Errorf("read check-in status before claiming: %w", err)
	}
	if !st.Claimable {
		return "", quota.ErrRewardNotFound
	}

	reply, err := f.post(ctx, client, site, provider, workbuddyCheckinPath, map[string]any{})
	if err != nil {
		return "", fmt.Errorf("%w: %v", quota.ErrRedeemUnconfirmed, err)
	}
	switch {
	case reply.Code == workbuddyAlreadyDone:
		return "", quota.ErrRewardNotFound
	case reply.Status >= 500:
		return "", fmt.Errorf("%w: status %d", quota.ErrRedeemUnconfirmed, reply.Status)
	case reply.Status != http.StatusOK:
		return "", fmt.Errorf("%w: status %d%s", quota.ErrRedeemRejected, reply.Status, errorDetail(reply.Raw))
	case reply.Code != "" && reply.Code != "0":
		return "", fmt.Errorf("%w: %s", quota.ErrRedeemRejected, reply.Msg)
	}

	after, err := f.checkinStatus(ctx, client, site, provider)
	if err != nil || !after.Claimed {
		return "", fmt.Errorf("%w: the claim was accepted but the status does not show it", quota.ErrRedeemUnconfirmed)
	}
	return "Checked in", nil
}

var _ quota.Redeemer = (*WorkBuddyFetcher)(nil)
