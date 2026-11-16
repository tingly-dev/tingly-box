package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const glmLimitFixture = `{"code":200,"success":true,"data":{"level":"pro","limits":[
 {"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":40,"nextResetTime":1900000000000}]}}`

func glmProvider() *ai.Provider {
	return &ai.Provider{UUID: "glm-uuid", Name: "GLM", Token: "plan-key", AuthType: ai.AuthTypeAPIKey}
}

// glmServer serves the monitor endpoint plus the reset-card list and use
// endpoints, recording what the use endpoint received.
type glmServer struct {
	*httptest.Server
	list         string
	listStatus   int
	useStatus    int
	useBody      string
	uses         atomic.Int32
	lastUseAuth  atomic.Value
	lastBody     atomic.Value
	lastListAuth atomic.Value
}

func newGLMServer(t *testing.T, list string) *glmServer {
	t.Helper()
	s := &glmServer{list: list, listStatus: 200, useStatus: 200, useBody: `{"code":200,"success":true,"msg":"ok"}`}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/monitor/usage/quota/limit":
			io.WriteString(w, glmLimitFixture)
		case "/api/biz/customer-package-reset/list":
			s.lastListAuth.Store(r.Header.Get("Authorization"))
			w.WriteHeader(s.listStatus)
			io.WriteString(w, s.list)
		case "/api/biz/customer-package-reset/use":
			s.uses.Add(1)
			s.lastUseAuth.Store(r.Header.Get("Authorization"))
			b, _ := io.ReadAll(r.Body)
			s.lastBody.Store(string(b))
			w.WriteHeader(s.useStatus)
			io.WriteString(w, s.useBody)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func future() string { return time.Now().Add(48 * time.Hour).Format("2006-01-02 15:04:05") }

func TestGLMFetch_ShowsAvailableResetCards(t *testing.T) {
	s := newGLMServer(t, `{"code":200,"success":true,"data":[
	 {"recordId":101,"resetType":"WEEK","status":"UNUSED","expireTime":"`+future()+`"},
	 {"recordId":102,"resetType":"FIVE_HOUR","status":"USED","expireTime":"`+future()+`"},
	 {"recordId":103,"resetType":"WEEK","status":"UNUSED","expireTime":"2001-01-01 00:00:00"}]}`)
	usage, err := (&GLMFetcher{baseURL: s.URL}).Fetch(context.Background(), glmProvider())
	if err != nil {
		t.Fatal(err)
	}
	if len(usage.Windows) == 0 {
		t.Fatal("quota windows were dropped")
	}
	bd := findBreakdown(t, usage, "reset-card-101")
	if bd.Group != "resource" || bd.Redeem == nil || bd.Redeem.ID != "101" || bd.Redeem.Scope != "weekly" {
		t.Fatalf("weekly card = %+v redeem=%+v", bd, bd.Redeem)
	}
	if bd.Windows[0].ResetsAt != nil {
		t.Error("a card is spent, not refilled: it must carry no reset time")
	}
	for _, key := range []string{"reset-card-102", "reset-card-103"} {
		for _, other := range usage.Breakdowns {
			if other.Key == key {
				t.Errorf("%s (spent or expired) must not be offered", key)
			}
		}
	}
	if got := s.lastListAuth.Load(); got != "plan-key" {
		t.Errorf("list authorization = %q, want the bare key", got)
	}
}

func TestGLMFetch_CardListFailureKeepsWindows(t *testing.T) {
	for name, tweak := range map[string]func(*glmServer){
		"http error":   func(s *glmServer) { s.listStatus = 500 },
		"unknown data": func(s *glmServer) { s.list = `{"code":200,"success":true,"data":"surprise"}` },
		"refused":      func(s *glmServer) { s.list = `{"code":1001,"success":false,"msg":"no"}` },
	} {
		t.Run(name, func(t *testing.T) {
			s := newGLMServer(t, `[]`)
			tweak(s)
			usage, err := (&GLMFetcher{baseURL: s.URL}).Fetch(context.Background(), glmProvider())
			if err != nil || len(usage.Windows) == 0 {
				t.Fatalf("windows must survive: err=%v windows=%d", err, len(usage.Windows))
			}
		})
	}
}

func TestParseGLMResetCards_Shapes(t *testing.T) {
	now := time.Now()
	ms := now.Add(time.Hour).UnixMilli()
	for name, body := range map[string]string{
		"array":        `{"code":0,"data":[{"id":"7","resetType":"WEEK","expireTime":` + itoa(ms) + `}]}`,
		"list in data": `{"success":true,"data":{"list":[{"recordId":7,"resetType":"WEEK","expireTime":` + itoa(ms) + `}]}}`,
		"records":      `{"code":200,"data":{"records":[{"recordId":7,"type":"WEEK","expiredTime":` + itoa(ms) + `}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			cards, err := parseGLMResetCards([]byte(body), now)
			if err != nil || len(cards) != 1 || !cards[0].Available || cards[0].ID != "7" || cards[0].Scope != "weekly" {
				t.Fatalf("cards=%+v err=%v", cards, err)
			}
		})
	}
	if cards, err := parseGLMResetCards([]byte(`{"code":200,"data":null}`), now); err != nil || len(cards) != 0 {
		t.Errorf("null data is an empty list, got %+v %v", cards, err)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestGLMRedeem_SpendsOnceWithTheListedCard(t *testing.T) {
	s := newGLMServer(t, `{"code":200,"success":true,"data":[{"recordId":101,"resetType":"WEEK","status":"UNUSED","expireTime":"`+future()+`"}]}`)
	msg, err := (&GLMFetcher{baseURL: s.URL}).Redeem(context.Background(), glmProvider(), "101")
	if err != nil {
		t.Fatal(err)
	}
	if msg != "ok" || s.uses.Load() != 1 {
		t.Fatalf("msg=%q uses=%d", msg, s.uses.Load())
	}
	if got := s.lastUseAuth.Load(); got != "plan-key" {
		t.Errorf("use authorization = %q, want the bare key", got)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(s.lastBody.Load().(string)), &body)
	if body["targetType"] != "PERSONAL" || body["resetType"] != "WEEK" || body["recordId"] != float64(101) ||
		body["grantType"] != "DIRECT" || body["requestId"] == "" || body["requestId"] == nil {
		t.Fatalf("use body = %v", body)
	}
}

func TestGLMRedeem_RefusesWhatTheListDoesNotOffer(t *testing.T) {
	s := newGLMServer(t, `{"code":200,"success":true,"data":[{"recordId":101,"resetType":"WEEK","status":"USED"}]}`)
	for _, id := range []string{"101", "999"} {
		_, err := (&GLMFetcher{baseURL: s.URL}).Redeem(context.Background(), glmProvider(), id)
		if !errors.Is(err, quota.ErrRewardNotFound) {
			t.Errorf("id %s: err = %v, want ErrRewardNotFound", id, err)
		}
	}
	if s.uses.Load() != 0 {
		t.Fatal("nothing may be sent for a card that is not available")
	}
}

func TestGLMRedeem_Outcomes(t *testing.T) {
	list := `{"code":200,"success":true,"data":[{"recordId":101,"resetType":"WEEK","status":"UNUSED"}]}`
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"business refusal on HTTP 200": {200, `{"code":500,"success":false,"msg":"already used"}`, quota.ErrRedeemRejected},
		"client error":                 {403, `{}`, quota.ErrRedeemRejected},
		"server error is unknown":      {502, `bad gateway`, quota.ErrRedeemUnconfirmed},
		"unreadable answer is unknown": {200, `<html>`, quota.ErrRedeemUnconfirmed},
	} {
		t.Run(name, func(t *testing.T) {
			s := newGLMServer(t, list)
			s.useStatus, s.useBody = tc.status, tc.body
			_, err := (&GLMFetcher{baseURL: s.URL}).Redeem(context.Background(), glmProvider(), "101")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if s.uses.Load() != 1 {
				t.Fatalf("sent %d times, want exactly once (never retried)", s.uses.Load())
			}
		})
	}
}
