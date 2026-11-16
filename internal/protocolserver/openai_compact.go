package protocolserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleResponsesCompact attempts standalone native compaction independently
// of the configured inference protocol. Returned windows remain opaque;
// summary conversion and remote V2 are separate capabilities.
func (ph *ProtocolHandler) HandleResponsesCompact(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if !IsValidRuleScenario(scenario) || !typ.ScenarioSupportsTransport(scenario, typ.TransportOpenAI) {
		compactBadRequest(c, fmt.Sprintf("scenario %s does not support OpenAI compaction", scenario))
		return
	}
	body, err := c.GetRawData()
	if err != nil {
		compactBadRequest(c, "Failed to read request body: "+err.Error())
		return
	}
	requestModel, err := compactRequestModel(body)
	if err != nil {
		compactBadRequest(c, err.Error())
		return
	}
	rule, err := ph.determineRuleWithScenario(c, scenario, requestModel)
	if err != nil {
		compactBadRequest(c, err.Error())
		return
	}
	// Select one service without classifying opaque history as a chat prompt.
	provider, service, err := ph.selectServiceForEmbeddings(c, scenario, rule)
	if err != nil {
		compactBadRequest(c, err.Error())
		return
	}
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), resolveSessionID(c, nil)))
	// Compact is its own operation. Chat/Responses inference preferences do
	// not establish whether this provider implements /responses/compact.
	provider = provider.ResolveStyle(ph.resolveAttemptStyle(c, rule, provider, protocol.TypeOpenAIResponses))
	SetTrackingContext(c, rule, provider, service.Model, requestModel, false)
	if provider.APIStyle == protocol.APIStyleAnthropic {
		unsupportedResponsesCompaction(c, "Native compact is not supported on the Anthropic endpoint")
		return
	}
	ResolveRuleFlagsWithScenario(c, rule, scenario, ph.deps.Config.GetScenarioConfig(scenario), protocol.TypeOpenAIResponses, protocol.TypeOpenAIResponses, provider)
	upstreamBody, err := sjson.SetBytes(body, "model", service.Model)
	if err != nil {
		compactBadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, service.Model)
	result, cancel, err := forwarding.ForwardOpenAIResponsesCompact(fc, wrapper, upstreamBody)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		writeCompactUpstreamError(c, err)
		return
	}
	var usage responses.ResponseUsage
	_ = json.Unmarshal([]byte(gjson.GetBytes(result, "usage").Raw), &usage)
	ph.trackUsageWithTokenUsage(c, protocolusage.FromOpenAIResponses(usage), nil)
	// CompactedResponse normally has no model; only rewrite an echoed alias.
	if model := gjson.GetBytes(result, "model"); model.Type == gjson.String {
		if rewritten, rewriteErr := sjson.SetBytes(result, "model", requestModel); rewriteErr == nil {
			result = rewritten
		}
	}
	c.Data(http.StatusOK, "application/json", result)
}

func compactRequestModel(body []byte) (string, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return "", fmt.Errorf("request body must be a JSON object")
	}
	model := gjson.GetBytes(body, "model")
	if model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		return "", fmt.Errorf("model is required")
	}
	return model.String(), nil
}

func compactBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{Error: ErrorDetail{Message: message, Type: "invalid_request_error"}})
}

// Unsupported is a terminal client error, not a transient server failure.
func unsupportedResponsesCompaction(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{Error: ErrorDetail{
		Message: message, Type: "invalid_request_error", Code: "unsupported_compaction",
	}})
}

// Only explicit capability errors are normalized. Authentication, quota,
// model-not-found and transient failures retain their upstream status and code.
func writeCompactUpstreamError(c *gin.Context, err error) {
	c.Error(err).SetType(gin.ErrorTypePublic)
	failure := protocol.ClassifyUpstreamFailure(err, http.StatusBadGateway)
	kind, code := "api_error", ""
	status := failure.Status
	var upstream *openai.Error
	if errors.As(err, &upstream) {
		kind, code = upstream.Type, upstream.Code
	}

	unsupported := code == "unsupported_compaction" || code == "unsupported_operation" || code == "not_implemented"
	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		unsupported = true
	}
	// A plain 404 at this exact endpoint means it is absent; preserve specific
	// errors such as model_not_found rather than guessing they mean unsupported.
	if status == http.StatusNotFound && (code == "" || code == "not_found" || code == "endpoint_not_found") {
		unsupported = true
	}
	if unsupported && (status == 400 || status == 404 || status == 405 || status == 501) {
		unsupportedResponsesCompaction(c, "The selected upstream does not support native Responses compaction")
		return
	}
	if kind == "" {
		kind = "api_error"
	}
	c.JSON(failure.Status, ErrorResponse{Error: ErrorDetail{Message: failure.Message, Type: kind, Code: code}})
}
