package stream

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// StreamConverter reads from an upstream stream and emits target-protocol
// events one at a time. It maintains internal conversion state (tool call
// accumulation, usage tracking, etc.) but never touches gin.Context or
// writes SSE directly.
//
// Next() returns (event, false, nil) for each target event, (nil, true, nil)
// when the upstream is exhausted, or (nil, false, err) on error.
type StreamConverter interface {
	Next() (event interface{}, done bool, err error)
	Usage() *protocol.TokenUsage
}

// RunConverter bridges a StreamConverter into ProcessStream. It sets up SSE
// headers, drives the converter via ProcessStream (which dispatches hooks
// automatically), and calls the writer for each emitted event.
func RunConverter(hc *protocol.HandleContext, conv StreamConverter, writer func(event interface{}) error) (*protocol.TokenUsage, error) {
	hc.SetupSSEHeaders()

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			event, done, err := conv.Next()
			if err != nil {
				return false, err, nil
			}
			if done {
				return false, nil, nil
			}
			return true, nil, event
		},
		writer,
	)

	return conv.Usage(), err
}
