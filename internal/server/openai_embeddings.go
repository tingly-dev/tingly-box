package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIEmbeddings serves OpenAI-compatible embedding requests.
//
// The endpoint is exposed via the mixin route group, so any scenario whose
// descriptor declares TransportOpenAI or TransportEmbed can reach it. The
// canonical home is the dedicated `embed` scenario; `openai` scenario also
// works because its descriptor is extended with TransportEmbed.
func (ph *ProtocolHandler) HandleOpenAIEmbeddings(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	if !IsValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) &&
		!typ.ScenarioSupportsTransport(scenarioType, typ.TransportEmbed) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("scenario %s does not support embeddings", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to read request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	var req openai.EmbeddingNewParams
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if string(req.Model) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if isEmbeddingInputEmpty(req.Input) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Input is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	requestModel := string(req.Model)
	responseModel := requestModel

	rule, err := ph.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	provider, selectedService, err := ph.deps.RoutingSelector.SelectServiceForEmbeddings(c, scenarioType, rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if provider.APIStyle != protocol.APIStyleOpenAI {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("unsupported provider api style for embeddings: %s", provider.APIStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	actualModel := selectedService.Model
	req.Model = openai.EmbeddingModel(actualModel)

	sessionID := resolveSessionID(c, &req)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	SetTrackingContext(c, rule, provider, actualModel, responseModel, false)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	resp, cancel, err := forwarding.ForwardOpenAIEmbeddings(fc, wrapper, &req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		ph.trackUsageWithTokenUsage(c, usage, err)
		logrus.Errorf("Failed to forward embeddings request: %v", err)
		c.JSON(protocol.UpstreamStatus(err, http.StatusInternalServerError), ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	usage := protocol.NewTokenUsageWithCache(int(resp.Usage.PromptTokens), 0, 0)
	ph.trackUsageWithTokenUsage(c, usage, nil)

	responseJSON, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to process response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	responseMap["model"] = responseModel
	c.JSON(http.StatusOK, responseMap)
}

// isEmbeddingInputEmpty returns true if no variant of the union input is set.
func isEmbeddingInputEmpty(input openai.EmbeddingNewParamsInputUnion) bool {
	if !param.IsOmitted(input.OfString) && input.OfString.Value != "" {
		return false
	}
	if len(input.OfArrayOfStrings) > 0 {
		return false
	}
	if len(input.OfArrayOfTokens) > 0 {
		return false
	}
	if len(input.OfArrayOfTokenArrays) > 0 {
		return false
	}
	return true
}
