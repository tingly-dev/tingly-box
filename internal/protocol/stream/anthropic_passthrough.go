package stream

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// HandleAnthropic handles Anthropic v1 streaming response.
// Returns (UsageStat, error)
func HandleAnthropic(hc *protocol.HandleContext, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion]) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens, cacheTokens int
	var hasUsage bool

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

			// Read usage from SDK struct; fall back to raw JSON for providers
			// or test decoders where apijson doesn't populate struct fields.
			raw := evt.RawJSON()
			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			} else if v := gjson.Get(raw, "usage.input_tokens"); v.Int() > 0 {
				inputTokens = int(v.Int())
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			} else if v := gjson.Get(raw, "usage.output_tokens"); v.Int() > 0 {
				outputTokens = int(v.Int())
				hasUsage = true
			}
			if evt.Usage.CacheReadInputTokens > 0 {
				cacheTokens = int(evt.Usage.CacheReadInputTokens)
				hasUsage = true
			} else if v := gjson.Get(raw, "usage.cache_read_input_tokens"); v.Int() > 0 {
				cacheTokens = int(v.Int())
				hasUsage = true
			}

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
			if !hasUsage {
				return protocol.ZeroTokenUsage(), nil
			}
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
		}

		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier,
		// instead of a 200 SSE error event.
		if !hc.GinContext.Writer.Written() {
			SendStreamingError(hc.GinContext, err)
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
		}
		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}

// HandleAnthropicBeta handles Anthropic v1 beta streaming response.
// Returns (UsageStat, error)
func HandleAnthropicBeta(hc *protocol.HandleContext, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (*protocol.TokenUsage, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	var inputTokens, outputTokens, cacheTokens int
	var hasUsage bool

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

			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			}
			if evt.Usage.CacheReadInputTokens > 0 {
				cacheTokens = int(evt.Usage.CacheReadInputTokens)
				hasUsage = true
			}

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
			if !hasUsage {
				return protocol.ZeroTokenUsage(), nil
			}
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
		}

		// Stream failed before any content reached the client: surface a
		// retryable 5xx so mid-request failover can try the next tier,
		// instead of a 200 SSE error event.
		if !hc.GinContext.Writer.Written() {
			SendStreamingError(hc.GinContext, err)
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
		}
		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}
