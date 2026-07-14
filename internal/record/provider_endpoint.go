package record

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

// ObserveProvider wraps the terminal provider Endpoint when recording is
// enabled. A nil Recorder returns next unchanged, keeping the default path free
// of wrapper allocation and stream assembly.
func ObserveProvider(next stage.Endpoint, recorder *Recorder, meta ExchangeMetadata) stage.Endpoint {
	if recorder == nil {
		return next
	}
	meta.Protocol = next.Protocol()
	return &providerEndpoint{
		next:     next,
		recorder: recorder,
		meta:     meta,
	}
}

type providerEndpoint struct {
	next     stage.Endpoint
	recorder *Recorder
	meta     ExchangeMetadata
}

func (e *providerEndpoint) Protocol() protocol.APIType {
	return e.next.Protocol()
}

func (e *providerEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	exchange, _ := e.recorder.BeginExchange(e.meta, call.Request)
	response, callErr := e.next.Complete(ctx, call)
	if exchange != nil {
		var value any
		if response != nil {
			value = response.Value
		}
		finishExchange(exchange, value, callErr)
	}
	return response, callErr
}

func (e *providerEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	exchange, _ := e.recorder.BeginExchange(e.meta, call.Request)
	stream, callErr := e.next.Stream(ctx, call)
	if callErr != nil {
		finishExchange(exchange, nil, callErr)
		return nil, callErr
	}
	if exchange == nil || stream == nil {
		return stream, nil
	}

	streamAssembler, _ := assembler.NewStreamAssembler(e.meta.Protocol)
	return &providerStream{
		next:      stream,
		exchange:  exchange,
		assembler: streamAssembler,
	}, nil
}

type providerStream struct {
	next      stage.EventStream
	exchange  *Exchange
	assembler assembler.StreamAssembler

	finishOnce sync.Once
}

func (s *providerStream) Next(ctx context.Context) (stage.Event, error) {
	event, err := s.next.Next(ctx)
	if err == nil {
		if s.assembler != nil {
			if assembleErr := s.assembler.Add(event.Value); assembleErr != nil {
				// Recording is observational. Stop assembling this response without
				// changing the provider stream seen by the caller.
				s.assembler = nil
			}
		}
		return event, nil
	}

	if errors.Is(err, io.EOF) {
		s.finish(nil)
	} else {
		s.finish(err)
	}
	return event, err
}

func (s *providerStream) Close() error {
	closeErr := s.next.Close()
	if closeErr != nil {
		s.finish(closeErr)
	} else {
		// A successful outer Stage/Bridge may stop after the provider's terminal
		// event without pulling one additional EOF from this inner stream. The
		// request driver still closes the chain normally, so preserve the
		// assembled provider response as a successful exchange. Cancellation and
		// other early termination already reach Next and win finishOnce first.
		s.finish(nil)
	}
	return closeErr
}

func (s *providerStream) Result() stage.StreamResult {
	return s.next.Result()
}

func (s *providerStream) finish(streamErr error) {
	s.finishOnce.Do(func() {
		var response any
		if streamErr == nil && s.assembler != nil {
			response, _ = s.assembler.Finish()
		}
		finishExchange(s.exchange, response, streamErr)
	})
}

func finishExchange(exchange *Exchange, response any, callErr error) {
	if exchange == nil {
		return
	}
	if err := exchange.Finish(response, callErr); err != nil && response != nil {
		// A capture/serialization failure must not leave the exchange pending or
		// affect request execution. Preserve its outcome without the response.
		_ = exchange.Finish(nil, callErr)
	}
}
