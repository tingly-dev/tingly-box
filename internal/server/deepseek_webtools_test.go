package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/config"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestDeepSeekProviderResolution(t *testing.T) {
	providers := []*typ.Provider{
		nil,
		{UUID: "disabled", Enabled: false, APIBase: "https://api.deepseek.com/v1", Token: "key"},
		{UUID: "third-party", Name: "DeepSeek", Enabled: true, APIBase: "https://other.example/v1", Token: "key"},
		{UUID: "fake-host", Enabled: true, APIBase: "https://api.deepseek.com.evil.example/v1", Token: "key"},
		{UUID: "oauth", Enabled: true, APIBase: "https://api.deepseek.com/v1", AuthType: typ.AuthTypeOAuth, Token: "key"},
		{UUID: "no-key", Enabled: true, APIBase: "https://api.deepseek.com/v1"},
	}
	require.Nil(t, resolveDeepSeekProvider(providers))
	providers = append(providers, &typ.Provider{UUID: "b", Enabled: true, APIBase: "https://api.deepseek.com/anthropic", Token: "b-key"})
	require.Equal(t, "b", resolveDeepSeekProvider(providers).UUID)
	providers = append(providers, &typ.Provider{UUID: "a", Enabled: true, APIBase: "https://other.example", APIBaseOpenAI: "https://api.deepseek.com/v1", Token: "a-key"})
	require.Equal(t, "a", resolveDeepSeekProvider(providers).UUID)
	require.Equal(t, "disabled", providers[1].UUID, "resolver must not sort the caller's configuration in place")
}

func TestDeepSeekToolUsageRecordsPriceAndAttribution(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	defer cfg.StoreManager().Close()
	s := &Server{config: cfg}
	service := s.newDeepSeekWebtools()
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			fmt.Fprint(w, `{"results":[{"title":"result"}]}`)
		case 2:
			fmt.Fprint(w, `{"results":[]}`)
		default:
			w.WriteHeader(402)
			fmt.Fprint(w, `{"error":{"message":"balance"}}`)
		}
	}))
	defer upstream.Close()
	service.Resolve = func() *typ.Provider {
		return &typ.Provider{UUID: "p", Name: "DeepSeek", APIBase: upstream.URL, Token: "key"}
	}
	ctx := coretool.WithUsageIdentity(context.Background(), coretool.UsageIdentity{UserID: "alice", TeamID: "team", Scenario: "codex", RuleUUID: "rule"})
	for i := 0; i < 3; i++ {
		_, _, _ = service.Call(ctx, "search", []byte(`{"query":"go"}`))
	}
	records, total, err := s.UsageStore().GetRecords(time.Time{}, time.Time{}, nil, 100, 0)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	var cost float64
	var billable int64
	errors := 0
	for _, record := range records {
		require.Equal(t, "requests", record.EventType)
		require.Equal(t, "CNY", record.Currency)
		require.Equal(t, 0.005, record.UnitPrice)
		require.Equal(t, "alice", record.UserID)
		require.Equal(t, "team", record.TeamID)
		require.Equal(t, "rule", record.RuleUUID)
		require.Equal(t, "codex", record.Scenario)
		require.Zero(t, record.TotalTokens)
		cost += record.RequestCostCNY
		billable += record.BillableRequests
		if record.Status == "error" {
			errors++
		}
	}
	require.InDelta(t, 0.005, cost, 0.000001)
	require.EqualValues(t, 1, billable)
	require.Equal(t, 1, errors)
}
