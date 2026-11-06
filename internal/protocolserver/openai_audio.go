package protocolserver

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func audioBadRequest(c *gin.Context, status int, message string) {
	writeErrorResponse(c, status, ErrorResponse{Error: ErrorDetail{Message: message, Type: "invalid_request_error"}})
}

func audioScenario(c *gin.Context, transport typ.ScenarioTransport) (typ.RuleScenario, bool) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if !IsValidRuleScenario(scenario) || !typ.ScenarioSupportsTransport(scenario, transport) {
		audioBadRequest(c, http.StatusBadRequest, fmt.Sprintf("scenario %s does not support %s", scenario, transport))
		return scenario, false
	}
	return scenario, true
}

func (ph *ProtocolHandler) resolveAudioRoute(c *gin.Context, scenario typ.RuleScenario, model string, streamed bool) (*typ.Provider, string, bool) {
	rule, err := ph.determineRuleWithScenario(c, scenario, model)
	if err != nil {
		audioBadRequest(c, http.StatusBadRequest, err.Error())
		return nil, "", false
	}
	// Audio has no chat context; skip content-based routing but retain health,
	// affinity, load balancing and the routing trace.
	provider, service, err := ph.selectService(c, scenario, rule, nil)
	if err != nil {
		audioBadRequest(c, http.StatusBadRequest, err.Error())
		return nil, "", false
	}
	provider = provider.ResolveStyle(protocol.APIStyleOpenAI)
	if provider.APIStyle != protocol.APIStyleOpenAI || provider.IsOAuth() || provider.IsVirtual() {
		audioBadRequest(c, http.StatusBadRequest, "provider does not support the OpenAI Audio API; configure an OpenAI-compatible API-key provider")
		return nil, "", false
	}
	// Carry rule/scenario wire headers into the pooled SDK transport without
	// applying any chat-body transforms to audio requests.
	ResolveRuleFlagsWithScenario(c, rule, scenario, ph.deps.Config.GetScenarioConfig(scenario), "", "", provider)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), resolveSessionID(c, nil)))
	SetTrackingContext(c, rule, provider, service.Model, model, streamed)
	return provider, service.Model, true
}

// forwardAudio copies binary, text, JSON and SSE responses without rewriting
// them. Its observer keeps only bounded JSON/SSE metadata for optional usage;
// audio bytes are never saved to the request/response recording layer.
func (ph *ProtocolHandler) forwardAudio(c *gin.Context, provider *typ.Provider, model, path, contentType string, body io.Reader) {
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	response, cancel, err := forwarding.ForwardOpenAIAudio(fc, wrapper, path, contentType, body)
	if cancel != nil {
		defer cancel()
	}
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		if response != nil {
			c.Header("Retry-After", response.Header.Get("Retry-After"))
		}
		ph.failForward(c, err)
		return
	}
	if response == nil || response.Body == nil {
		ph.failForward(c, errors.New("empty upstream audio response"))
		return
	}
	for _, name := range []string{"Content-Type", "Content-Disposition", "X-Request-ID", "Retry-After"} {
		if value := response.Header.Get(name); value != "" {
			c.Header(name, value)
		}
	}
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Header("Content-Type", "application/octet-stream")
	}
	observer := newAudioUsageObserver(response.Header.Get("Content-Type"))
	if observer.sse {
		c.Set(ContextKeyStreamed, true)
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
	}
	c.Status(response.StatusCode)
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if _, exists := c.Get(ContextKeyFirstTokenTime); !exists {
				c.Set(ContextKeyFirstTokenTime, time.Now())
			}
			observer.Write(buffer[:n])
			if _, err = c.Writer.Write(buffer[:n]); err != nil {
				break
			}
			c.Writer.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				err = readErr
			}
			break
		}
	}
	// A read failure after headers must not append a JSON error to audio/SSE.
	// Abort the response so clients can distinguish truncation from success.
	usage := observer.usage()
	trackingErr := err
	if trackingErr == nil {
		trackingErr = observer.streamError
	}
	ph.trackUsageWithTokenUsage(c, usage, trackingErr)
	if err != nil {
		panic(http.ErrAbortHandler)
	}
}

const maxAudioUsageMetadata = 1024 * 1024

type audioUsageObserver struct {
	sse, json   bool
	buffer      []byte
	overflow    bool
	tokens      *protocol.TokenUsage
	streamError error
}

func newAudioUsageObserver(contentType string) *audioUsageObserver {
	return &audioUsageObserver{
		sse:    strings.HasPrefix(contentType, "text/event-stream"),
		json:   strings.Contains(contentType, "json"),
		tokens: protocol.NewTokenUsageWithCache(0, 0, 0),
	}
}

func (o *audioUsageObserver) Write(data []byte) {
	if !o.sse && !o.json {
		return
	}
	if !o.sse {
		if !o.overflow && len(o.buffer)+len(data) <= maxAudioUsageMetadata {
			o.buffer = append(o.buffer, data...)
		} else {
			o.overflow = true
			o.buffer = nil
		}
		return
	}
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		if index < 0 {
			index = len(data)
		}
		if !o.overflow && len(o.buffer)+index <= maxAudioUsageMetadata {
			o.buffer = append(o.buffer, data[:index]...)
		} else {
			o.overflow = true
			o.buffer = nil
		}
		if index == len(data) {
			return
		}
		if !o.overflow {
			line := bytes.TrimSpace(o.buffer)
			if bytes.HasPrefix(line, []byte("data:")) {
				o.observe(bytes.TrimSpace(line[5:]))
			}
		}
		o.buffer = o.buffer[:0]
		o.overflow = false
		data = data[index+1:]
	}
}

func (o *audioUsageObserver) observe(body []byte) {
	if gjson.GetBytes(body, "type").String() == "error" || gjson.GetBytes(body, "error").IsObject() {
		o.streamError = errors.New("upstream audio stream error")
	}
	u := gjson.GetBytes(body, "usage")
	if !u.Exists() {
		return
	}
	in := u.Get("input_tokens")
	if !in.Exists() {
		in = u.Get("prompt_tokens")
	}
	out := u.Get("output_tokens")
	if !out.Exists() {
		out = u.Get("completion_tokens")
	}
	// These are provider-reported totals, never estimated from audio bytes.
	o.tokens = protocol.NewTokenUsageWithCache(int(in.Int()), int(out.Int()), 0)
}

func (o *audioUsageObserver) usage() *protocol.TokenUsage {
	if o.json && !o.overflow {
		o.observe(o.buffer)
	}
	if o.sse && !o.overflow && bytes.HasPrefix(bytes.TrimSpace(o.buffer), []byte("data:")) {
		o.observe(bytes.TrimSpace(bytes.TrimSpace(o.buffer)[5:]))
	}
	return o.tokens
}
