package server

import (
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SDKSessionRequest is the body for POST /api/v1/sdk/session.
type SDKSessionRequest struct {
	// Scenario is the rule scenario the SDK session should bind to.
	// Defaults to "experiment" when empty.
	Scenario string `json:"scenario"`
	// Name is a human label that shows up in tingly-box logs as the caller,
	// so experiments are distinguishable in the request history.
	Name string `json:"name"`
}

// SDKSessionResponse is returned by POST /api/v1/sdk/session. It hands the
// Python SDK everything it needs to construct a tingly-box-bound OpenAI /
// Anthropic client: a base URL, a bearer token, and the transports the
// scenario accepts.
type SDKSessionResponse struct {
	// BaseURL is the scenario root, e.g. "http://127.0.0.1:12580/tingly/experiment".
	// The OpenAI SDK should target BaseURL+"/v1"; the Anthropic SDK targets BaseURL.
	BaseURL string `json:"base_url"`
	// Token is the bearer token to authenticate against the gateway. In v0.1
	// this is the gateway model token (long-lived); scoped short-lived tokens
	// are a follow-up.
	Token string `json:"token"`
	// Scenario is the resolved scenario id.
	Scenario string `json:"scenario"`
	// Transport is "openai", "anthropic", or "both", derived from the scenario
	// descriptor. It tells the SDK which client styles are valid.
	Transport string `json:"transport"`
	// Ready is true when an active rule with at least one service is bound to
	// the scenario. When false, requests will fail until the user binds a rule;
	// the SDK's `tingly doctor` surfaces this as the next action.
	Ready bool `json:"ready"`
	// Services is the number of active services bound to the scenario's rule.
	Services int `json:"services"`
	// ExpiresAt is the token expiry. Empty in v0.1 (long-lived model token).
	ExpiresAt string `json:"expires_at,omitempty"`
}

// CreateSDKSession mints an SDK session for a scenario. It is the single
// gateway-side endpoint the `tingly` Python module relies on: given a scenario,
// it returns the base URL, bearer token, and accepted transports so a user can
// write an experiment or plugin in a handful of lines and reuse the gateway's
// routing, fallback, guard rails, quota, and logging.
func (s *Server) CreateSDKSession(c *gin.Context) {
	var req SDKSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Tolerate an empty body — default everything.
		req = SDKSessionRequest{}
	}

	scenario := typ.RuleScenario(req.Scenario)
	if scenario == "" {
		scenario = typ.ScenarioExperiment
	}

	descriptor, ok := typ.GetScenarioDescriptor(scenario)
	if !ok || !descriptor.AllowRuleBinding {
		c.JSON(http.StatusNotFound, gin.H{
			"success":         false,
			"error":           "unknown or non-bindable scenario: " + string(scenario),
			"valid_scenarios": bindableScenarioIDs(),
		})
		return
	}

	ready, services := s.scenarioRuleStatus(scenario)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": SDKSessionResponse{
			BaseURL:   s.scenarioBaseURL(scenario),
			Token:     s.config.GetModelToken(),
			Scenario:  string(scenario),
			Transport: scenarioTransportLabel(descriptor),
			Ready:     ready,
			Services:  services,
		},
	})
}

// scenarioBaseURL builds the externally reachable scenario root URL. A bind
// host of 0.0.0.0 / empty is rewritten to 127.0.0.1 so the returned URL is
// usable by a local SDK client.
func (s *Server) scenarioBaseURL(scenario typ.RuleScenario) string {
	host := s.config.GetServerHost()
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	port := s.config.GetServerPort()
	if port == 0 {
		port = 12580
	}
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/tingly/" + string(scenario.Base()),
	}).String()
}

// scenarioRuleStatus reports whether an active rule with at least one active
// service is bound to the scenario, and how many active services it has.
func (s *Server) scenarioRuleStatus(scenario typ.RuleScenario) (ready bool, services int) {
	for i := range s.config.GetRequestConfigs() {
		rule := s.config.GetRequestConfigs()[i]
		if !rule.Active {
			continue
		}
		if rule.GetScenario().Base() != scenario.Base() {
			continue
		}
		n := len(rule.GetActiveServices())
		if n > services {
			services = n
		}
		if n > 0 {
			ready = true
		}
	}
	return ready, services
}

// scenarioTransportLabel collapses a descriptor's supported transports into the
// label the SDK understands: "openai", "anthropic", or "both".
func scenarioTransportLabel(descriptor typ.ScenarioDescriptor) string {
	openai, anthropic := false, false
	for _, t := range descriptor.SupportedTransport {
		switch t {
		case typ.TransportOpenAI:
			openai = true
		case typ.TransportAnthropic:
			anthropic = true
		}
	}
	switch {
	case openai && anthropic:
		return "both"
	case anthropic:
		return "anthropic"
	default:
		return "openai"
	}
}

// bindableScenarioIDs lists scenario ids a caller may bind an SDK session to.
func bindableScenarioIDs() []string {
	var ids []string
	for _, d := range typ.RegisteredScenarioDescriptors() {
		if d.AllowRuleBinding {
			ids = append(ids, string(d.ID))
		}
	}
	return ids
}
