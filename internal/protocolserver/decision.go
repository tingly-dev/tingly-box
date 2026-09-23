package protocolserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const maxDecisionRequestBytes = 8 << 20

// HandleDecision proxies the native structured-decision protocol without
// translating it through a prose-oriented chat API.
func (ph *ProtocolHandler) HandleDecision(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if !IsValidRuleScenario(scenario) || !typ.ScenarioSupportsTransport(scenario, typ.TransportDecision) {
		decisionError(c, http.StatusBadRequest, fmt.Sprintf("scenario %s does not support decisions", scenario))
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxDecisionRequestBytes+1))
	if err != nil {
		decisionError(c, http.StatusBadRequest, "failed to read request body: "+err.Error())
		return
	}
	if len(body) > maxDecisionRequestBytes {
		decisionError(c, http.StatusRequestEntityTooLarge, "request body exceeds 8 MiB")
		return
	}

	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		decisionError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	var requestModel string
	if err := json.Unmarshal(req["model"], &requestModel); err != nil || strings.TrimSpace(requestModel) == "" {
		decisionError(c, http.StatusBadRequest, "model is required")
		return
	}
	var questions map[string]json.RawMessage
	if err := json.Unmarshal(req["questions"], &questions); err != nil || len(questions) == 0 {
		decisionError(c, http.StatusBadRequest, "at least one question is required")
		return
	}

	rule, err := ph.determineRuleWithScenario(c, scenario, requestModel)
	if err != nil {
		decisionError(c, http.StatusBadRequest, err.Error())
		return
	}
	provider, service, err := ph.selectService(c, scenario, rule, nil)
	if err != nil {
		decisionError(c, http.StatusBadRequest, err.Error())
		return
	}
	if provider.APIStyle != protocol.APIStyleDecision {
		decisionError(c, http.StatusBadRequest, fmt.Sprintf("unsupported provider api style for decisions: %s", provider.APIStyle))
		return
	}

	routedModel, _ := json.Marshal(service.Model)
	req["model"] = routedModel
	body, err = json.Marshal(req)
	if err != nil {
		decisionError(c, http.StatusInternalServerError, "failed to encode upstream request")
		return
	}
	SetTrackingContext(c, rule, provider, service.Model, requestModel, false)

	upstreamURL, err := decisionEndpoint(provider.APIBase)
	if err != nil {
		decisionError(c, http.StatusBadRequest, err.Error())
		return
	}
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		decisionError(c, http.StatusInternalServerError, "failed to create upstream request")
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/json")
	if token := provider.GetAccessToken(); token != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+token)
	}

	issuer := ai.Issuer("")
	if provider.OAuthDetail != nil {
		issuer = provider.OAuthDetail.GetIssuer()
	}
	transport, release := client.GetGlobalTransportPool().AcquireTransport(provider.UUID, service.Model, provider.ProxyURL, issuer, typ.SessionID{})
	defer release()
	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}
	resp, err := (&http.Client{Transport: transport, Timeout: timeout}).Do(upstreamReq)
	if err != nil {
		ph.trackUsageWithTokenUsage(c, protocol.ZeroTokenUsage(), err)
		decisionError(c, http.StatusBadGateway, "decision upstream request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		ph.trackUsageWithTokenUsage(c, protocol.ZeroTokenUsage(), readErr)
		decisionError(c, http.StatusBadGateway, "failed to read decision upstream response")
		return
	}
	ph.trackUsageWithTokenUsage(c, protocol.ZeroTokenUsage(), nil)
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Status(resp.StatusCode)
	_, _ = c.Writer.Write(responseBody)
}

func decisionEndpoint(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid decision provider api_base %q", base)
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(u.Path, "/decisions") {
		u.Path += "/decisions"
	}
	return u.String(), nil
}

func decisionError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"code": -1, "message": message, "data": nil})
}
