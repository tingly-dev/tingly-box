package protocolserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
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
		rejectRequest(c, "inbound", "", fmt.Errorf("invalid scenario: %s", scenario))
		return
	}

	if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) &&
		!typ.ScenarioSupportsTransport(scenarioType, typ.TransportEmbed) {
		rejectRequest(c, "inbound", "", fmt.Errorf("scenario %s does not support embeddings", scenario))
		return
	}

	bodyBytes, err := c.GetRawData()
	if err != nil {
		rejectRequest(c, "inbound", "", fmt.Errorf("Failed to read request body: %w", err))
		return
	}

	var req openai.EmbeddingNewParams
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		rejectRequest(c, "inbound", "", fmt.Errorf("Invalid request body: %w", err))
		return
	}

	if string(req.Model) == "" {
		rejectRequest(c, "inbound", "", errors.New("Model is required"))
		return
	}

	if isEmbeddingInputEmpty(req.Input) {
		rejectRequest(c, "inbound", string(req.Model), errors.New("Input is required"))
		return
	}

	requestModel := string(req.Model)
	responseModel := requestModel

	rule, err := ph.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		rejectRequest(c, "routing", requestModel, err)
		return
	}

	provider, selectedService, err := ph.selectServiceForEmbeddings(c, scenarioType, rule)
	if err != nil {
		rejectRequest(c, "routing", requestModel, err)
		return
	}

	// Resolve dual endpoint: when the provider has an OpenAI-compatible
	// dual URL configured, route there natively to avoid a transform.
	// Runs before the style check so dual providers pass either way.
	provider = provider.ResolveStyle(protocol.APIStyleOpenAI)

	if provider.APIStyle != protocol.APIStyleOpenAI {
		rejectRequest(c, "routing", requestModel, fmt.Errorf("unsupported provider api style for embeddings: %s", provider.APIStyle))
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
		stream.SendForwardingError(c, err)
		return
	}

	usage := protocol.NewTokenUsageWithCache(int(resp.Usage.PromptTokens), 0, 0)
	ph.trackUsageWithTokenUsage(c, usage, nil)

	// Echo the caller's request model, not the routed upstream model.
	resp.Model = responseModel
	c.JSON(http.StatusOK, resp)
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
