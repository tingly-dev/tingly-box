package server

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/obs"
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
	EventCount   int       `json:"event_count"`
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
// memory sinks.
type requestGroup struct {
	id       string
	events   []ModelRequestEvent
	summary  ModelRequestSummary
	severity logrus.Level // most severe (lowest) level seen across events
	hasModel bool         // saw a model_request or smart_routing event, or AI tracking fields
}

// GetModelRequests returns recent model requests, one row per correlation id,
// built by joining the HTTP access log, model_request stage logs and
// smart-routing traces from the in-memory sinks.
//
// Query parameters:
//   - limit: maximum number of requests to return (default: 100, max: 1000)
//   - scenario / provider / status: optional exact-match filters
func (h *WebHandler) GetModelRequests(c *gin.Context) {
	if h.deps.MultiLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Logger not available"})
		return
	}

	limit := parseLimit(c.DefaultQuery("limit", "100"))
	scenarioFilter := c.Query("scenario")
	providerFilter := c.Query("provider")
	statusFilter := c.Query("status")

	groups := h.collectRequestGroups()

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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Logger not available"})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request id is required"})
		return
	}

	groups := h.collectRequestGroups()
	g, ok := groups[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}

	sort.SliceStable(g.events, func(i, j int) bool {
		return g.events[i].Time.Before(g.events[j].Time)
	})

	sum := g.summary
	sum.RequestID = g.id
	sum.EventCount = len(g.events)

	c.JSON(http.StatusOK, ModelRequestDetail{
		ModelRequestSummary: sum,
		Events:              g.events,
	})
}

// collectRequestGroups scans the HTTP, model_request and smart_routing memory
// sinks and groups every entry by its request_id.
func (h *WebHandler) collectRequestGroups() map[string]*requestGroup {
	groups := make(map[string]*requestGroup)

	ingest := func(source obs.LogSource) {
		entries := h.deps.MultiLogger.WithSource(source).GetMemoryEntries()
		for _, entry := range entries {
			id, _ := entry.Data["request_id"].(string)
			if id == "" {
				continue
			}
			g := groups[id]
			if g == nil {
				g = &requestGroup{id: id}
				groups[id] = g
			}

			fields := make(map[string]interface{}, len(entry.Data))
			for k, v := range entry.Data {
				if k == "source" {
					continue
				}
				fields[k] = v
			}
			stage, _ := entry.Data["stage"].(string)

			g.events = append(g.events, ModelRequestEvent{
				Time:    entry.Time,
				Source:  string(source),
				Level:   entry.Level.String(),
				Stage:   stage,
				Message: entry.Message,
				Fields:  fields,
			})

			if source == obs.LogSourceModelRequest || source == obs.LogSourceSmartRouting {
				g.hasModel = true
			}
			applyToSummary(g, source, entry)
		}
	}

	ingest(obs.LogSourceHTTP)
	ingest(obs.LogSourceModelRequest)
	ingest(obs.LogSourceSmartRouting)

	return groups
}

// applyToSummary folds one entry's fields into the request's summary, treating
// the HTTP envelope as the authoritative source for transport metadata.
func applyToSummary(g *requestGroup, source obs.LogSource, entry *logrus.Entry) {
	data := entry.Data

	// Track the most severe level seen (lower logrus level == more severe).
	if g.summary.MaxLevel == "" || entry.Level < g.severity {
		g.severity = entry.Level
		g.summary.MaxLevel = entry.Level.String()
	}
	if entry.Level <= logrus.ErrorLevel {
		g.summary.HasError = true
	}
	if _, ok := data["error"]; ok {
		g.summary.HasError = true
	}

	if source == obs.LogSourceHTTP {
		// The access log is emitted once per request at completion; use it as
		// the canonical timestamp and transport metadata.
		g.summary.Time = entry.Time
		if v, ok := data["status"].(int); ok {
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
}

func parseLimit(s string) int {
	limit, err := strconv.Atoi(s)
	if err != nil || limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	return limit
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
