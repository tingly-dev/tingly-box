// Package sdkstream presents a Protocol Stage EventStream as the SDK stream
// types the existing client writers consume, so the client-facing output of
// a Stage pipeline goes through exactly the code that writes it today.
//
// Anthropic V1 is served here from Beta events: V1 is a subset of Beta on the
// wire, so a Beta event is re-read as its V1 form. Content V1 cannot express
// is refused explicitly (see AnthropicV1Downgrade) rather than dropped.
package sdkstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

// Option configures a stream presentation.
type Option func(*anthropicDecoder)

// OnHeartbeat calls f for every stage.Heartbeat, on the goroutine reading the
// stream (the one writing to the client), so f may write to the client.
// Without it heartbeats are dropped.
func OnHeartbeat(f func()) Option {
	return func(d *anthropicDecoder) { d.heartbeat = f }
}

// AnthropicBeta returns events as an Anthropic Beta SDK stream. Closing the
// SDK stream closes events.
func AnthropicBeta(ctx context.Context, events stage.EventStream, options ...Option) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	return anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](newAnthropicDecoder(ctx, events, false, options), nil)
}

// AnthropicV1 returns Beta events as an Anthropic V1 SDK stream for a V1
// client. A block V1 cannot express ends the stream with an error.
func AnthropicV1(ctx context.Context, events stage.EventStream, options ...Option) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	return anthropicstream.NewStream[anthropic.MessageStreamEventUnion](newAnthropicDecoder(ctx, events, true, options), nil)
}

func newAnthropicDecoder(ctx context.Context, events stage.EventStream, v1 bool, options []Option) *anthropicDecoder {
	d := &anthropicDecoder{ctx: ctx, events: events, v1: v1}
	for _, option := range options {
		option(d)
	}
	return d
}

// anthropicDecoder feeds wire payloads of stage events to the SDK stream,
// which decodes them exactly as it decodes a provider's SSE body.
type anthropicDecoder struct {
	ctx       context.Context
	events    stage.EventStream
	v1        bool
	heartbeat func()
	event     anthropicstream.Event
	err       error
}

func (d *anthropicDecoder) Next() bool {
	if d.err != nil {
		return false
	}
	event, err := d.events.Next(d.ctx)
	for err == nil {
		if _, ok := event.Value.(stage.Heartbeat); !ok {
			break
		}
		if d.heartbeat != nil {
			d.heartbeat()
		}
		event, err = d.events.Next(d.ctx)
	}
	if errors.Is(err, io.EOF) {
		return false
	}
	if err != nil {
		d.err = err
		return false
	}
	data, err := WireJSON(event.Value)
	if err != nil {
		d.err = err
		return false
	}
	if gjson.GetBytes(data, "type").String() == "error" {
		// An in-band error a bridge's converter emitted (e.g. a truncated
		// upstream). Surface it as the stream's error with its own message,
		// so the writer reports it once rather than wrapping its JSON.
		d.err = InBandError{
			Type:    gjson.GetBytes(data, "error.type").String(),
			Message: gjson.GetBytes(data, "error.message").String(),
		}
		return false
	}
	if d.v1 {
		if err := checkV1Event(data); err != nil {
			d.err = err
			return false
		}
	}
	d.event = anthropicstream.Event{Type: gjson.GetBytes(data, "type").String(), Data: data}
	return true
}

func (d *anthropicDecoder) Event() anthropicstream.Event { return d.event }

func (d *anthropicDecoder) Close() error { return d.events.Close() }

func (d *anthropicDecoder) Err() error { return d.err }

// InBandError is an error event carried inside a stream.
type InBandError struct {
	Type    string
	Message string
}

func (e InBandError) Error() string { return e.Message }

// WireJSON renders a stage event value as its wire payload. Carrier types
// expose the payload through RawJSON, as the SSE writers use it.
func WireJSON(value any) ([]byte, error) {
	if raw, ok := value.(interface{ RawJSON() string }); ok {
		if data := raw.RawJSON(); data != "" {
			return []byte(data), nil
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("sdkstream: marshal %T: %w", value, err)
	}
	return data, nil
}

// v1ContentBlockTypes are the content block types the Anthropic V1 API
// defines (anthropic.ContentBlockUnion).
var v1ContentBlockTypes = map[string]bool{
	"text": true, "thinking": true, "redacted_thinking": true, "tool_use": true,
	"server_tool_use": true, "web_search_tool_result": true, "web_fetch_tool_result": true,
	"code_execution_tool_result": true, "bash_code_execution_tool_result": true,
	"text_editor_code_execution_tool_result": true, "tool_search_tool_result": true,
	"container_upload": true,
}

func checkV1Event(data []byte) error {
	if gjson.GetBytes(data, "type").String() != "content_block_start" {
		return nil
	}
	return checkV1Block(gjson.GetBytes(data, "content_block.type").String())
}

func checkV1Block(blockType string) error {
	if !v1ContentBlockTypes[blockType] {
		return fmt.Errorf("Anthropic Beta content block %q has no V1 form", blockType)
	}
	return nil
}

// AnthropicV1Downgrade re-reads a Beta message as its V1 form for a V1
// client, keeping its wire bytes. A block V1 cannot express is an error.
func AnthropicV1Downgrade(message *anthropic.BetaMessage) (*anthropic.Message, error) {
	raw := []byte(message.RawJSON())
	if len(raw) == 0 {
		var err error
		if raw, err = json.Marshal(message); err != nil {
			return nil, fmt.Errorf("sdkstream: marshal Beta message: %w", err)
		}
	}
	for _, block := range gjson.GetBytes(raw, "content").Array() {
		if err := checkV1Block(block.Get("type").String()); err != nil {
			return nil, err
		}
	}
	var v1 anthropic.Message
	if err := json.Unmarshal(raw, &v1); err != nil {
		return nil, fmt.Errorf("sdkstream: read Beta message as V1: %w", err)
	}
	return &v1, nil
}
