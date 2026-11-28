package usage

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/db"
)

func TestUsageEventTypeDefaultAndToolViews(t *testing.T) {
	store, err := db.NewUsageStore(t.TempDir())
	require.NoError(t, err)
	defer store.Close()
	for _, event := range []string{"tokens", "requests"} {
		r := &db.UsageRecord{ProviderUUID: "p", ProviderName: "Provider", Model: "chat", EventType: event, Timestamp: time.Now().Add(-time.Minute)}
		if event == "tokens" {
			r.InputTokens = 12
			r.OutputTokens = 8
		} else {
			r.Model = "deepseek-tools/search"
			r.BillableRequests = 1
			r.RequestCostCNY = .005
		}
		require.NoError(t, store.RecordUsage(r))
	}
	h := NewHandler(store)
	router := gin.New()
	router.GET("/stats", h.GetStats)
	router.GET("/series", h.GetTimeSeries)
	router.GET("/records", h.GetRecords)
	for _, path := range []string{"/stats", "/series", "/records"} {
		for _, query := range []string{"", "?event_type=tokens", "?event_type=requests"} {
			out := httptest.NewRecorder()
			router.ServeHTTP(out, httptest.NewRequest("GET", path+query, nil))
			require.Equal(t, 200, out.Code)
			var result struct {
				Data []map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(out.Body.Bytes(), &result))
			require.Len(t, result.Data, 1)
			row := result.Data[0]
			if query == "?event_type=requests" {
				require.InDelta(t, .005, row["request_cost_cny"], 1e-9)
				require.EqualValues(t, 0, row["total_tokens"])
			} else {
				require.EqualValues(t, 20, row["total_tokens"])
				require.EqualValues(t, 0, row["request_cost_cny"])
			}
			if path != "/records" {
				require.EqualValues(t, 1, row["request_count"])
			}
		}
		for _, invalid := range []string{"all", "bad", ""} {
			out := httptest.NewRecorder()
			router.ServeHTTP(out, httptest.NewRequest("GET", path+"?event_type="+invalid, nil))
			require.Equal(t, 400, out.Code)
		}
	}
}
