package stream

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// HandleAnthropic handles Anthropic v1 streaming response.
// Returns (UsageStat, error)
func HandleAnthropic(hc *protocol.HandleContext, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion]) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	acc := usage.NewAnthropicAccumulator()
	var sawMessageStart, sawMessageStop bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if streamResp.Err() != nil {
				return false, streamResp.Err(), nil
			}
			if !streamResp.Next() {
				// Surface an error that the SDK only set during this Next()
				// (e.g. an in-band SSE error event or a pre-content upstream
				// failure) so the handler can emit a retryable status instead
				// of a clean finish.
				return false, streamResp.Err(), nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.MessageStreamEventUnion)
			switch evt.Type {
			case "message_start":
				sawMessageStart = true
			case "message_stop":
				sawMessageStop = true
			}

			acc.Consume(evt)

			if hc.Guardrails != nil && hc.Guardrails.Enabled {
				if handled, rewritten, err := guardrailsmutate.RewriteAnthropicToolUseEvent(hc.Guardrails.CredentialMask, hc.Guardrails.Stream, evt); err != nil {
					return err
				} else if handled {
					for _, rewrittenEvent := range rewritten {
						sendAnthropicStreamEvent(hc.GinContext, rewrittenEvent.EventType, rewrittenEvent.Payload, hc.GinContext.Writer)
					}
					return nil
				}
			}

			// For message_start events, modify the model in the raw JSON
			// to preserve the original API response structure
			if evt.Type == "message_start" {
				var eventMap map[string]json.RawMessage
				if err := json.Unmarshal([]byte(evt.RawJSON()), &eventMap); err == nil {
					var msgMap map[string]json.RawMessage
					if err := json.Unmarshal(eventMap["message"], &msgMap); err == nil {
						msgMap["model"] = json.RawMessage(`"` + hc.ResponseModel + `"`)
						eventMap["message"], _ = json.Marshal(msgMap)
					}
					modified, _ := json.Marshal(eventMap)
					hc.GinContext.SSEvent(evt.Type, string(modified))
				} else {
					hc.GinContext.SSEvent(evt.Type, evt.RawJSON())
				}
			} else {
				hc.GinContext.SSEvent(evt.Type, evt.RawJSON())
			}
			hc.GinContext.Writer.Flush()
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.WithContext(hc.GinContext.Request.Context()).Debug("Anthropic v1 stream canceled by client")
			return acc.Result(), nil
		}

		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier,
		// instead of a 200 SSE error event.
		if !hc.GinContext.Writer.Written() {
			SendStreamingError(hc.GinContext, err)
			return acc.Result(), err
		}
		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return acc.Result(), err
	}

	// Upstream cut mid-stream (content started but never terminated): surface
	// an honest error event rather than fabricating a clean message_stop. Real
	// SDK clients raise on it (the turn was truncated); lenient clients keep
	// the partial content already sent. Cleanly finished streams already
	// forwarded their own message_delta / message_stop.
	if sawMessageStart && !sawMessageStop {
		MarshalAndSendErrorEvent(hc.GinContext, "upstream stream ended before completion", "stream_error", "incomplete_stream")
	}

	return acc.Result(), nil
}

// HandleAnthropicBeta handles Anthropic v1 beta streaming response.
// Returns (UsageStat, error)
func HandleAnthropicBeta(hc *protocol.HandleContext, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	acc := usage.NewAnthropicAccumulator()
	var sawMessageStart, sawMessageStop bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if streamResp.Err() != nil {
				return false, streamResp.Err(), nil
			}
			if !streamResp.Next() {
				// Surface an error that the SDK only set during this Next()
				// (e.g. an in-band SSE error event or a pre-content upstream
				// failure) so the handler can emit a retryable status instead
				// of a clean finish.
				return false, streamResp.Err(), nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.BetaRawMessageStreamEventUnion)
			switch evt.Type {
			case "message_start":
				sawMessageStart = true
			case "message_stop":
				sawMessageStop = true
			}

			acc.ConsumeBeta(evt)

			if hc.Guardrails != nil && hc.Guardrails.Enabled {
				if handled, rewritten, err := guardrailsmutate.RewriteAnthropicToolUseEvent(hc.Guardrails.CredentialMask, hc.Guardrails.Stream, evt); err != nil {
					return err
				} else if handled {
					for _, rewrittenEvent := range rewritten {
						sendAnthropicStreamEvent(hc.GinContext, rewrittenEvent.EventType, rewrittenEvent.Payload, hc.GinContext.Writer)
					}
					return nil
				}
			}

			// For message_start events, modify the model in the raw JSON
			// to preserve the original API response structure
			if evt.Type == "message_start" {
				var eventMap map[string]json.RawMessage
				if err := json.Unmarshal([]byte(evt.RawJSON()), &eventMap); err == nil {
					var msgMap map[string]json.RawMessage
					if err := json.Unmarshal(eventMap["message"], &msgMap); err == nil {
						msgMap["model"] = json.RawMessage(`"` + hc.ResponseModel + `"`)
						eventMap["message"], _ = json.Marshal(msgMap)
					}
					modified, _ := json.Marshal(eventMap)
					hc.GinContext.SSEvent(evt.Type, string(modified))
				} else {
					hc.GinContext.SSEvent(evt.Type, evt.RawJSON())
				}
			} else {
				hc.GinContext.SSEvent(evt.Type, evt.RawJSON())
			}
			hc.GinContext.Writer.Flush()
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.WithContext(hc.GinContext.Request.Context()).Debug("Anthropic v1 beta stream canceled by client")
			return acc.Result(), nil
		}

		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier,
		// instead of a 200 SSE error event.
		if !hc.GinContext.Writer.Written() {
			SendStreamingError(hc.GinContext, err)
			return acc.Result(), err
		}
		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return acc.Result(), err
	}

	// See HandleAnthropic: surface an honest error event when the upstream was
	// cut after content started.
	if sawMessageStart && !sawMessageStop {
		MarshalAndSendErrorEvent(hc.GinContext, "upstream stream ended before completion", "stream_error", "incomplete_stream")
	}

	return acc.Result(), nil
}
