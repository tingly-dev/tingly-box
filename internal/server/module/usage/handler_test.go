package usage

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/db"
)

// mockUsageStore is a mock implementation of UsageStore for testing
type mockUsageStore struct {
	stats          []db.AggregatedStat
	timeSeriesData []db.TimeSeriesData
	records        []db.UsageRecord
	statsErr       error
	timeSeriesErr  error
	recordsErr     error
}

func (m *mockUsageStore) GetAggregatedStats(query db.UsageStatsQuery) ([]db.AggregatedStat, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.stats, nil
}

func (m *mockUsageStore) GetTimeSeries(startTime, endTime time.Time, interval string, filters map[string]string) ([]db.TimeSeriesData, error) {
	if m.timeSeriesErr != nil {
		return nil, m.timeSeriesErr
	}
	return m.timeSeriesData, nil
}

func (m *mockUsageStore) GetUsageRecords(query UsageRecordsQuery) ([]db.UsageRecord, int64, error) {
	if m.recordsErr != nil {
		return nil, 0, m.recordsErr
	}
	return m.records, int64(len(m.records)), nil
}

func (m *mockUsageStore) DeleteOldRecords(cutoffDate time.Time) (int64, error) {
	return 0, nil
}

func setupTestRouter(store *mockUsageStore) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	usageStore := &db.UsageStore{} // We'll use mock pattern in handler
	_ = NewHandler(usageStore)
	// Override the store for testing (in real scenario, use interface)
	router.GET("/usage/stats", func(c *gin.Context) {
		if store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Usage store not available"})
			return
		}
		query := db.UsageStatsQuery{
			GroupBy:   c.DefaultQuery("group_by", "model"),
			Limit:     100,
			SortBy:    c.DefaultQuery("sort_by", "total_tokens"),
			SortOrder: c.DefaultQuery("sort_order", "desc"),
			StartTime: parseTimeQuery(c, "start_time", time.Now().Add(-24*time.Hour)),
			EndTime:   parseTimeQuery(c, "end_time", time.Now()),
		}
		stats, _ := store.GetAggregatedStats(query)
		data := make([]AggregatedStat, len(stats))
		for i, s := range stats {
			data[i] = AggregatedStat{
				Key:             s.Key,
				ProviderUUID:    s.ProviderUUID,
				ProviderName:    s.ProviderName,
				Model:           s.Model,
				Scenario:        s.Scenario,
				UserID:          s.UserID,
				RequestCount:    s.RequestCount,
				TotalTokens:     s.TotalTokens,
				InputTokens:     s.InputTokens,
				OutputTokens:    s.OutputTokens,
				AvgInputTokens:  s.AvgInputTokens,
				AvgOutputTokens: s.AvgOutputTokens,
				AvgLatencyMs:    s.AvgLatencyMs,
				ErrorCount:      s.ErrorCount,
				ErrorRate:       s.ErrorRate,
				StreamedCount:   s.StreamedCount,
				StreamedRate:    s.StreamedRate,
				CacheReadTokens: s.CacheReadTokens,
			}
		}
		c.JSON(http.StatusOK, UsageStatsResponse{
			Meta: UsageStatsMeta{
				StartTime:  query.StartTime.Format(time.RFC3339),
				EndTime:    query.EndTime.Format(time.RFC3339),
				GroupBy:    query.GroupBy,
				TotalCount: len(data),
			},
			Data: data,
		})
	})

	return router
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler(nil)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestParseTimeQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		result := parseTimeQuery(c, "time", time.Unix(100, 0))
		c.JSON(200, gin.H{"timestamp": result.Unix()})
	})

	// Test with valid ISO8601 time
	req, _ := http.NewRequest("GET", "/test?time=2024-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestParseIntQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/test", func(c *gin.Context) {
		result := parseIntQuery(c, "value", 10)
		c.JSON(200, gin.H{"value": result})
	})

	// Test with valid integer
	req, _ := http.NewRequest("GET", "/test?value=42", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Test with invalid integer (should use default)
	req2, _ := http.NewRequest("GET", "/test?value=invalid", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w2.Code)
	}
}

