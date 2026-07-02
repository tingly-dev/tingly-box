package stream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

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
	streamClosed := false
	defer func() {
		if !streamClosed {
			streamResp.Close()
		}
	}()
	defer hc.ReleaseStreamState()

	hc.SetupSSEHeaders()

	acc := usage.NewAnthropicAccumulator()
	var sawMessageStart, sawMessageStop bool
	var processErr error

	protocol.RunLoop(hc.GinContext, func(_ io.Writer) bool {
		select {
		case <-hc.GinContext.Request.Context().Done():
			return false
		default:
		}

		// Pre-check: surface an error the SDK set before the current call
		// (e.g. from a previous Close or an in-band SSE error event).
		if streamResp.Err() != nil {
			processErr = streamResp.Err()
			return false
		}

		if !streamResp.Next() {
			// Explicit close on stream end matches the original nextFunc
			// behavior: Close() before returning to surface transport errors.
			streamClosed = true
			if err := streamResp.Close(); err != nil {
				processErr = err
			} else if streamResp.Err() != nil {
				processErr = streamResp.Err()
			}
			return false
		}

		event := streamResp.Current()
		evt := &event

		// Call stream event hooks (recorder, guardrails)
		for _, hook := range hc.OnStreamEventHooks {
			if hookErr := hook(evt); hookErr != nil {
				processErr = hookErr
				return false
			}
		}

		switch evt.Type {
		case "message_start":
			sawMessageStart = true
		case "message_stop":
			sawMessageStop = true
		}

		acc.Consume(evt)

		// This passthrough writes raw upstream bytes via c.SSEvent, bypassing
		// sendAnthropicStreamEvent, so mark TTFT here on the first content delta.
		if isAnthropicContentDeltaEvent(evt.Type) {
			protocol.MarkFirstToken(hc.GinContext)
		}

		if hc.Guardrails != nil && hc.Guardrails.Enabled {
			if handled, rewritten, err := guardrailsmutate.RewriteAnthropicToolUseEvent(hc.Guardrails.CredentialMask, hc.Guardrails.Stream, evt); err != nil {
				processErr = err
				return false
			} else if handled {
				for _, rewrittenEvent := range rewritten {
					sendAnthropicStreamEvent(hc.GinContext, rewrittenEvent.EventType, rewrittenEvent.Payload, hc.GinContext.Writer)
				}
				hc.GinContext.Writer.Flush()
				return true
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
				hc.GinContext.SSEvent(evt.Type, strings.Clone(evt.RawJSON()))
			}
		} else {
			hc.GinContext.SSEvent(evt.Type, strings.Clone(evt.RawJSON()))
		}
		hc.GinContext.Writer.Flush()
		return true
	})

	if processErr != nil {
		for _, hook := range hc.OnStreamErrorHooks {
			hook(processErr)
		}
		if errors.Is(processErr, context.Canceled) || protocol.IsContextCanceled(processErr) {
			logrus.WithContext(hc.GinContext.Request.Context()).Debug("Anthropic v1 stream canceled by client")
			return acc.Result(), nil
		}
		if !hc.GinContext.Writer.Written() {
			SendAnthropicStreamingError(hc.GinContext, processErr)
			return acc.Result(), processErr
		}
		SendAnthropicStreamErrorEvent(hc.GinContext, processErr, protocol.AnthropicErrAPI)
		return acc.Result(), processErr
	}

	// Upstream cut mid-stream (content started but never terminated): surface
	// an honest error event rather than fabricating a clean message_stop. Real
	// SDK clients raise on it (the turn was truncated); lenient clients keep
	// the partial content already sent. Cleanly finished streams already
	// forwarded their own message_delta / message_stop.
	if sawMessageStart && !sawMessageStop {
		SendAnthropicStreamErrorEvent(hc.GinContext, errors.New("upstream stream ended before completion"), protocol.AnthropicErrAPI)
	}

	for _, hook := range hc.OnStreamCompleteHooks {
		hook()
	}

	return acc.Result(), nil
}

