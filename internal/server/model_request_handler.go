package server

import (
	"crypto/sha256"
	"encoding/json"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/paginate"
)

// ModelRequestEvent is a single log line belonging to one model request,
// regardless of which pipeline stage emitted it (HTTP envelope, protocol
// conversion / upstream client call, or smart-routing evaluation).
type ModelRequestEvent struct {
	Time    time.Time              `json:"time"`
	Source  string                 `json:"source"`
	Level   string                 `json:"level"`
	Stage   string                 `json:"stage,omitempty"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// ModelRequestSummary is the per-request row shown in the Requests view. It is
// derived by correlating every event that shares a request_id.
type ModelRequestSummary struct {
	RequestID    string    `json:"request_id"`
	Time         time.Time `json:"time"`
	Scenario     string    `json:"scenario,omitempty"`
	RequestModel string    `json:"request_model,omitempty"`
	RoutedModel  string    `json:"routed_model,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	Method       string    `json:"method,omitempty"`
	Path         string    `json:"path,omitempty"`
	Status       int       `json:"status,omitempty"`
	LatencyMs    int64     `json:"latency_ms,omitempty"`
	HasError     bool      `json:"has_error"`
	MaxLevel     string    `json:"max_level,omitempty"`
	// TraceID links the request to its span tree in the in-memory trace
	// buffer (GET /api/v1/traces/{trace_id}); empty when the span was not
	// sampled or tracing is disabled.
	TraceID    string `json:"trace_id,omitempty"`
	EventCount int    `json:"event_count"`

	// Failover visibility: how many failover hops this request took and the
	// service path it walked ("prov-a/model-a → prov-b/model-b"). Zero/empty
	// when the first attempt served the request. Derived from the failover
	// loop's stage=failover_retry events, so the list view can answer "did
	// this request fail over, and to where" without opening the timeline.
	FailoverHops int    `json:"failover_hops,omitempty"`
	FailoverPath string `json:"failover_path,omitempty"`
}

// ModelRequestDetail is a summary plus the full, time-ordered event timeline.
type ModelRequestDetail struct {
	ModelRequestSummary
	Events []ModelRequestEvent `json:"events"`
}

// ModelRequestsResponse is the list response for the Requests view.
type ModelRequestsResponse struct {
	Total    int                   `json:"total"`
	Requests []ModelRequestSummary `json:"requests"`
}

// requestGroup accumulates events for a single request_id while scanning the
// retained history and live memory.
type requestGroup struct {
	id       string
	events   []ModelRequestEvent
	summary  ModelRequestSummary
	severity logrus.Level // most severe (lowest) level seen across events
	hasModel bool         // saw a model_request or smart_routing event, or AI tracking fields
}

// GetModelRequests returns recent model requests, one row per correlation id,
// built by joining the HTTP access log, model_request stage logs and
// smart-routing traces from recent JSON history and the live memory sinks.
//
// Query parameters:
//   - limit: maximum number of requests to return (default: 100, max: 1000)
//   - scenario / provider / status: optional exact-match filters
func (h *WebHandler) GetModelRequests(c *gin.Context) {
	if h.deps.MultiLogger == nil {
		apierr.Message(c, http.StatusServiceUnavailable, "Logger not available")
		return
	}

	limit := paginate.Limit(c, 100, 1000)
	scenarioFilter := c.Query("scenario")
	providerFilter := c.Query("provider")
	statusFilter := c.Query("status")

	groups, err := h.collectRequestGroups()
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, "Failed to read request history")
		return
	}

	summaries := make([]ModelRequestSummary, 0, len(groups))
	for _, g := range groups {
		if !g.hasModel {
			continue
		}
		sum := g.summary
		sum.RequestID = g.id
		sum.EventCount = len(g.events)
		if scenarioFilter != "" && sum.Scenario != scenarioFilter {
			continue
		}
		if providerFilter != "" && sum.Provider != providerFilter {
			continue
		}
		if statusFilter != "" && strconv.Itoa(sum.Status) != statusFilter {
			continue
		}
		summaries = append(summaries, sum)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Time.After(summaries[j].Time)
	})

	total := len(summaries)
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}

	c.JSON(http.StatusOK, ModelRequestsResponse{
		Total:    total,
		Requests: summaries,
	})
}

