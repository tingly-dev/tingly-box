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

	"github.com/tingly-dev/tingly-box/internal/client"
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
	// fallback choices to translation backends compatible with this pair.
	view := *rule
	view.Services = nil
	for _, svc := range rule.GetActiveServices() {
		p, err := ph.deps.Config.GetProviderByUUID(svc.Provider)
		if err != nil {
			continue
		}
		backend := ph.translationBackend(c, rule, p, svc.Model)
		if backend != nil && backend.Supports(req.SourceLang, req.TargetLang) {
			view.Services = append(view.Services, svc)
		}
	}
	if len(view.Services) == 0 {
		translationError(c, 400, "no_compatible_translation_service", "configure an LLM, native MT model or translation service supporting this language pair", nil)
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
				backend := ph.translationBackend(c, rule, provider, model)
				if backend == nil || !backend.Supports(req.SourceLang, req.TargetLang) {
					translationError(c, 400, "unsupported_language_pair", "backend does not support this language pair", &i)
					return
				}
				UpdateTrackingForFailover(c, provider, model)
				result, err := backend.Translate(c.Request.Context(), translation.TextInput{Text: text, SourceLang: req.SourceLang, TargetLang: req.TargetLang})
				if result.Tokens != nil {
					ph.trackUsageWithTokenUsage(c, result.Tokens, err)
				}
				if err != nil {
					failure := protocol.ClassifyUpstreamFailure(err, 502)
					var nativeError *client.TranslationHTTPError
					if errors.As(err, &nativeError) {
						failure.Status = nativeError.Status
						if failure.Status < 400 {
							failure.Status = 502
						}
					}
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

				if ph.deps.HealthMonitor != nil {
					ph.deps.HealthMonitor.ReportSuccess(loadbalance.FormatServiceID(provider.UUID, model))
				}
				loadbalance.RecordServiceSuccess(rule.UUID, loadbalance.FormatServiceID(provider.UUID, model))
				c.JSON(200, result.Result)
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
