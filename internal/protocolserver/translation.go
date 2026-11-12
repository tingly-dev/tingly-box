package protocolserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/translation"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type translationErrorDetail struct {
	Type       string `json:"type"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	InputIndex *int   `json:"input_index,omitempty"`
}

func translationError(c *gin.Context, status int, code, message string, index *int) {
	c.Error(fmt.Errorf("%s: %s", code, message)).SetType(gin.ErrorTypePublic)
	c.JSON(status, struct {
		Error translationErrorDetail `json:"error"`
	}{translationErrorDetail{
		Type: "translation_error", Code: code, Message: message, InputIndex: index,
	}})
}

// HandleTranslation serves synchronous ordered text batches. It filters the
// configured services before selection and rebuilds native MT semantics on
// every failover attempt. It never replays already completed batch items.
func (ph *ProtocolHandler) HandleTranslation(c *gin.Context) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		translationError(c, 415, "invalid_content_type", "use application/json", nil)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, translation.MaxBodyBytes))
	if err != nil {
		var large *http.MaxBytesError
		status := 400
		if errors.As(err, &large) {
			status = 413
		}
		translationError(c, status, "invalid_request", "cannot read translation body (maximum 256 KiB)", nil)
		return
	}
	var req translation.Request
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if !utf8.Valid(body) || d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
		translationError(c, 400, "invalid_request", "expected one JSON object with model, text[], source_lang and target_lang", nil)
		return
	}
	if err := req.Normalize(); err != nil {
		translationError(c, 400, "invalid_request", err.Error(), nil)
		return
	}
	scenario := typ.RuleScenario(c.Param("scenario"))
	if !IsValidRuleScenario(scenario) || (!typ.ScenarioSupportsTransport(scenario, typ.TransportOpenAI) && !typ.ScenarioSupportsTransport(scenario, typ.TransportAnthropic)) {
		translationError(c, 400, "unsupported_scenario", "scenario does not support text translation", nil)
		return
	}
	rule, err := ph.determineRuleWithScenario(c, scenario, req.Model)
	if err != nil {
		translationError(c, 400, "route_not_configured", err.Error(), nil)
		return
	}
	if rule.SmartEnabled || len(rule.SmartRouting) != 0 {
		translationError(c, 400, "unsupported_rule", "v1 translation requires a rule without smart-routing partitions", nil)
		return
	}
	// A shallow view preserves the stored rule while narrowing all initial and
	// fallback choices to verified native translation services for this pair.
	view := *rule
	view.Services = nil
	for _, svc := range rule.GetActiveServices() {
		if !translation.Supports(catalog.TranslationAdapter(svc.Model), req.SourceLang, req.TargetLang) {
			continue
		}
		p, err := ph.deps.Config.GetProviderByUUID(svc.Provider)
		if err != nil || p == nil || !p.Enabled || p.IsOAuth() || p.IsVirtual() || p.IsMultiFieldCredential() {
			continue
		}
		if p.ResolveStyle(protocol.APIStyleOpenAI).APIStyle == protocol.APIStyleOpenAI {
			view.Services = append(view.Services, svc)
		}
	}
	if len(view.Services) == 0 {
		translationError(c, 400, "no_compatible_translation_service", "configure a cataloged translation model with a supported language pair and OpenAI serving endpoint", nil)
		return
	}
	applyRuleFlags(c, ResolveRuleFlags(c, rule))
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), resolveSessionID(c, &req)))
	out := translation.Response{Model: req.Model, Translations: make([]translation.Result, 0, len(req.Text))}
	for i, text := range req.Text {
		if c.Request.Context().Err() != nil {
			translationError(c, 504, "request_canceled", "translation request canceled", &i)
			return
		}
		// Routing receives original text; native templates are built after selection.
		routingReq := &openai.ChatCompletionNewParams{Model: req.Model, Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage(text)}}
		p, svc, err := ph.selectService(c, scenario, &view, routingReq)
		if err != nil {
			translationError(c, 503, "service_unavailable", "no translation service available", &i)
			return
		}
		SetTrackingContext(c, rule, p, svc.Model, req.Model, false)
		writer := c.Writer
		buffer := newFirstChunkGate(writer)
		func() {
			c.Writer = buffer
			defer func() { c.Writer = writer }()
			ph.DispatchWithPriorityFailover(c, &view, p, svc.Model, func(provider *typ.Provider, model string) {
				if c.Request.Context().Err() != nil {
					translationError(c, 504, "request_canceled", "translation request canceled", &i)
					return
				}
				backend := catalog.TranslationAdapter(model)
				native, err := translation.ChatRequest(backend, model, text, req.SourceLang, req.TargetLang)
				if err != nil {
					translationError(c, 400, "unsupported_language_pair", err.Error(), &i)
					return
				}
				provider = provider.ResolveStyle(protocol.APIStyleOpenAI)
				UpdateTrackingForFailover(c, provider, model)
				wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, model)
				resp, cancel, err := forwarding.ForwardOpenAIChat(forwarding.NewForwardContext(c.Request.Context(), provider), wrapper, native)
				if cancel != nil {
					defer cancel()
				}
				if err != nil {
					failure := protocol.ClassifyUpstreamFailure(err, 502)
					// An upstream failure carries no measured token counts. Report
					// availability without inserting a fabricated zero-token measurement.
					if ph.deps.HealthMonitor != nil {
						if failure.Status == 401 || failure.Status == 403 {
							ph.deps.HealthMonitor.ReportAuthError(loadbalance.FormatServiceID(provider.UUID, model), failure.Status)
						} else if failure.Status == 429 {
							ph.deps.HealthMonitor.ReportRateLimit(loadbalance.FormatServiceID(provider.UUID, model))
						}
					}
					status := failure.Status
					if status == 401 || status == 403 {
						status = 502
					}
					if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
						status = 504
					}
					translationError(c, status, "upstream_error", failure.Message, &i)
					return
				}
				result, resultErr := translation.CompletionResult(resp)
				// Only record token measurements when the provider supplies them.
				if resp != nil && resp.JSON.Usage.Valid() && resp.Usage.JSON.PromptTokens.Valid() && resp.Usage.JSON.CompletionTokens.Valid() && resp.Usage.PromptTokens >= 0 && resp.Usage.CompletionTokens >= 0 {
					ph.trackUsageWithTokenUsage(c, protocol.NewTokenUsageWithCache(int(resp.Usage.PromptTokens), int(resp.Usage.CompletionTokens), int(resp.Usage.PromptTokensDetails.CachedTokens)), resultErr)
				}
				if resultErr != nil {
					translationError(c, 502, "invalid_translation", resultErr.Error(), &i)
					return
				}
				if ph.deps.HealthMonitor != nil {
					ph.deps.HealthMonitor.ReportSuccess(loadbalance.FormatServiceID(provider.UUID, model))
				}
				loadbalance.RecordServiceSuccess(rule.UUID, loadbalance.FormatServiceID(provider.UUID, model))
				c.JSON(200, result)
			})
		}()
		if buffer.Status() != 200 {
			buffer.CommitIfBuffered()
			return
		}
		var result translation.Result
		if err := json.Unmarshal(buffer.buf.Bytes(), &result); err != nil {
			translationError(c, 502, "invalid_translation", "invalid translation result", &i)
			return
		}
		out.Translations = append(out.Translations, result)
	}
	c.JSON(200, out)
}
