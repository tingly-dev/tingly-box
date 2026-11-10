package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func testService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Service{Resolve: func() *typ.Provider {
		return &typ.Provider{UUID: "provider", Token: "provider-secret", APIBase: srv.URL + "/v1"}
	}}
}

func TestSearchMappingAndRendering(t *testing.T) {
	var received map[string]any
	service := testService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tools/search", r.URL.Path)
		require.Equal(t, "Bearer provider-secret", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		fmt.Fprint(w, `{"results":[{"title":"Page","url":"https://example.com","snippet":"short","summary":"model summary","content":"full body","date":"2026-11-10","rerank_score":1.2,"authority":2}]}`)
	})
	recorded := 0
	service.Record = func(_ context.Context, p *typ.Provider, op string, billable bool, _ time.Duration, err error) {
		recorded++
		require.Equal(t, "provider", p.UUID)
		require.Equal(t, "search", op)
		require.True(t, billable)
		require.NoError(t, err)
	}
	args := map[string]any{"query": "golang", "count": float64(5), "allowed_domains": []any{"go.dev", "github.com"}, "blocked_domains": []string{"example.org"}}
	out, err := service.Tool(context.Background(), "search", args)
	require.NoError(t, err)
	require.Equal(t, "golang (site:go.dev OR site:github.com) -site:example.org", received["query"])
	require.EqualValues(t, 5, received["count"])
	require.NotContains(t, received, "ori_question")
	require.Contains(t, out, "model summary")
	require.NotContains(t, out, "full body")
	require.Contains(t, out, "rerank_score: 1.2; authority: 2", "low-quality results must be annotated, not dropped")
	args["need_content"], args["ori_question"] = true, "explicit original question"
	out, err = service.Tool(context.Background(), "search", args)
	require.NoError(t, err)
	require.Contains(t, out, "full body")
	require.Equal(t, "explicit original question", received["ori_question"])
	require.Equal(t, 2, recorded)
}

func TestFetchAndBillingConditions(t *testing.T) {
	for _, tc := range []struct {
		op, body string
		billable bool
	}{
		{"search", `{"results":[]}`, false},
		{"fetch", `{"title":"Page","content":"# Markdown"}`, true},
		{"fetch", `{"title":"Page","content":""}`, false},
	} {
		t.Run(tc.op+tc.body, func(t *testing.T) {
			service := testService(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/tools/"+tc.op, r.URL.Path)
				fmt.Fprint(w, tc.body)
			})
			records := 0
			service.Record = func(_ context.Context, _ *typ.Provider, _ string, billable bool, _ time.Duration, err error) {
				records++
				require.Equal(t, tc.billable, billable)
				require.NoError(t, err)
			}
			_, status, err := service.Call(context.Background(), tc.op, []byte(`{}`))
			require.NoError(t, err)
			require.Equal(t, 200, status)
			require.Equal(t, 1, records)
		})
	}
}

func TestRetryStatusesAndBalance(t *testing.T) {
	for _, status := range []int{400, 401, 402, 403, 404, 408, 413, 429, 500, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			calls, records := 0, 0
			service := testService(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"upstream failure","type":"api_error"}}`)
			})
			service.Record = func(_ context.Context, _ *typ.Provider, _ string, billable bool, _ time.Duration, err error) {
				records++
				require.False(t, billable)
				require.Error(t, err)
			}
			body, got, err := service.Call(context.Background(), "search", []byte(`{"query":"go"}`))
			require.Error(t, err)
			require.Equal(t, status, got)
			require.Contains(t, string(body), "upstream failure")
			expected := 1
			if status == 408 || status == 429 || status >= 500 {
				expected = 3
			}
			require.Equal(t, expected, calls)
			require.Equal(t, 1, records)
			if status == 402 {
				require.Contains(t, err.Error(), "top up your DeepSeek balance")
			}
		})
	}
}

func TestRetryThenSuccessAndCancellation(t *testing.T) {
	calls, records := 0, 0
	service := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"results":[{"title":"Go"}]}`)
	})
	service.Record = func(_ context.Context, _ *typ.Provider, _ string, billable bool, _ time.Duration, err error) {
		records++
		require.True(t, billable)
		require.NoError(t, err)
	}
	_, _, err := service.Call(context.Background(), "search", []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, records)
	service.Record = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = service.Call(ctx, "search", []byte(`{}`))
	require.ErrorIs(t, err, context.Canceled)
}

func TestFetchValidationAndNoRedirectCredentialLeak(t *testing.T) {
	service := testService(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/elsewhere", 302) })
	for _, target := range []string{"file:///etc/passwd", "https://user:password@example.com", "https://"} {
		_, err := service.Tool(context.Background(), "fetch", map[string]any{"url": target})
		require.Error(t, err)
	}
	_, status, err := service.Call(context.Background(), "fetch", []byte(`{"url":"https://example.com"}`))
	require.Error(t, err)
	require.Equal(t, 302, status)
}

func TestMalformedResponseNotRetried(t *testing.T) {
	calls := 0
	service := testService(t, func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "not json") })
	_, status, err := service.Call(context.Background(), "search", []byte(`{}`))
	require.Error(t, err)
	require.Equal(t, 502, status)
	require.Equal(t, 1, calls)
	require.False(t, HasResults("fetch", []byte(`{"content":"   "}`)))
	require.False(t, HasResults("search", []byte(`null`)))
	var unavailable *Service
	require.False(t, unavailable.Available())
	_, _, err = unavailable.Call(context.Background(), "search", nil)
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "provider-secret"))
}

func TestProviderBaseNormalization(t *testing.T) {
	service := testService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tools/search", r.URL.Path)
		fmt.Fprint(w, `{"results":[]}`)
	})
	provider := service.Resolve()
	base := strings.TrimSuffix(provider.APIBase, "/v1")
	for _, path := range []string{"", "/v1/", "/anthropic", "/anthropic/v1"} {
		provider.APIBase = base + path
		service.Resolve = func() *typ.Provider { return provider }
		_, _, err := service.Call(context.Background(), "search", []byte(`{"query":"go"}`))
		require.NoError(t, err)
	}
}
