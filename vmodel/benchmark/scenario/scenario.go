// Package scenario holds the reusable mock-provider fixtures for the vmodel
// benchmark: named Scenarios, each carrying per-format MockResponseBuilders and
// a set of check.Assertions. The scenario responder in the parent benchmark
// package serves these over HTTP; protocoltest re-exports them as aliases.
//
// Dependency direction is acyclic: scenario → check → internal/protocol, and
// scenario → vmodel (for SharedMockSpec / ErrorInjection / GenericRegistry).
package scenario

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/check"
)

// errorSpecCache caches error specs by ID to avoid repeated linear searches.
// Initialized lazily on first use.
var (
	errorSpecCache     map[string]vmodel.SharedMockSpec
	errorSpecCacheOnce sync.Once
	errorSpecCacheMu   sync.RWMutex
)

// ResponseFormat represents the format of the mock response for different endpoints.
type ResponseFormat string

const (
	FormatOpenAIChat      ResponseFormat = "openai_chat"      // /v1/chat/completions
	FormatOpenAIResponses ResponseFormat = "openai_responses" // /v1/responses
	FormatAnthropic       ResponseFormat = "anthropic"        // /v1/messages
	FormatGoogle          ResponseFormat = "google"           // /v1beta/models/.../generateContent
)

// MockResponseBuilder defines how a virtual server should respond for one response format.
type MockResponseBuilder struct {
	NonStream func() (statusCode int, body []byte)
	Stream    func() []string

	// StreamHTTPError, when >= 400, makes the streaming endpoint reply with this
	// HTTP status and the NonStream JSON body instead of a 200 SSE stream. It
	// models pre-content failures (auth / rate-limit / 5xx) that real providers
	// surface as an HTTP error even for streaming requests — the failure happens
	// before any SSE frame is written, so the status line itself carries it.
	// Mid-stream failures leave this zero and use the 200 + SSE path.
	StreamHTTPError int
}

// Scenario is a named test scenario describing what the mock provider returns
// and what assertions to run on the round-trip result.
//
// Scenario satisfies vmodel.VirtualModel so scenario storage can reuse
// vmodel.GenericRegistry.
type Scenario struct {
	Name        string
	Description string
	Tags        []string

	MockResponses map[ResponseFormat]MockResponseBuilder

	// Assertions are the content-level checks. They assume the response is the
	// scenario's own MockResponses fixture, so they only hold when the upstream
	// is test-controlled (the virtual mock).
	Assertions []check.Assertion

	// Structural are the upstream-independent checks (status, shape, counts —
	// never exact content). Consumers that route the scenario through an
	// upstream whose response is not test-controlled (a live provider, an
	// in-process vmodel) run these instead of Assertions.
	Structural []check.Assertion

	// SkipTransitive marks scenarios that should be excluded from two-hop
	// transitive tests (e.g. error scenarios that produce no comparable output).
	SkipTransitive bool
}

var _ vmodel.VirtualModel = Scenario{}

func (s Scenario) GetID() string          { return s.Name }
func (s Scenario) GetName() string        { return s.Name }
func (s Scenario) GetDescription() string { return s.Description }

func (s Scenario) GetType() vmodel.VirtualModelType {
	return vmodel.VirtualModelTypeStatic
}

func (s Scenario) SimulatedDelay() time.Duration { return 0 }

func (s Scenario) ToModel() vmodel.Model {
	return vmodel.Model{
		ID:      s.Name,
		Object:  "model",
		Created: 0,
		OwnedBy: vmodel.DefaultMockOwnedBy,
	}
}

// AllScenarios returns the full set of built-in validation scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		TextScenario(),
		ToolUseScenario(),
		ToolResultScenario(),
		ThinkingScenario(),
		MultiTurnScenario(),
		StreamingTextScenario(),
		StreamingToolUseScenario(),
		IncompleteScenario(),
		ErrorScenario(),
		Error500Scenario(),
		ErrorAuth401Scenario(),
		ErrorMidStreamCloseScenario(),
	}
}

// AllErrorScenarios returns all error scenarios for tests that need to
// exercise error handling comprehensively.
func AllErrorScenarios() []Scenario {
	return []Scenario{
		ErrorScenario(),
		Error500Scenario(),
		ErrorAuth401Scenario(),
		ErrorMidStreamCloseScenario(),
	}
}

// ─── vmodel error integration helpers ─────────────────────────────────────────────