// GetModelRequestDetail returns the full event timeline for a single request id.
func (h *WebHandler) GetModelRequestDetail(c *gin.Context) {
	if h.deps.MultiLogger == nil {
		apierr.Message(c, http.StatusServiceUnavailable, "Logger not available")
		return
	}

	id := c.Param("id")
	if id == "" {
		apierr.Message(c, http.StatusBadRequest, "request id is required")
		return
	}

	groups, err := h.collectRequestGroups()
	if err != nil {
		apierr.Message(c, http.StatusInternalServerError, "Failed to read request history")
		return
	}
	g, ok := groups[id]
	if !ok {
		apierr.Message(c, http.StatusNotFound, "request not found")
		return
	}

	sum := g.summary
	sum.RequestID = g.id
	sum.EventCount = len(g.events)

	c.JSON(http.StatusOK, ModelRequestDetail{
		ModelRequestSummary: sum,
		Events:              g.events,
	})
}

const recentRequestEventLimit = 5000

// collectRequestGroups joins a bounded persistent history with live memory
// entries. Memory is a cache, not the retention boundary for request details.
func (h *WebHandler) collectRequestGroups() (map[string]*requestGroup, error) {
	groups := make(map[string]*requestGroup)
	entries, err := h.deps.MultiLogger.ReadJSONLogsFiltered(recentRequestEventLimit, func(e obs.LogEntry) bool {
		if stringField(e.Fields, "request_id") == "" {
			return false
		}
		switch obs.LogSource(e.Source) {
		case obs.LogSourceHTTP:
			return strings.HasPrefix(stringField(e.Fields, "path"), "/tingly/") ||
				firstStringField(e.Fields, "scenario", "request_model", "routed_provider") != ""
		case obs.LogSourceModelRequest, obs.LogSourceSmartRouting, obs.LogSourceSystem, obs.LogSourceAction, "":
			return true
		default:
			return false
		}
	})
	if err != nil {
		return nil, err
	}
	// Normalize live entries once, then fold both sources in time order.
	for _, source := range []obs.LogSource{obs.LogSourceHTTP, obs.LogSourceModelRequest, obs.LogSourceSmartRouting, obs.LogSourceSystem, obs.LogSourceAction} {
		for _, e := range h.deps.MultiLogger.WithSource(source).GetMemoryEntries() {
			fields := maps.Clone(e.Data)
			delete(fields, "source")
			entries = append(entries, obs.LogEntry{Time: e.Time, Level: e.Level.String(), Message: e.Message, Source: string(source), Fields: fields})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Time.Before(entries[j].Time) })
	seen := make(map[[32]byte]bool)
	for _, entry := range entries {
		id := stringField(entry.Fields, "request_id")
		level, err := logrus.ParseLevel(entry.Level)
		if id == "" || err != nil {
			continue
		}
		if entry.Source == "" {
			entry.Source = string(obs.LogSourceSystem)
		}
		// JSON normalizes memory values and persisted numbers for deduplication.
		if encoded, err := json.Marshal(entry); err == nil {
			key := sha256.Sum256(encoded)
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		g := groups[id]
		if g == nil {
			g = &requestGroup{id: id}
			groups[id] = g
		}
		g.events = append(g.events, ModelRequestEvent{
			Time:    entry.Time,
			Source:  entry.Source,
			Level:   entry.Level,
			Stage:   stringField(entry.Fields, "stage"),
			Message: entry.Message,
			Fields:  entry.Fields,
		})
		if entry.Source == string(obs.LogSourceModelRequest) || entry.Source == string(obs.LogSourceSmartRouting) {
			g.hasModel = true
		}
		applyToSummary(g, entry, level)
	}

	return groups, nil
}

// applyToSummary folds one entry's fields into the request's summary, treating
// the HTTP envelope as the authoritative source for transport metadata.
func applyToSummary(g *requestGroup, entry obs.LogEntry, level logrus.Level) {
	data := entry.Fields

	// Track the most severe level seen (lower logrus level == more severe).
	if g.summary.MaxLevel == "" || level < g.severity {
		g.severity = level
		g.summary.MaxLevel = entry.Level
	}
	if level <= logrus.ErrorLevel {
		g.summary.HasError = true
	}
	if _, ok := data["error"]; ok {
		g.summary.HasError = true
	}

	if entry.Source == string(obs.LogSourceHTTP) {
		// The access log is emitted once per request at completion; use it as
		// the canonical timestamp and transport metadata.
		g.summary.Time = entry.Time
		if v := intField(data["status"]); v != 0 {
			g.summary.Status = v
			if v >= 400 {
				g.summary.HasError = true
			}
		}
		g.summary.Method = stringField(data, "method")
		g.summary.Path = stringField(data, "path")
		if ms := latencyMs(data["latency"]); ms > 0 {
			g.summary.LatencyMs = ms
		}
		if v := stringField(data, "scenario"); v != "" {
			g.summary.Scenario = v
			g.hasModel = true
		}
		if v := stringField(data, "request_model"); v != "" {
			g.summary.RequestModel = v
			g.hasModel = true
		}
		if v := stringField(data, "routed_model"); v != "" {
			g.summary.RoutedModel = v
		}
		if v := stringField(data, "routed_provider"); v != "" {
			g.summary.Provider = v
			g.hasModel = true
		}
		if v := stringField(data, "trace_id"); v != "" {
			g.summary.TraceID = v
		}
		return
	}

	// model_request / smart_routing entries supplement fields the envelope may
	// lack (e.g. when the request errored before the access log captured them).
	// Until an HTTP envelope arrives (Status==0), track the latest event time.
	if g.summary.Status == 0 && entry.Time.After(g.summary.Time) {
		g.summary.Time = entry.Time
	}
	if g.summary.Scenario == "" {
		g.summary.Scenario = stringField(data, "scenario")
	}
	if g.summary.RequestModel == "" {
		g.summary.RequestModel = stringField(data, "request_model")
	}
	if g.summary.RoutedModel == "" {
		g.summary.RoutedModel = firstStringField(data, "routed_model", "selected_model", "model")
	}
	if g.summary.Provider == "" {
		g.summary.Provider = firstStringField(data, "provider", "selected_provider", "routed_provider")
	}

	// Fold failover hops into the summary. The failover loop emits one
	// stage=failover_retry event per hop (with from_service/to_service), in
	// order, so the path accumulates as "A → B → C".
	if stringField(data, "stage") == "failover_retry" {
		g.summary.FailoverHops++
		if from := stringField(data, "from_service"); from != "" && g.summary.FailoverPath == "" {
			g.summary.FailoverPath = from
		}
		if to := stringField(data, "to_service"); to != "" {
			if g.summary.FailoverPath != "" {
				g.summary.FailoverPath += " → "
			}
			g.summary.FailoverPath += to
		}
	}
}

// JSON-backed entries decode numbers as float64, memory entries retain ints.
func intField(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func stringField(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func firstStringField(data map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v := stringField(data, k); v != "" {
			return v
		}
	}
	return ""
}

// latencyMs normalises a logrus "latency" field (stored as time.Duration in
// memory) into milliseconds.
func latencyMs(v interface{}) int64 {
	switch d := v.(type) {
	case time.Duration:
		return d.Milliseconds()
	case int64:
		return time.Duration(d).Milliseconds()
	case float64:
		return time.Duration(int64(d)).Milliseconds()
	default:
		return 0
	}
}