// HandleAnthropicBeta handles Anthropic v1 beta streaming response.
// Returns (UsageStat, error)
func HandleAnthropicBeta(hc *protocol.HandleContext, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (*protocol.TokenUsage, error) {
	streamClosed := false
	defer func() {
		if !streamClosed {
			streamResp.Close()
		}
	}()
	defer hc.ReleaseStreamState()

	hc.SetupSSEHeaders()

	acc := usage.NewAnthropicAccumulator()
	var sawMessageStart, sawMessageStop bool

	var processErr error

	protocol.RunLoop(hc.GinContext, func(_ io.Writer) bool {
		select {
		case <-hc.GinContext.Request.Context().Done():
			return false
		default:
		}

		// Pre-check: surface an error the SDK set before the current call
		// (e.g. from a previous Close or an in-band SSE error event).
		if streamResp.Err() != nil {
			processErr = streamResp.Err()
			return false
		}

		if !streamResp.Next() {
			// Explicit close on stream end matches the original nextFunc
			// behavior: Close() before returning to surface transport errors.
			streamClosed = true
			if err := streamResp.Close(); err != nil {
				processErr = err
			} else if streamResp.Err() != nil {
				processErr = streamResp.Err()
			}
			return false
		}

		event := streamResp.Current()
		evt := &event

		// Call stream event hooks (recorder, guardrails)
		for _, hook := range hc.OnStreamEventHooks {
			if hookErr := hook(evt); hookErr != nil {
				processErr = hookErr
				return false
			}
		}

		switch evt.Type {
		case "message_start":
			sawMessageStart = true
		case "message_stop":
			sawMessageStop = true
		}

		acc.ConsumeBeta(evt)

		// This passthrough writes raw upstream bytes via c.SSEvent, bypassing
		// sendAnthropicStreamEvent, so mark TTFT here on the first content delta.
		if isAnthropicContentDeltaEvent(evt.Type) {
			protocol.MarkFirstToken(hc.GinContext)
		}

		if hc.Guardrails != nil && hc.Guardrails.Enabled {
			if handled, rewritten, err := guardrailsmutate.RewriteAnthropicToolUseEvent(hc.Guardrails.CredentialMask, hc.Guardrails.Stream, evt); err != nil {
				processErr = err
				return false
			} else if handled {
				for _, rewrittenEvent := range rewritten {
					sendAnthropicStreamEvent(hc.GinContext, rewrittenEvent.EventType, rewrittenEvent.Payload, hc.GinContext.Writer)
				}
				hc.GinContext.Writer.Flush()
				return true
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
				hc.GinContext.SSEvent(evt.Type, strings.Clone(evt.RawJSON()))
			}
		} else {
			hc.GinContext.SSEvent(evt.Type, strings.Clone(evt.RawJSON()))
		}
		hc.GinContext.Writer.Flush()
		return true
	})

	if processErr != nil {
		for _, hook := range hc.OnStreamErrorHooks {
			hook(processErr)
		}
		if errors.Is(processErr, context.Canceled) || protocol.IsContextCanceled(processErr) {
			logrus.WithContext(hc.GinContext.Request.Context()).Debug("Anthropic v1 beta stream canceled by client")
			return acc.Result(), nil
		}
		if !hc.GinContext.Writer.Written() {
			SendAnthropicStreamingError(hc.GinContext, processErr)
			return acc.Result(), processErr
		}
		SendAnthropicStreamErrorEvent(hc.GinContext, processErr, protocol.AnthropicErrAPI)
		return acc.Result(), processErr
	}

	// See HandleAnthropic: surface an honest error event when the upstream was
	// cut after content started.
	if sawMessageStart && !sawMessageStop {
		SendAnthropicStreamErrorEvent(hc.GinContext, errors.New("upstream stream ended before completion"), protocol.AnthropicErrAPI)
	}

	for _, hook := range hc.OnStreamCompleteHooks {
		hook()
	}

	return acc.Result(), nil
}
