package vmodelclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/typ"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
)

// AnthropicClient implements client.AnthropicClientInterface using the
// in-process vmodel Anthropic registry. No HTTP, no auth.
type AnthropicClient struct {
	reg      *anthropicvm.Registry
	provider *typ.Provider
}

// NewAnthropicClient creates an in-process Anthropic vmodel client backed by reg.
func NewAnthropicClient(reg *anthropicvm.Registry, provider *typ.Provider) *AnthropicClient {
	return &AnthropicClient{reg: reg, provider: provider}
}

func (c *AnthropicClient) GetProvider() *typ.Provider  { return c.provider }
func (c *AnthropicClient) APIStyle() protocol.APIStyle { return protocol.APIStyleAnthropic }
func (c *AnthropicClient) SetRecordSink(_ *obs.Sink)   {}
func (c *AnthropicClient) Client() *anthropic.Client   { return nil }
func (c *AnthropicClient) Close() error                { return nil }

func (c *AnthropicClient) ListModels(_ context.Context) ([]string, error) {
	models := c.reg.ListModels()
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids, nil
}

// BetaMessagesNew calls the vmodel synchronously and returns an
// *anthropic.BetaMessage built via JSON round-trip.
func (c *AnthropicClient) BetaMessagesNew(_ context.Context, req *anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	vm := c.reg.Get(string(req.Model))
	if vm == nil {
		return nil, fmt.Errorf("vmodel not found: %s", req.Model)
	}
	if err := injectedPreContentError(vm); err != nil {
		return nil, err
	}

	vmReq := &protocol.AnthropicBetaMessagesRequest{
		BetaMessageNewParams: req,
	}

	resp, err := vm.HandleAnthropic(vmReq)
	if err != nil {
		return nil, err
	}

	contentBlocks := betaContentToJSON(resp)
	textContent := betaTextContent(resp)

	inputTokens := token.EstimateBetaAnthropicTokens(req.Messages)
	outputTokens := token.EstimateTokensString(textContent)

	payload := map[string]interface{}{
		"id":            fmt.Sprintf("msg_virtual_%d", time.Now().UnixNano()),
		"type":          "message",
		"role":          "assistant",
		"model":         string(req.Model),
		"content":       contentBlocks,
		"stop_reason":   string(resp.StopReason),
		"stop_sequence": nil,
		"usage": map[string]interface{}{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("vmodel marshal: %w", err)
	}
	var out anthropic.BetaMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("vmodel unmarshal: %w", err)
	}
	return &out, nil
}

// BetaMessagesNewStreaming wraps vm.HandleAnthropicStream with a channel-based
// decoder so the caller gets a standard anthropicstream.Stream.
func (c *AnthropicClient) BetaMessagesNewStreaming(_ context.Context, req *anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	dec, err := c.betaStreamDecoder(req)
	if err != nil {
		return anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](anthropicErrDecoder{err}, nil)
	}
	return anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](dec, nil)
}

// MessagesNew handles v1 (non-beta) Anthropic messages by lifting the params to
// beta, delegating to BetaMessagesNew, then converting the BetaMessage result to a
// Message (both share the same wire shape for the fields vmodel produces).
func (c *AnthropicClient) MessagesNew(ctx context.Context, req *anthropic.MessageNewParams) (*anthropic.Message, error) {
	betaReq, err := v1ToBetaParams(req)
	if err != nil {
		return nil, fmt.Errorf("vmodel v1→beta lift: %w", err)
	}
	betaMsg, err := c.BetaMessagesNew(ctx, betaReq)
	if err != nil {
		return nil, err
	}
	var out anthropic.Message
	if err := jsonReround(betaMsg, &out); err != nil {
		return nil, fmt.Errorf("vmodel beta→v1: %w", err)
	}
	return &out, nil
}

// MessagesNewStreaming handles v1 streaming by lifting params to beta and reusing
// the beta decoder. MessageStreamEventUnion and BetaRawMessageStreamEventUnion share
// the same SSE wire format, so the same decoder drives both stream types.
func (c *AnthropicClient) MessagesNewStreaming(_ context.Context, req *anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	betaReq, err := v1ToBetaParams(req)
	if err == nil {
		var dec anthropicstream.Decoder
		if dec, err = c.betaStreamDecoder(betaReq); err == nil {
			return anthropicstream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)
		}
	}
	return anthropicstream.NewStream[anthropic.MessageStreamEventUnion](anthropicErrDecoder{err}, nil)
}

