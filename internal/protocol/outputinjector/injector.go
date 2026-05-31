// Package outputinjector is the protocol-agnostic hook layer that lets a
// business object rewrite the first text-bearing event in a response stream
// (or the first text content block of a non-stream response).
//
// Business implementations (e.g. server/output/VisionTextPrefix) implement
// the OutputInjector interface; this package owns the per-protocol parsing
// of "is this event a text-bearing event, and where does the text live"
// and the in-place mutation via sjson / typed-field writes. The injector
// itself knows nothing about which protocol it is running under — it only
// supplies a prefix string the first time it is asked.
//
// Wire-up:
//   - For streams: AttachToHandleContext registers a single raw-event hook
//     on hc.OnStreamRawEventHooks; the hook covers all four protocols via
//     the event-type switch in this package.
//   - For non-stream: handlers call PrependToNonStreamResponse(inj, resp)
//     just before c.JSON(resp) is issued.
package outputinjector

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// OutputInjector is the smallest contract a business object needs to take
// part in output injection: supply the text to prepend the first time it is
// asked, and "" thereafter. Implementations track their own injected state.
type OutputInjector interface {
	// PrefixText returns the prefix to insert the first time it is consumed;
	// subsequent calls return "". Returning "" at any time means "no
	// injection for this event" and the bytes pass through unchanged.
	PrefixText() string
}

// AttachToHandleContext wires the injector into BOTH response paths:
//   - the raw-bytes stream hook chain (covers all four protocol stream paths
//     via the event-type switch below);
//   - the non-stream response hook chain (fires once per non-stream resp).
//
// No-op when inj or hc is nil. Mutation happens via sjson on the JSON bytes
// of each stream event, or in-place struct/map writes on non-stream resps.
func AttachToHandleContext(hc *protocol.HandleContext, inj OutputInjector) {
	if inj == nil || hc == nil {
		return
	}
	hc.WithOnStreamRawEvent(func(eventType string, eventRaw []byte) ([]byte, error) {
		modified, _ := prependToStreamEvent(inj, eventType, eventRaw)
		return modified, nil
	})
	hc.WithOnNonStreamResponse(func(resp any) {
		_ = PrependToNonStreamResponse(inj, resp)
	})
}

// prependToStreamEvent inspects a single stream event and, if it is the
// first text-bearing event, prepends inj.PrefixText() into the relevant
// JSON field. Returns (possibly-modified bytes, did-inject).
//
// Event-type routing covers:
//   - Anthropic content_block_delta with delta.type == "text_delta"
//     -> field "delta.text"
//   - OpenAI Chat completion chunk with choices[0].delta.content set
//     -> field "choices.0.delta.content"
//   - OpenAI Responses response.output_text.delta event
//     -> field "delta"
//   - OpenAI Responses response.output_item.done / response.completed:
//     not handled here — by the time these arrive the first text delta
//     has already been seen (or there was none, e.g. tool-only response).
func prependToStreamEvent(inj OutputInjector, eventType string, eventRaw []byte) ([]byte, bool) {
	switch eventType {
	case "content_block_delta":
		// Anthropic v1 / Beta: only the text_delta variant carries text.
		if gjson.GetBytes(eventRaw, "delta.type").String() != "text_delta" {
			return eventRaw, false
		}
		prefix := inj.PrefixText()
		if prefix == "" {
			return eventRaw, false
		}
		existing := gjson.GetBytes(eventRaw, "delta.text").String()
		modified, err := sjson.SetBytes(eventRaw, "delta.text", prefix+existing)
		if err != nil {
			return eventRaw, false
		}
		return modified, true

	case "chat.completion.chunk":
		// OpenAI Chat: prepend to the first non-empty content delta. Skip
		// chunks with no content (role-only, finish, tool_calls-only, ...).
		content := gjson.GetBytes(eventRaw, "choices.0.delta.content")
		if !content.Exists() || content.Type == gjson.Null {
			return eventRaw, false
		}
		existing := content.String()
		if existing == "" {
			return eventRaw, false
		}
		prefix := inj.PrefixText()
		if prefix == "" {
			return eventRaw, false
		}
		modified, err := sjson.SetBytes(eventRaw, "choices.0.delta.content", prefix+existing)
		if err != nil {
			return eventRaw, false
		}
		return modified, true

	case "response.output_text.delta":
		// OpenAI Responses: text deltas arrive as standalone events with a
		// top-level "delta" string field.
		delta := gjson.GetBytes(eventRaw, "delta")
		if !delta.Exists() || delta.String() == "" {
			return eventRaw, false
		}
		prefix := inj.PrefixText()
		if prefix == "" {
			return eventRaw, false
		}
		modified, err := sjson.SetBytes(eventRaw, "delta", prefix+delta.String())
		if err != nil {
			return eventRaw, false
		}
		return modified, true
	}
	return eventRaw, false
}

