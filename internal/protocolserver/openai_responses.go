package protocolserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleResponsesCreate handles POST /v1/responses
func (ph *ProtocolHandler) HandleResponsesCreate(c *gin.Context) {
	scenario := c.Param("scenario")

	// Read raw body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		rejectRequest(c, "inbound", "", fmt.Errorf("Failed to read request body: %w", err))
		return
	}

	// Parse request (minimal parsing for validation)
	var req = &protocol.ResponseCreateRequest{}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		rejectRequest(c, "inbound", "", fmt.Errorf("Invalid request body: %w", err))
		return
	}

	// Validate required fields
	if param.IsOmitted(req.Model) || string(req.Model) == "" {
		rejectRequest(c, "inbound", "", errors.New("Model is required"))
		return
	}

	// Check if input is provided (either string or array)
	inputValue := protocol.GetInputValue(req.Input)
	if inputValue == nil {
		rejectRequest(c, "inbound", string(req.Model), errors.New("Input is required"))
		return
	}

	// Determine provider & model
	var (
		provider        *typ.Provider
		selectedService *loadbalance.Service
		rule            *typ.Rule
	)

	scenarioType := typ.RuleScenario(scenario)
	if !IsValidRuleScenario(scenarioType) {
		rejectRequest(c, "inbound", string(req.Model), fmt.Errorf("invalid scenario: %s", scenario))
		return
	}

	if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) {
		rejectRequest(c, "inbound", string(req.Model), fmt.Errorf("scenario %s does not support OpenAI responses", scenario))
		return
	}

	// Check if this is the request model name first
	rule, err = ph.determineRuleWithScenario(c, scenarioType, req.Model)
	if err != nil {
		rejectRequest(c, "routing", string(req.Model), err)
		return
	}

	// Select service using routing pipeline
	provider, selectedService, err = ph.selectService(c, scenarioType, rule, req)
	if err != nil {
		rejectRequest(c, "routing", string(req.Model), err)
		return
	}

	actualModel := selectedService.Model
	maxAllowed := ph.deps.TemplateManager.GetMaxTokensForModelByProvider(provider, actualModel)

	// Inject session ID into request context so all downstream code can access it
	sessionID := resolveSessionID(c, req.ResponseNewParams)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	// Set tracking context with all metadata (eliminates need for explicit parameter passing)
	SetTrackingContext(c, rule, provider, actualModel, req.Model, req.Stream)

	// Convert request to OpenAI SDK format first so fallback conversions can reuse it.
	params, err := ph.convertToResponsesParams(bodyBytes, actualModel)
	if err != nil {
		rejectRequest(c, "transform", string(req.Model), fmt.Errorf("Failed to convert request: %w", err))
		return
	}

	req.ResponseNewParams = params
	// req.Model is replaced with actualModel (resolved backend model) from this point on
	req.Model = actualModel

	// Apply vision proxy BEFORE failover loop so it runs exactly once.
	// Vision descriptions must be consistent across all failover attempts.
	ph.applyVisionProxy(c, scenarioType, rule, req.ResponseNewParams)

	ph.ResponsesCreate(c, scenarioType, provider, rule, req, rule.RequestModel, maxAllowed)
}

// ResponsesCreate runs the provider-independent prologue once, then drives the
// failover loop whose per-attempt callback re-runs the provider-dependent
// pipeline (target resolution → transform → dispatch) so failover can rotate
// across heterogeneous API styles.
func (ph *ProtocolHandler) ResponsesCreate(c *gin.Context, scenarioType typ.RuleScenario, provider *typ.Provider, rule *typ.Rule, req *protocol.ResponseCreateRequest, responseModel string, maxAllowed int) {
	// ── One-time prologue (provider-independent) ──

	// Auto-detect context-1m from incoming beta header for Claude Code/Desktop/Codex.
	applyContextOneM(c, rule)

	isStreaming := req.Stream
	scenarioConfig := ph.deps.Config.GetScenarioConfig(scenarioType)
	actualModel := string(req.Model)

	// Get or create the recorder (pristine request body), mirroring the
	// Anthropic entry points. Enablement is the effective capture-point
	// selection: the rule's recording flag overrides the scenario-level
	// recording_v2 default.
	if recMode := typ.EffectiveRecording(rule, scenarioConfig); recMode.Enabled() {
		bs, err := json.Marshal(req.ResponseNewParams)
		if err != nil {
			bs = []byte("{}")
		}
		ph.EnsureProtocolRecorder(c, string(scenarioType), provider, actualModel, obs.RecordMode(recMode), bs)
	}

	// Snapshot a pristine template only when failover is possible. The template
	// is the typed ResponseNewParams (post-vision-proxy — cloned per attempt so
	// PreprocessInputData and vision proxy are not re-run).
	multi := len(rule.GetActiveServices()) > 1

	// ── Per-attempt pipeline (provider-dependent) ──
	ph.DispatchWithPriorityFailover(c, rule, provider, actualModel,
		func(p *typ.Provider, retryModel string) {
			areq := req
			if multi {
				clonedParams, err := CloneResponsesParams(req.ResponseNewParams)
				if err != nil {
					ph.FailAttemptSetup(c, err)
					return
				}
				areq.ResponseNewParams = clonedParams
			}
			ph.runOpenAIResponsesAttempt(c, areq, p, retryModel, rule, isStreaming, scenarioType, scenarioConfig)
		})
}

