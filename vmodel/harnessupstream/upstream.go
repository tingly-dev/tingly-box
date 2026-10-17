// Package harnessupstream turns a tingly-box instance into the upstream end of
// a relay test: harness → gateway under test → this instance's /virtual
// endpoints. It is opt-in (tingly-box start --harness-upstream) and never part
// of the production defaults, per the vmodel registration discipline.
//
// When enabled, the /virtual/{openai,anthropic}/v1 routes answer requests whose
// model is "harness-<scenario>" with that scenario's raw wire fixture (the same
// scenario.MockResponseBuilder the in-process matrix serves), and every request
// those routes receive is recorded for /virtual/captures. Any other model falls
// through to the regular vmodel handlers unchanged. See .design/harness-relay.md.
package harnessupstream

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

// ModelPrefix is prepended to every scenario name to form its model ID, so the
// generic scenario names (text, tool_use, ...) cannot collide with vmodels.
const ModelPrefix = "harness-"

// RequestIDHeader is the correlation header the harness sets on each request.
// A gateway that forwards it lets /virtual/captures match a request exactly.
const RequestIDHeader = "X-Harness-Request-Id"

// ModelID returns the model ID that serves the named scenario.
func ModelID(scenarioName string) string { return ModelPrefix + scenarioName }

// scenarioRoutes maps the provider-native path below a /virtual/{openai,anthropic}
// group to whether the scenario responder serves it.
var scenarioRoutes = map[string]bool{
	"/v1/chat/completions": true,
	"/v1/responses":        true,
	"/v1/messages":         true,
}

// Upstream serves the built-in scenarios under their harness-* model IDs and
// records what the /virtual routes receive.
type Upstream struct {
	scenarios *benchmark.Server
	models    map[string]bool
	captures  *captureLog
}

// New registers every built-in scenario (scenario.AllScenarios) under its
// harness-* model ID.
func New() *Upstream {
	u := &Upstream{
		scenarios: benchmark.NewScenarioServer(),
		models:    map[string]bool{},
		captures:  newCaptureLog(defaultCaptureLimit),
	}
	for _, sc := range scenario.AllScenarios() {
		sc.Name = ModelID(sc.Name)
		u.scenarios.RegisterScenario(sc)
		u.models[sc.Name] = true
	}
	return u
}

// Models returns the registered harness-* model IDs, sorted.
func (u *Upstream) Models() []string {
	ids := make([]string, 0, len(u.models))
	for id := range u.models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Intercept returns middleware for a /virtual/{openai,anthropic} route group
// mounted at groupPrefix. It records every request, then answers harness-*
// models on the chat / responses / messages routes from the scenario fixture
// and aborts; anything else continues to the vmodel handlers.
func (u *Upstream) Intercept(groupPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "read request body: " + err.Error()})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		model := modelOf(body)
		u.captures.add(c.Request, model, body)

		nativePath := strings.TrimPrefix(c.Request.URL.Path, groupPrefix)
		if !scenarioRoutes[nativePath] || !u.models[model] {
			c.Next()
			return
		}

		req := c.Request.Clone(c.Request.Context())
		req.URL.Path = nativePath
		req.Body = io.NopCloser(bytes.NewReader(body))
		u.scenarios.ServeHTTP(c.Writer, req)
		c.Abort()
	}
}

// ListCaptures serves GET /virtual/captures. Query parameters (all optional):
// request_id and model filter the records, limit caps how many come back
// (newest first, default 20).
func (u *Upstream) ListCaptures(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := parsePositive(v); err == nil {
			limit = n
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
	}
	c.JSON(http.StatusOK, CapturesResponse{
		Captures: u.captures.find(c.Query("request_id"), c.Query("model"), limit),
	})
}

// ResetCaptures serves DELETE /virtual/captures, dropping every record.
func (u *Upstream) ResetCaptures(c *gin.Context) {
	u.captures.reset()
	c.Status(http.StatusNoContent)
}

// CapturesResponse is the body of GET /virtual/captures.
type CapturesResponse struct {
	Captures []Capture `json:"captures"`
}

// modelOf extracts the "model" field of a JSON request body.
func modelOf(body []byte) string {
	var m struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &m)
	return m.Model
}