// betaStreamDecoder resolves the vmodel, enforces error injection, and builds the
// streaming decoder shared by the beta and v1 streaming entry points.
func (c *AnthropicClient) betaStreamDecoder(req *anthropic.BetaMessageNewParams) (anthropicstream.Decoder, error) {
	vm := c.reg.Get(string(req.Model))
	if vm == nil {
		return nil, fmt.Errorf("vmodel not found: %s", req.Model)
	}
	if err := injectedPreContentError(vm); err != nil {
		return nil, err
	}
	return newAnthropicVModelDecoder(vm, &protocol.AnthropicBetaMessagesRequest{BetaMessageNewParams: req}), nil
}

// v1ToBetaParams converts MessageNewParams → BetaMessageNewParams via JSON
// round-trip. Both share the same wire structure; beta adds optional betas[] which
// is absent from v1 but harmless to omit.
func v1ToBetaParams(req *anthropic.MessageNewParams) (*anthropic.BetaMessageNewParams, error) {
	var out anthropic.BetaMessageNewParams
	if err := jsonReround(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// jsonReround marshals src and unmarshals it into dst — the SDK union types have no
// public constructors, so JSON is the only safe way to convert between the parallel
// v1/beta shapes.
func jsonReround(src, dst any) error {
	raw, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func (c *AnthropicClient) MessagesCountTokens(_ context.Context, _ *anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	return nil, fmt.Errorf("count tokens not supported by vmodel")
}

func (c *AnthropicClient) BetaMessagesCountTokens(_ context.Context, _ *anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	return nil, fmt.Errorf("count tokens not supported by vmodel")
}

// ── streaming decoder ─────────────────────────────────────────────────────────

type anthropicVModelDecoder struct {
	ch      <-chan anthropicEvent
	current anthropicEvent
	err     error
}

type anthropicEvent struct {
	eventType string
	data      []byte
}

func newAnthropicVModelDecoder(vm anthropicvm.VirtualModel, req *protocol.AnthropicBetaMessagesRequest) *anthropicVModelDecoder {
	ch := make(chan anthropicEvent, 64)
	d := &anthropicVModelDecoder{ch: ch}

	msgID := fmt.Sprintf("msg_virtual_%d", time.Now().UnixNano())
	model := string(req.Model)

	go func() {
		defer close(ch)

		emit := func(eventType string, payload map[string]interface{}) {
			b, err := json.Marshal(payload)
			if err != nil {
				return
			}
			ch <- anthropicEvent{eventType: eventType, data: b}
		}

		// Track started content blocks for proper bracketing.
		startedBlocks := map[int]bool{}
		var startOrder []int

		startBlock := func(index int, contentBlock map[string]interface{}) {
			if startedBlocks[index] {
				return
			}
			startedBlocks[index] = true
			startOrder = append(startOrder, index)
			emit("content_block_start", map[string]interface{}{
				"type":          "content_block_start",
				"index":         index,
				"content_block": contentBlock,
			})
		}
		stopAllBlocks := func() {
			for _, idx := range startOrder {
				emit("content_block_stop", map[string]interface{}{
					"type":  "content_block_stop",
					"index": idx,
				})
			}
			startOrder = nil
			startedBlocks = map[int]bool{}
		}

		// Emit message_start first.
		emit("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":            msgID,
				"type":          "message",
				"role":          "assistant",
				"model":         model,
				"content":       []interface{}{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
			},
		})

		var explicitUsage *anthropicUsageCapture
		var stopReason string
		var accumulatedText string

		streamErr := vm.HandleAnthropicStream(req, func(ev any) {
			switch e := ev.(type) {
			case anthropicvm.StreamStartEvent:
				// message_start was already emitted above; skip duplicate.

			case anthropicvm.TextDeltaEvent:
				accumulatedText += e.Text
				startBlock(e.Index, map[string]interface{}{"type": "text", "text": ""})
				emit("content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": e.Index,
					"delta": map[string]interface{}{"type": "text_delta", "text": e.Text},
				})

			case anthropicvm.ThinkingDeltaEvent:
				startBlock(e.Index, map[string]interface{}{"type": "thinking", "thinking": ""})
				emit("content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": e.Index,
					"delta": map[string]interface{}{"type": "thinking_delta", "thinking": e.Thinking},
				})

			case anthropicvm.ToolUseEvent:
				if !startedBlocks[e.Index] {
					startedBlocks[e.Index] = true
					startOrder = append(startOrder, e.Index)
					emit("content_block_start", map[string]interface{}{
						"type":  "content_block_start",
						"index": e.Index,
						"content_block": map[string]interface{}{
							"type":  "tool_use",
							"id":    e.ID,
							"name":  e.Name,
							"input": json.RawMessage(e.Input),
						},
					})
				}

			case anthropicvm.UsageEvent:
				u := e.Usage
				explicitUsage = &anthropicUsageCapture{
					input:    u.PromptTokens,
					output:   u.CompletionTokens,
					cached:   u.CachedInputTokens,
					creation: u.CacheCreationInputTokens,
				}

			case anthropicvm.DoneEvent:
				stopReason = e.StopReason
				stopAllBlocks()

				usageMap := map[string]interface{}{}
				if explicitUsage != nil {
					usageMap["input_tokens"] = explicitUsage.input
					usageMap["output_tokens"] = explicitUsage.output
					if explicitUsage.cached > 0 {
						usageMap["cache_read_input_tokens"] = explicitUsage.cached
					}
					if explicitUsage.creation > 0 {
						usageMap["cache_creation_input_tokens"] = explicitUsage.creation
					}
				} else {
					usageMap["input_tokens"] = token.EstimateBetaAnthropicTokens(req.Messages)
					usageMap["output_tokens"] = token.EstimateTokensString(accumulatedText)
				}

				emit("message_delta", map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason":   stopReason,
						"stop_sequence": nil,
					},
					"usage": usageMap,
				})

				emit("message_stop", map[string]interface{}{
					"type": "message_stop",
				})
			}
		})

		if streamErr != nil {
			d.err = streamErr
		}
	}()

	return d
}

