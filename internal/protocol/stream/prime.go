// Package stream — prime.go
//
// Eager first-event start for OpenAI Responses streams. The OpenAI Go
// SDK returns a *Stream from NewStreaming without issuing the HTTP
// request — the real upstream verdict surfaces only on the first
// Next(). PrimeResponsesStream forces that first Next() so a pre-stream
// upstream error can be returned to the dispatch caller (which retries
// the next priority tier) before any byte hits the wire. The first
// event it reads is not discarded: it is replayed via
// firstEventReplayStream so the handler sees a complete stream.
//
// NOTE: this is unrelated to the probe subsystem, which issues separate
// synthetic health-check requests. This one pulls the first event of the
// real business stream and replays it — nothing extra is sent.
package stream

import (
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

// ResponsesStreamIter is the iterator surface stream handlers consume.
// Both the SDK's *openaistream.Stream[responses.ResponseStreamEventUnion]
// and the replay wrapper below satisfy it.
type ResponsesStreamIter interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
	Close() error
}

// firstEventReplayStream yields a pre-read first event then delegates to
// the underlying SDK stream. nextCount==1 returns the cached event
// (inner hasn't been advanced, so inner.Current() would return zero);
// nextCount>=2 delegates to inner.
type firstEventReplayStream struct {
	first     responses.ResponseStreamEventUnion
	nextCount int
	inner     *openaistream.Stream[responses.ResponseStreamEventUnion]
}

func (p *firstEventReplayStream) Next() bool {
	p.nextCount++
	if p.nextCount == 1 {
		return true
	}
	return p.inner.Next()
}

func (p *firstEventReplayStream) Current() responses.ResponseStreamEventUnion {
	if p.nextCount == 1 {
		return p.first
	}
	return p.inner.Current()
}

func (p *firstEventReplayStream) Err() error   { return p.inner.Err() }
func (p *firstEventReplayStream) Close() error { return p.inner.Close() }

// PrimeResponsesStream calls Next() once on the SDK stream to force the
// lazy HTTP request and surface pre-stream upstream errors. Returns:
//
//   - (iter, nil)   primed; iter replays the read event first, then
//                   delegates to the SDK stream.
//   - (iter, nil)   degenerate "no events, no error" — iter is the SDK
//                   stream itself (which immediately reports Next()=false).
//   - (nil, err)    pre-stream failure — caller retries next priority tier.
func PrimeResponsesStream(stream *openaistream.Stream[responses.ResponseStreamEventUnion]) (ResponsesStreamIter, error) {
	if stream == nil {
		return nil, nil
	}
	if !stream.Next() {
		if err := stream.Err(); err != nil {
			return nil, err
		}
		return stream, nil
	}
	return &firstEventReplayStream{
		first: stream.Current(),
		inner: stream,
	}, nil
}