func TestGetStats_Success(t *testing.T) {
	mockStore := &mockUsageStore{
		stats: []db.AggregatedStat{
			{
				Key:          "gpt-4",
				RequestCount: 100,
				TotalTokens:  50000,
			},
		},
	}
	router := setupTestRouter(mockStore)

	req, _ := http.NewRequest("GET", "/usage/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	// Handler returns the structured UsageStatsResponse directly (meta + data),
	// not a {success: true, ...} envelope.
	assert.Contains(t, body, `"meta"`)
	assert.Contains(t, body, `"data"`)
	assert.Contains(t, body, "gpt-4")
}

func TestGetStats_NoStore(t *testing.T) {
	handler := NewHandler(nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/usage/stats", handler.GetStats)

	req, _ := http.NewRequest("GET", "/usage/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestUsageStatsResponseStructure(t *testing.T) {
	meta := UsageStatsMeta{
		StartTime:  "2024-01-01T00:00:00Z",
		EndTime:    "2024-01-02T00:00:00Z",
		GroupBy:    "model",
		TotalCount: 1,
	}

	data := []AggregatedStat{
		{
			Key:          "gpt-4",
			RequestCount: 100,
			TotalTokens:  50000,
		},
	}

	response := UsageStatsResponse{
		Meta: meta,
		Data: data,
	}

	if response.Meta.GroupBy != "model" {
		t.Errorf("expected GroupBy 'model', got %q", response.Meta.GroupBy)
	}

	if len(response.Data) != 1 {
		t.Errorf("expected 1 data item, got %d", len(response.Data))
	}

	if response.Data[0].Key != "gpt-4" {
		t.Errorf("expected Key 'gpt-4', got %q", response.Data[0].Key)
	}
}

func TestTimeSeriesResponseStructure(t *testing.T) {
	meta := TimeSeriesMeta{
		Interval:  "hour",
		StartTime: "2024-01-01T00:00:00Z",
		EndTime:   "2024-01-02T00:00:00Z",
	}

	data := []TimeSeriesData{
		{
			Timestamp:    "2024-01-01T00:00:00Z",
			RequestCount: 10,
			TotalTokens:  5000,
		},
	}

	response := TimeSeriesResponse{
		Meta: meta,
		Data: data,
	}

	if response.Meta.Interval != "hour" {
		t.Errorf("expected Interval 'hour', got %q", response.Meta.Interval)
	}

	if len(response.Data) != 1 {
		t.Errorf("expected 1 data item, got %d", len(response.Data))
	}
}

func TestUsageRecordsResponseStructure(t *testing.T) {
	meta := UsageRecordsMeta{
		Total:  100,
		Limit:  50,
		Offset: 0,
	}

	data := []UsageRecordResponse{
		{
			ID:          1,
			Model:       "gpt-4",
			InputTokens: 1000,
			Status:      "success",
		},
	}

	response := UsageRecordsResponse{
		Meta: meta,
		Data: data,
	}

	if response.Meta.Total != 100 {
		t.Errorf("expected Total 100, got %d", response.Meta.Total)
	}

	if len(response.Data) != 1 {
		t.Errorf("expected 1 data item, got %d", len(response.Data))
	}
}

// Helper functions for testing
func TestParseTimeQuery_Helper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		queryVal string
		wantSec  int64
	}{
		{
			name:     "valid ISO8601",
			queryVal: "2024-01-01T00:00:00Z",
			wantSec:  1704067200,
		},
		{
			name:     "empty string uses default",
			queryVal: "",
			wantSec:  100, // default value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			if tt.queryVal != "" {
				c.Request = httptest.NewRequest("GET", "/?time="+tt.queryVal, nil)
			} else {
				c.Request = httptest.NewRequest("GET", "/", nil)
			}

			result := parseTimeQuery(c, "time", time.Unix(100, 0))
			if result.Unix() != tt.wantSec {
				t.Errorf("parseTimeQuery() = %v, want %v", result.Unix(), tt.wantSec)
			}
		})
	}
}