// runOpenAIResponsesAttempt executes the provider-dependent half of an OpenAI
// Responses request for one failover attempt. Setup failures route through
// failAttemptSetup so the orchestrator can advance to the next candidate.
func (ph *ProtocolHandler) runOpenAIResponsesAttempt(c *gin.Context, req *protocol.ResponseCreateRequest, provider *typ.Provider, actualModel string, rule *typ.Rule, isStreaming bool, scenarioType typ.RuleScenario, scenarioConfig *typ.ScenarioConfig) {
	// Resolve dual endpoint: when the provider has an OpenAI-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = provider.ResolveStyle(protocol.APIStyleOpenAI)
	c.Set(ContextKeyProvider, provider)
	if provider.Timeout <= 0 {
		provider.Timeout = constant.DefaultRequestTimeout
	}

	req.Model = responses.ResponsesModel(actualModel)
	maxAllowed := ph.deps.TemplateManager.GetMaxTokensForModelByProvider(provider, actualModel)

	// Determine target API type based on provider API style
	target := protocol.TypeOpenAIResponses
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		target = protocol.TypeAnthropicBeta
	case protocol.APIStyleGoogle:
		ph.FailAttemptSetup(c, fmt.Errorf("Responses API does not support Google-style providers yet. Provider: %s", provider.Name))
		return
	case protocol.APIStyleOpenAI:
		modelOverride := ph.deps.TemplateManager.GetOpenAIEndpointOverrideForModel(provider, actualModel)
		resolvedTarget, routeErr := ResolveOpenAIEndpoint(provider, ResolveRuleFlags(c, rule), IncomingAPIResponses, modelOverride)
		if routeErr != nil {
			ph.FailAttemptSetup(c, routeErr)
			return
		}
		target = resolvedTarget
	default:
		ph.FailAttemptSetup(c, fmt.Errorf("Unsupported provider API style: %s", provider.APIStyle))
		return
	}

	// Resolve flags with scenario injection, consistent with the chat/v1/beta
	// handlers (this also applies the custom User-Agent to the request context).
	ruleFlags := ResolveRuleFlagsWithScenario(c, rule, scenarioType, scenarioConfig, protocol.TypeOpenAIResponses, target, provider)
	reqCtx, err := ph.TransformOpenAIResponses(c, req, target, provider, isStreaming, scenarioType, maxAllowed, RulePreBaseTransforms(ruleFlags), RulePreVendorTransforms(ruleFlags))
	if err != nil {
		ph.FailAttemptSetup(c, fmt.Errorf("Transform failed: %w", err))
		return
	}
	defer reqCtx.Release()

	// Carry the response-shaping hints for downstream dispatch, matching the
	// chat handler (consumed by shouldStripUsage on the conversion sub-paths).
	reqCtx.Extra["cursor_compat"] = ruleFlags.CursorCompat
	reqCtx.Extra["skip_usage"] = ruleFlags.SkipUsage

	reqCtx.RequestModel = actualModel
	ph.DispatchChainResult(c, reqCtx, rule, provider, isStreaming)
}

// convertToResponsesParams converts raw JSON to OpenAI SDK params format
// This handles the model override and forwards the rest as-is
func (ph *ProtocolHandler) convertToResponsesParams(bodyBytes []byte, actualModel string) (*responses.ResponseNewParams, error) {
	// Preprocess to add type fields to input items (needed for union deserialization)
	// and flatten output_text content blocks
	processedData, err := protocol.PreprocessInputData(bodyBytes)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(processedData, &raw); err != nil {
		return nil, err
	}

	// Override the model
	raw["model"] = actualModel

	// Marshal back to JSON and unmarshal into ResponseNewParams
	modifiedJSON, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var params = &responses.ResponseNewParams{}
	if err := json.Unmarshal(modifiedJSON, params); err != nil {
		return nil, err
	}

	return params, nil
}

// HandleResponsesGet handles GET /v1/responses/{id}
func (ph *ProtocolHandler) HandleResponsesGet(c *gin.Context) {
	responseID := c.Param("id")

	if responseID == "" {
		rejectRequest(c, "inbound", "", errors.New("Response ID is required"))
		return
	}

	// Phase 1: We don't store responses, so return not found
	// In future phases, we would retrieve from storage
	rejectRequestWithStatus(c, http.StatusNotFound, "inbound", "", "response_not_found",
		errors.New("Response retrieval is not supported in this version. Responses are not stored server-side."))
}
