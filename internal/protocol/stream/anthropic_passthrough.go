package stream

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ===================================================================
// Anthropic Handle Functions
// ===================================================================

// HandleAnthropicV1Stream handles Anthropic v1 streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1Stream(hc *protocol.HandleContext, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion]) (*protocol.TokenUsage, error) {
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
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.MessageStreamEventUnion)

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

			if handled, err := rewriteAnthropicGuardrailsEvent(hc.GinContext, false, evt.Type, int(evt.Index), evt.ContentBlock, evt); err != nil {
				return err
			} else if handled {
				return nil
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
			logrus.Debug("Anthropic v1 stream canceled by client")
			if !hasUsage {
				return protocol.ZeroTokenUsage(), nil
			}
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
	}

	if err := injectAnthropicGuardrailsBlock(hc.GinContext, false); err != nil {
		logrus.Debugf("Guardrails inject error: %v", err)
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}

// HandleAnthropicV1BetaStream handles Anthropic v1 beta streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1BetaStream(hc *protocol.HandleContext, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (*protocol.TokenUsage, error) {
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
				return false, nil, nil
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

			if handled, err := rewriteAnthropicGuardrailsEvent(hc.GinContext, true, evt.Type, int(evt.Index), evt.ContentBlock, evt); err != nil {
				return err
			} else if handled {
				return nil
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
			logrus.Debug("Anthropic v1 beta stream canceled by client")
			if !hasUsage {
				return protocol.ZeroTokenUsage(), nil
			}
			return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), err
	}

	if err := injectAnthropicGuardrailsBlock(hc.GinContext, true); err != nil {
		logrus.Debugf("Guardrails inject error: %v", err)
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}

// ===================================================================
// OpenAI Handle Functions
// ===================================================================
