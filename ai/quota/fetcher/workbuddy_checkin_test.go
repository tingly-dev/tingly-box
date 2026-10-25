package fetcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const workbuddySummaryFixture = `{"code":0,"data":{"IsPaidUser":false,"Packages":[
 {"PackageCode":"p","CapacityUnit":"credits","CycleTotalCapacity":"100","CycleRemainCapacity":"60","CycleUsedCapacity":"40"}]}}`

// checkinServer fakes the billing meter with a stateful check-in: the status
// answers statusBody until a claim, then claimedBody; the claim answers
// claimStatus / claimBody and flips the state only when it is a success.
type checkinServer struct {
	*httptest.Server
	statusBody, claimedBody string
	claimStatus             int
	claimBody               string
	claimed                 atomic.Bool
	claims, statuses        atomic.Int32
}

func newCheckinServer(t *testing.T, statusBody string) *checkinServer {
	t.Helper()
	s := &checkinServer{
		statusBody:  statusBody,
		claimedBody: `{"code":0,"data":{"checkedIn":true}}`,
		claimStatus: 200,
		claimBody:   `{"code":0,"data":{}}`,
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case workbuddySummaryPath:
			_, _ = w.Write([]byte(workbuddySummaryFixture))
		case workbuddyCheckinStatusPath:
			s.statuses.Add(1)
			if s.claimed.Load() {
				_, _ = w.Write([]byte(s.claimedBody))
			} else {
				_, _ = w.Write([]byte(s.statusBody))
			}
		case workbuddyCheckinPath:
			s.claims.Add(1)
			if s.claimStatus == 200 && s.claimBody == `{"code":0,"data":{}}` {
				s.claimed.Store(true)
			}
			w.WriteHeader(s.claimStatus)
			_, _ = w.Write([]byte(s.claimBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

const notCheckedIn = `{"code":0,"data":{"checkedIn":false,"consecutiveDays":3}}`

func TestWorkBuddyCheckin_OfferedOnlyWhenClaimable(t *testing.T) {
	for name, tc := range map[string]struct {
		status string
		offer  bool
	}{
		"bool false":          {notCheckedIn, true},
		"state string":        {`{"code":0,"data":{"status":"CLAIMABLE"}}`, true},
		"nested":              {`{"code":0,"data":{"activity":{"todayCheckedIn":false}}}`, true},
		"snake case":          {`{"code":0,"data":{"is_checked_in":false}}`, true},
		"already claimed":     {`{"code":0,"data":{"checkedIn":true}}`, false},
		"claimed state":       {`{"code":0,"data":{"status":"CLAIMED"}}`, false},
		"unrecognised answer": {`{"code":0,"data":{"surprise":1}}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			s := newCheckinServer(t, tc.status)
			usage, err := (&WorkBuddyFetcher{baseURL: s.URL}).Fetch(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddyCN))
			if err != nil || len(usage.Windows) == 0 {
				t.Fatalf("credits must survive: err=%v", err)
			}
			var got *quota.UsageBreakdown
			for _, bd := range usage.Breakdowns {
				if bd.Key == "reward-daily-checkin" {
					got = bd
				}
			}
			if (got != nil) != tc.offer {
				t.Fatalf("offered = %v, want %v", got != nil, tc.offer)
			}
			if got != nil && (got.Redeem == nil || got.Redeem.ID != "daily-checkin" || got.Redeem.Scope != "daily" || !got.Redeem.Auto) {
				t.Errorf("redeem = %+v", got.Redeem)
			}
			if s.claims.Load() != 0 {
				t.Error("a read must never claim")
			}
		})
	}
}

func TestWorkBuddyCheckin_StatusFailureKeepsCredits(t *testing.T) {
	s := newCheckinServer(t, `x`)
	s.statusBody = `<html>`
	usage, err := (&WorkBuddyFetcher{baseURL: s.URL}).Fetch(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddyCN))
	if err != nil || len(usage.Windows) == 0 || len(usage.Breakdowns) != 0 {
		t.Fatalf("err=%v windows=%d breakdowns=%d", err, len(usage.Windows), len(usage.Breakdowns))
	}
}

func TestWorkBuddyCheckin_EnterpriseAccountsAreLeftAlone(t *testing.T) {
	s := newCheckinServer(t, notCheckedIn)
	provider := workbuddyTestProvider(ai.IssuerWorkBuddyCN)
	provider.OAuthDetail.ExtraFields[oauth.WorkBuddyMetaDomain] = "corp.example.com"
	f := &WorkBuddyFetcher{baseURL: s.URL}

	usage, err := f.Fetch(context.Background(), provider)
	if err != nil || len(usage.Breakdowns) != 0 || s.statuses.Load() != 0 {
		t.Fatalf("an enterprise account must not even be asked: err=%v breakdowns=%d statuses=%d", err, len(usage.Breakdowns), s.statuses.Load())
	}
	if _, err := f.Redeem(context.Background(), provider, "daily-checkin"); !errors.Is(err, quota.ErrRedeemUnsupported) {
		t.Fatalf("err = %v, want ErrRedeemUnsupported", err)
	}
	if s.claims.Load() != 0 {
		t.Fatal("nothing may be claimed for an enterprise account")
	}
}

func TestWorkBuddyRedeem_ClaimsOnceAndConfirms(t *testing.T) {
	s := newCheckinServer(t, notCheckedIn)
	msg, err := (&WorkBuddyFetcher{baseURL: s.URL}).Redeem(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddyCN), "daily-checkin")
	if err != nil || msg == "" {
		t.Fatalf("msg=%q err=%v", msg, err)
	}
	if s.claims.Load() != 1 || s.statuses.Load() != 2 {
		t.Fatalf("claims=%d statuses=%d, want one claim between two status reads", s.claims.Load(), s.statuses.Load())
	}
}

func TestWorkBuddyRedeem_NeverClaimsWithoutAClearStatus(t *testing.T) {
	for name, status := range map[string]string{
		"already claimed": `{"code":0,"data":{"checkedIn":true}}`,
		"unrecognised":    `{"code":0,"data":{"surprise":1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := newCheckinServer(t, status)
			_, err := (&WorkBuddyFetcher{baseURL: s.URL}).Redeem(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddyCN), "daily-checkin")
			if err == nil || s.claims.Load() != 0 {
				t.Fatalf("err=%v claims=%d, want a refusal and no claim", err, s.claims.Load())
			}
		})
	}
	s := newCheckinServer(t, notCheckedIn)
	if _, err := (&WorkBuddyFetcher{baseURL: s.URL}).Redeem(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddyCN), "other"); !errors.Is(err, quota.ErrRewardNotFound) {
		t.Fatalf("unknown reward id: err = %v", err)
	}
}

func TestWorkBuddyRedeem_Outcomes(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"repeat check-in":   {400, `{"code":10001,"msg":"already"}`, quota.ErrRewardNotFound},
		"business refusal":  {200, `{"code":5,"msg":"nope"}`, quota.ErrRedeemRejected},
		"client error":      {403, `{}`, quota.ErrRedeemRejected},
		"server error":      {502, `bad gateway`, quota.ErrRedeemUnconfirmed},
		"accepted but gone": {200, `{"code":0,"data":{"queued":true}}`, quota.ErrRedeemUnconfirmed},
	} {
		t.Run(name, func(t *testing.T) {
			s := newCheckinServer(t, notCheckedIn)
			s.claimStatus, s.claimBody = tc.status, tc.body
			_, err := (&WorkBuddyFetcher{baseURL: s.URL}).Redeem(context.Background(), workbuddyTestProvider(ai.IssuerWorkBuddyCN), "daily-checkin")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if s.claims.Load() != 1 {
				t.Fatalf("sent %d times, want exactly once", s.claims.Load())
			}
		})
	}
}