type anthropicUsageCapture struct {
	input, output, cached, creation int64
}

func (d *anthropicVModelDecoder) Next() bool {
	ev, ok := <-d.ch
	if !ok {
		return false
	}
	d.current = ev
	return true
}

func (d *anthropicVModelDecoder) Event() anthropicstream.Event {
	return anthropicstream.Event{Type: d.current.eventType, Data: d.current.data}
}

func (d *anthropicVModelDecoder) Close() error { return nil }
func (d *anthropicVModelDecoder) Err() error   { return d.err }

// anthropicErrDecoder is a no-op decoder that immediately returns an error.
type anthropicErrDecoder struct{ err error }

func (e anthropicErrDecoder) Next() bool                   { return false }
func (e anthropicErrDecoder) Event() anthropicstream.Event { return anthropicstream.Event{} }
func (e anthropicErrDecoder) Close() error                 { return nil }
func (e anthropicErrDecoder) Err() error                   { return e.err }

// ── content conversion helpers ────────────────────────────────────────────────

func betaContentToJSON(resp anthropicvm.VModelResponse) []interface{} {
	out := make([]interface{}, 0, len(resp.Content))
	for _, blk := range resp.Content {
		if blk.OfText != nil {
			out = append(out, map[string]interface{}{
				"type": "text",
				"text": blk.OfText.Text,
			})
		} else if blk.OfToolUse != nil {
			inputJSON, _ := json.Marshal(blk.OfToolUse.Input)
			out = append(out, map[string]interface{}{
				"type":  "tool_use",
				"id":    blk.OfToolUse.ID,
				"name":  blk.OfToolUse.Name,
				"input": json.RawMessage(inputJSON),
			})
		}
	}
	return out
}

func betaTextContent(resp anthropicvm.VModelResponse) string {
	s := ""
	for _, blk := range resp.Content {
		if blk.OfText != nil {
			s += blk.OfText.Text
		}
	}
	return s
}
