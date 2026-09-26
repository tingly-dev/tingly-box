package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// AnthropicWire is the wire an Anthropic provider is reached over.
type AnthropicWire uint8

const (
	// AnthropicWireBeta sends the Beta request as is (?beta=true).
	AnthropicWireBeta AnthropicWire = iota
	// AnthropicWireV1 downgrades the request to V1 and upgrades the response,
	// for routes that reach the provider over the V1 wire.
	AnthropicWireV1
)

// NewAnthropic returns the Anthropic Beta terminal endpoint.
func NewAnthropic(config Config, wire AnthropicWire) (stage.Endpoint, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if wire != AnthropicWireBeta && wire != AnthropicWireV1 {
		return nil, fmt.Errorf("upstream endpoint: unknown Anthropic wire %d", wire)
	}
	return &anthropicEndpoint{config: config, wire: wire}, nil
}

type anthropicEndpoint struct {
	config Config
	wire   AnthropicWire
}

func (*anthropicEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

func (e *anthropicEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	req, err := anthropicRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.config.Clients.GetAnthropicClient(ctx, e.config.Provider, e.config.Model)
	fc := e.config.forwardContext(ctx)

	var message *anthropic.BetaMessage
	var usage *protocol.TokenUsage
	if e.wire == AnthropicWireV1 {
		v1, err := request.ConvertAnthropicBetaToV1Request(req)
		if err != nil {
			return nil, err
		}
		v1Message, cancel, err := forwarding.ForwardAnthropicV1(fc, wrapper, v1)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, err
		}
		usage = protocolusage.FromAnthropicMessage(v1Message.Usage)
		if message, err = upgradeAnthropicMessage(v1Message); err != nil {
			return nil, err
		}
	} else {
		var cancel context.CancelFunc
		message, cancel, err = forwarding.ForwardAnthropicV1Beta(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, err
		}
		usage = protocolusage.FromAnthropicBetaMessage(message.Usage)
	}
	return &stage.Response{
		Value: message,
		Usage: usage,
		Model: e.config.model(string(message.Model)),
	}, nil
}

func (e *anthropicEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	req, err := anthropicRequest(call.Request)
	if err != nil {
		return nil, err
	}
	wrapper := e.config.Clients.GetAnthropicClient(ctx, e.config.Provider, e.config.Model)
	fc := e.config.forwardContext(ctx)
	out := &anthropicStream{
		client: wrapper,
		usage:  protocolusage.NewAnthropicAccumulator(),
		model:  e.config.Model,
	}
	if e.wire == AnthropicWireV1 {
		v1, err := request.ConvertAnthropicBetaToV1Request(req)
		if err != nil {
			return nil, err
		}
		out.v1, out.cancel, err = forwarding.ForwardAnthropicV1Stream(fc, wrapper, v1)
		if err != nil {
			if out.cancel != nil {
				out.cancel()
			}
			return nil, err
		}
		return out, nil
	}
	out.beta, out.cancel, err = forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, req)
	if err != nil {
		if out.cancel != nil {
			out.cancel()
		}
		return nil, err
	}
	return out, nil
}

func anthropicRequest(value any) (*anthropic.BetaMessageNewParams, error) {
	switch req := value.(type) {
	case *anthropic.BetaMessageNewParams:
		if req != nil {
			return req, nil
		}
	case anthropic.BetaMessageNewParams:
		return &req, nil
	}
	return nil, fmt.Errorf("Anthropic upstream endpoint: request has type %T, want anthropic.BetaMessageNewParams", value)
}

// upgradeAnthropicMessage lifts a V1 response into Beta through its wire JSON,
// the same superset projection the client edge uses for requests.
func upgradeAnthropicMessage(message *anthropic.Message) (*anthropic.BetaMessage, error) {
	var beta anthropic.BetaMessage
	if err := json.Unmarshal(anthropicRaw(message.RawJSON(), message), &beta); err != nil {
		return nil, fmt.Errorf("upgrade Anthropic v1 response to Beta: %w", err)
	}
	return &beta, nil
}

func anthropicRaw(raw string, value any) []byte {
	if raw != "" {
		return []byte(raw)
	}
	data, _ := json.Marshal(value)
	return data
}

// anthropicStream reads either wire and always yields Beta events. It holds
// the client wrapper so the pool's finalizer cannot close it mid-stream.
type anthropicStream struct {
	client client.AnthropicClientInterface
	beta   *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]
	v1     *anthropicstream.Stream[anthropic.MessageStreamEventUnion]
	cancel context.CancelFunc
	usage  *protocolusage.AnthropicAccumulator
	model  string

	closeOnce sync.Once
	closeErr  error
}

func (s *anthropicStream) Next(ctx context.Context) (stage.Event, error) {
	defer runtime.KeepAlive(s.client)
	if err := ctx.Err(); err != nil {
		return stage.Event{}, err
	}
	if s.v1 != nil {
		if !s.v1.Next() {
			return stage.Event{}, streamEnd(s.v1.Err())
		}
		v1 := s.v1.Current()
		// Accumulate from the V1 event: its accumulator carries the raw-JSON
		// fallbacks vendors on the V1 wire rely on.
		s.usage.Consume(&v1)
		var event anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(anthropicRaw(v1.RawJSON(), v1), &event); err != nil {
			return stage.Event{}, fmt.Errorf("upgrade Anthropic v1 stream event to Beta: %w", err)
		}
		s.observeModel(event)
		return stage.Event{Value: event}, nil
	}
	if !s.beta.Next() {
		return stage.Event{}, streamEnd(s.beta.Err())
	}
	event := s.beta.Current()
	s.usage.ConsumeBeta(&event)
	s.observeModel(event)
	return stage.Event{Value: event}, nil
}

func (s *anthropicStream) observeModel(event anthropic.BetaRawMessageStreamEventUnion) {
	if event.Type == "message_start" && event.Message.Model != "" {
		s.model = string(event.Message.Model)
	}
}

func (s *anthropicStream) Close() error {
	s.closeOnce.Do(func() {
		switch {
		case s.v1 != nil:
			s.closeErr = s.v1.Close()
		case s.beta != nil:
			s.closeErr = s.beta.Close()
		}
		if s.cancel != nil {
			s.cancel()
		}
	})
	return s.closeErr
}

func (s *anthropicStream) Result() stage.StreamResult {
	result := stage.StreamResult{Model: s.model}
	if s.usage.HasUsage() {
		result.Usage = s.usage.Result()
	}
	return result
}

// streamEnd maps an SDK stream's terminal error to the EventStream contract.
func streamEnd(err error) error {
	if err == nil || err == io.EOF {
		return io.EOF
	}
	return err
}