func TestParseIntQuery_Helper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		queryVal   string
		defaultVal int
		want       int
	}{
		{
			name:       "valid integer",
			queryVal:   "42",
			defaultVal: 10,
			want:       42,
		},
		{
			name:       "invalid integer uses default",
			queryVal:   "abc",
			defaultVal: 10,
			want:       10,
		},
		{
			name:       "empty string uses default",
			queryVal:   "",
			defaultVal: 10,
			want:       10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			if tt.queryVal != "" {
				c.Request = httptest.NewRequest("GET", "/?value="+tt.queryVal, nil)
			} else {
				c.Request = httptest.NewRequest("GET", "/", nil)
			}

			result := parseIntQuery(c, "value", tt.defaultVal)
			if result != tt.want {
				t.Errorf("parseIntQuery() = %v, want %v", result, tt.want)
			}
		})
	}
}

// TestReasoningTokensSurviveHandlerConversion exercises the real Handler
// (backed by a real db.UsageStore, not the mock) end to end, verifying
// ReasoningTokens survives the db.X -> usage.X conversion in all three
// handlers instead of being dropped like it used to be before persistence.
func TestReasoningTokensSurviveHandlerConversion(t *testing.T) {
	sm, err := db.NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()

	if err := store.RecordUsage(&db.UsageRecord{
		ProviderUUID:    "prov-1",
		ProviderName:    "openai",
		Model:           "gpt-5.1-reasoning",
		Scenario:        "default",
		UserID:          "admin",
		Timestamp:       time.Now(),
		InputTokens:     100,
		OutputTokens:    80,
		ReasoningTokens: 30,
		Status:          "success",
	}); err != nil {
		t.Fatalf("RecordUsage failed: %v", err)
	}

	handler := NewHandler(store)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/usage/stats", handler.GetStats)
	router.GET("/usage/timeseries", handler.GetTimeSeries)
	router.GET("/usage/records", handler.GetRecords)

	start := time.Now().Add(-time.Hour).Format(time.RFC3339)
	end := time.Now().Add(time.Hour).Format(time.RFC3339)

	t.Run("stats", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/usage/stats?start_time="+start+"&end_time="+end, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		assert.Contains(t, w.Body.String(), `"reasoning_tokens":30`)
	})

	t.Run("timeseries", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/usage/timeseries?interval=hour&start_time="+start+"&end_time="+end, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		assert.Contains(t, w.Body.String(), `"reasoning_tokens":30`)
	})

	t.Run("records", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/usage/records?start_time="+start+"&end_time="+end, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		assert.Contains(t, w.Body.String(), `"reasoning_tokens":30`)
	})
}

// TestUsageAPIUTCWindowFromLocalBrowser is the API-level counterpart of the
// #2797 regression: a UTC server, a UTC+8 browser asking for its "today" window
// (local midnight → 01:58), and the JSON that comes back. The record must be
// found and echoed with an explicit UTC timestamp.
func TestUsageAPIUTCWindowFromLocalBrowser(t *testing.T) {
	sm, err := db.NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager failed: %v", err)
	}
	defer sm.Close()
	store := sm.Usage()

	// 2026-11-26 01:58 in UTC+8 == 2026-11-25 17:58 UTC.
	instant := time.Date(2026, 11, 25, 17, 58, 0, 0, time.UTC)
	if err := store.RecordUsage(&db.UsageRecord{
		ProviderUUID: "prov-1",
		ProviderName: "openai",
		Model:        "gpt-5.1",
		Scenario:     "default",
		UserID:       "admin",
		Timestamp:    instant,
		InputTokens:  100,
		Status:       "success",
	}); err != nil {
		t.Fatalf("RecordUsage failed: %v", err)
	}

	handler := NewHandler(store)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/usage/records", handler.GetRecords)

	// The browser's local "today" window, carrying its +08:00 offset.
	start := url.QueryEscape("2026-11-26T00:00:00+08:00")
	end := url.QueryEscape("2026-11-26T01:58:00+08:00")
	req, _ := http.NewRequest("GET", "/usage/records?start_time="+start+"&end_time="+end, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	assert.Contains(t, body, `"total":1`, "the record inside the browser's local day must be returned")
	assert.Contains(t, body, `"timestamp":"2026-11-25T17:58:00Z"`, "record timestamps are serialized as UTC")
}