// PrependToNonStreamResponse mutates the response struct/map in place by
// prepending inj.PrefixText() to the first text content. Returns true if a
// mutation happened. No-op when inj is nil or the response carries no
// text-bearing slot (e.g. tool-only assistant message).
//
// Supports:
//   - *anthropic.Message              (Anthropic v1 non-stream)
//   - *anthropic.BetaMessage          (Anthropic Beta non-stream)
//   - *openai.ChatCompletion          (OpenAI Chat typed response)
//   - map[string]interface{}          (OpenAI Chat passthrough responseMap)
//   - *responses.Response             (OpenAI Responses non-stream)
func PrependToNonStreamResponse(inj OutputInjector, resp any) bool {
	if inj == nil || resp == nil {
		return false
	}
	switch r := resp.(type) {
	case *anthropic.Message:
		return prependAnthropicMessage(inj, r)
	case *anthropic.BetaMessage:
		return prependAnthropicBetaMessage(inj, r)
	case *openai.ChatCompletion:
		return prependOpenAIChatCompletion(inj, r)
	case map[string]interface{}:
		return prependOpenAIChatMap(inj, r)
	case *responses.Response:
		return prependOpenAIResponsesResponse(inj, r)
	}
	return false
}

func prependAnthropicMessage(inj OutputInjector, m *anthropic.Message) bool {
	for i := range m.Content {
		if m.Content[i].Type != "text" {
			continue
		}
		prefix := inj.PrefixText()
		if prefix == "" {
			return false
		}
		m.Content[i].Text = prefix + m.Content[i].Text
		return true
	}
	return false
}

func prependAnthropicBetaMessage(inj OutputInjector, m *anthropic.BetaMessage) bool {
	for i := range m.Content {
		if m.Content[i].Type != "text" {
			continue
		}
		prefix := inj.PrefixText()
		if prefix == "" {
			return false
		}
		m.Content[i].Text = prefix + m.Content[i].Text
		return true
	}
	return false
}

func prependOpenAIChatCompletion(inj OutputInjector, r *openai.ChatCompletion) bool {
	if len(r.Choices) == 0 {
		return false
	}
	prefix := inj.PrefixText()
	if prefix == "" {
		return false
	}
	r.Choices[0].Message.Content = prefix + r.Choices[0].Message.Content
	return true
}

func prependOpenAIChatMap(inj OutputInjector, m map[string]interface{}) bool {
	choices, ok := m["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return false
	}
	choice0, ok := choices[0].(map[string]interface{})
	if !ok {
		return false
	}
	msg, ok := choice0["message"].(map[string]interface{})
	if !ok {
		return false
	}
	existing, _ := msg["content"].(string)
	prefix := inj.PrefixText()
	if prefix == "" {
		return false
	}
	msg["content"] = prefix + existing
	return true
}

func prependOpenAIResponsesResponse(inj OutputInjector, r *responses.Response) bool {
	for i := range r.Output {
		item := &r.Output[i]
		// Only the "message" output type carries text content blocks.
		if item.Type != "message" {
			continue
		}
		for j := range item.Content {
			if item.Content[j].Type != "output_text" {
				continue
			}
			prefix := inj.PrefixText()
			if prefix == "" {
				return false
			}
			item.Content[j].Text = prefix + item.Content[j].Text
			return true
		}
	}
	return false
}