// BuildErrorFromSpec creates a MockResponseBuilder from a vmodel.SharedMockSpec
// for the specified protocol format. This enables the benchmark to use the same
// error definitions as vmodel, ensuring consistency across testing and production.
func BuildErrorFromSpec(format ResponseFormat, spec vmodel.SharedMockSpec) MockResponseBuilder {
	if spec.Error == nil {
		return MockResponseBuilder{} // No error configured
	}

	// Mid-stream failures are not HTTP-level errors: the upstream starts a
	// normal 200 response, emits partial content, then the connection drops. We
	// model that as a truncated content stream so the gateway forwards a 200
	// with the partial body (rather than an error envelope).
	if spec.Error.Stage == vmodel.ErrorStageMidStream {
		return buildMidStreamTruncated(format, spec.Content)
	}

	switch format {
	case FormatOpenAIChat, FormatOpenAIResponses:
		return buildOpenAIError(spec.Error)
	case FormatAnthropic:
		return buildAnthropicError(spec.Error)
	case FormatGoogle:
		return buildGoogleError(spec.Error)
	default:
		return MockResponseBuilder{}
	}
}

// buildMidStreamTruncated produces a 200 response whose streaming variant emits
// partial content and then stops without the terminal frames ([DONE] /
// message_stop), simulating a provider connection that drops mid-stream. The
// non-streaming variant returns the same text as a complete 200 body, since a
// non-streaming request cannot observe a mid-stream cut. content is the partial
// text the client should still receive (e.g. "...this stream will be truncated").
func buildMidStreamTruncated(format ResponseFormat, content string) MockResponseBuilder {
	if content == "" {
		content = "partial content before the stream was truncated"
	}
	switch format {
	case FormatOpenAIChat:
		body := map[string]interface{}{
			"id":      "chatcmpl-validate-midstream",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": content},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18},
		}
		return MockResponseBuilder{
			NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
			Stream: func() []string {
				delta := mustMarshal(map[string]interface{}{
					"id": "chatcmpl-validate-midstream", "object": "chat.completion.chunk",
					"created": 1700000000, "model": "gpt-4o",
					"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"role": "assistant", "content": content}, "finish_reason": nil}},
				})
				// No finish_reason chunk and no `[DONE]`: the stream is cut short.
				return []string{"data: " + string(delta)}
			},
		}
	case FormatOpenAIResponses:
		body := map[string]interface{}{
			"id": "resp-validate-midstream", "object": "realtime.response", "created_at": time.Now().Unix(), "model": "gpt-4o", "status": "completed",
			"output": []map[string]interface{}{{
				"id": "item-validate-midstream", "type": "message", "role": "assistant", "status": "completed",
				"content": []map[string]interface{}{{"type": "output_text", "text": content, "annotations": []interface{}{}}},
			}},
			"usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 8, "total_tokens": 18},
		}
		return MockResponseBuilder{
			NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
			Stream: func() []string {
				created := `data: {"type":"response.created","response":{"id":"resp-validate-midstream","object":"realtime.response","created_at":1700000000,"model":"gpt-4o","status":"in_progress","output":[]}}`
				added := `data: {"type":"response.output_item.added","response_id":"resp-validate-midstream","item":{"id":"item-validate-midstream","type":"message","role":"assistant","status":"in_progress","content":[]}}`
				delta := mustMarshal(map[string]interface{}{"type": "response.output_text.delta", "response_id": "resp-validate-midstream", "item_id": "item-validate-midstream", "output_index": 0, "content_index": 0, "delta": content})
				// No response.output_text.done / response.completed / [DONE]: cut short.
				return []string{created, added, "data: " + string(delta)}
			},
		}
	case FormatAnthropic:
		body := map[string]interface{}{
			"id": "msg-validate-midstream", "type": "message", "role": "assistant",
			"content":     []map[string]interface{}{{"type": "text", "text": content}},
			"model":       "claude-3-5-sonnet-20241022",
			"stop_reason": "end_turn", "stop_sequence": nil,
			"usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 8},
		}
		return MockResponseBuilder{
			NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
			Stream: func() []string {
				start := mustMarshal(map[string]interface{}{"type": "message_start", "message": map[string]interface{}{"id": "msg-validate-midstream", "type": "message", "role": "assistant", "content": []interface{}{}, "model": "claude-3-5-sonnet-20241022", "stop_reason": nil, "usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 0}}})
				delta := mustMarshal(map[string]interface{}{"type": "content_block_delta", "index": 0, "delta": map[string]interface{}{"type": "text_delta", "text": content}})
				// No content_block_stop / message_delta / message_stop: cut short.
				return []string{
					"event: message_start", "data: " + string(start),
					"event: content_block_start", `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
					"event: content_block_delta", "data: " + string(delta),
				}
			},
		}
	case FormatGoogle:
		body := map[string]interface{}{
			"candidates": []map[string]interface{}{{
				"content": map[string]interface{}{"role": "model", "parts": []map[string]interface{}{{"text": content}}},
				"index":   0,
			}},
			"usageMetadata": map[string]interface{}{"promptTokenCount": 10, "candidatesTokenCount": 8, "totalTokenCount": 18},
		}
		return MockResponseBuilder{
			NonStream: func() (int, []byte) { return 200, mustMarshal(body) },
			Stream: func() []string {
				chunk := mustMarshal(map[string]interface{}{"candidates": []map[string]interface{}{{"content": map[string]interface{}{"role": "model", "parts": []map[string]interface{}{{"text": content}}}, "index": 0}}})
				return []string{"data: " + string(chunk)}
			},
		}
	default:
		return MockResponseBuilder{}
	}
}

// buildOpenAIError creates an OpenAI-shaped error response from vmodel.ErrorInjection.
func buildOpenAIError(err *vmodel.ErrorInjection) MockResponseBuilder {
	status, message, typ := normalizeErrorSpec(err)

	body := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    typ,
		},
	}

	// Pre-compute stream payload to avoid repeated marshaling
	streamPayload, _ := json.Marshal(body["error"])
	streamLine := fmt.Sprintf("data: %s", string(streamPayload))

	return MockResponseBuilder{
		NonStream:       func() (int, []byte) { return status, mustMarshal(body) },
		Stream:          func() []string { return []string{streamLine} },
		StreamHTTPError: preContentStreamStatus(err, status),
	}
}

// preContentStreamStatus returns the HTTP status a streaming request should fail
// with for a pre-content error injection, or 0 for mid-stream injections (which
// must reply 200 and surface the failure inside the SSE stream).
func preContentStreamStatus(err *vmodel.ErrorInjection, status int) int {
	if err != nil && err.Stage == vmodel.ErrorStagePreContent {
		return status
	}
	return 0
}

// normalizeErrorSpec centralizes default value logic for error injection fields.
// Returns (status, message, type) with defaults applied for zero values.
func normalizeErrorSpec(err *vmodel.ErrorInjection) (status int, message string, typ string) {
	status = err.Status
	if status == 0 {
		status = 500
	}
	message = err.Message
	if message == "" {
		message = "simulated error"
	}
	typ = err.Type
	if typ == "" {
		typ = "api_error"
	}
	return status, message, typ
}

// buildAnthropicError creates an Anthropic-shaped error response from vmodel.ErrorInjection.
func buildAnthropicError(err *vmodel.ErrorInjection) MockResponseBuilder {
	status, message, typ := normalizeErrorSpec(err)

	body := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    typ,
			"message": message,
		},
	}

	// Pre-compute stream payload to avoid repeated marshaling
	streamPayload, _ := json.Marshal(body)
	streamLines := []string{
		"event: error",
		fmt.Sprintf("data: %s", string(streamPayload)),
	}

	return MockResponseBuilder{
		NonStream:       func() (int, []byte) { return status, mustMarshal(body) },
		Stream:          func() []string { return streamLines },
		StreamHTTPError: preContentStreamStatus(err, status),
	}
}

// buildGoogleError creates a Google-shaped error response from vmodel.ErrorInjection.
func buildGoogleError(err *vmodel.ErrorInjection) MockResponseBuilder {
	status, message, _ := normalizeErrorSpec(err)

	body := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    status,
			"message": message,
			"status":  "api_error",
		},
	}

	// Pre-compute stream payload to avoid repeated marshaling
	streamPayload, _ := json.Marshal(body["error"])
	streamLine := fmt.Sprintf("data: %s", string(streamPayload))

	return MockResponseBuilder{
		NonStream:       func() (int, []byte) { return status, mustMarshal(body) },
		Stream:          func() []string { return []string{streamLine} },
		StreamHTTPError: preContentStreamStatus(err, status),
	}
}

// GetErrorSpec returns a vmodel error spec by ID from the built-in error models.
// Searches SharedDefaultMocks() (basic errors) and ExtendedErrorSpecs().
// Results are cached for O(1) subsequent access. Returns zero value if not found.
func GetErrorSpec(id string) vmodel.SharedMockSpec {
	// Initialize cache on first use
	errorSpecCacheOnce.Do(func() {
		errorSpecCache = make(map[string]vmodel.SharedMockSpec)

		// Cache basic error models from SharedDefaultMocks
		for _, spec := range vmodel.SharedDefaultMocks() {
			if spec.Error != nil {
				errorSpecCache[spec.ID] = spec
			}
		}

		// Cache extended error models
		for _, spec := range vmodel.ExtendedErrorSpecs() {
			errorSpecCache[spec.ID] = spec
		}
	})

	errorSpecCacheMu.RLock()
	defer errorSpecCacheMu.RUnlock()

	return errorSpecCache[id]
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshal: %v", err))
	}
	return b
}
