// Package protocol is the pure stream-protocol layer of agentboot. It has no
// knowledge of how the agent process is started or how its events are routed
// — it only understands bytes flowing in and out of an io.Reader/io.Writer.
//
// This isolation makes the decoder testable against fixed JSON inputs without
// any process or goroutine plumbing.
package protocol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// Decoder reads a stream of JSON-encoded values from an io.Reader and emits
// them as Events on the channel returned by Stream.
type Decoder struct {
	src io.Reader
}

// NewDecoder constructs a Decoder reading from r.
func NewDecoder(r io.Reader) *Decoder { return &Decoder{src: r} }

// Stream consumes the decoder's input on a goroutine and returns the event
// channel together with an Err accessor.
//
// Channel-close semantics:
//   - The channel is closed exactly once, when the source reaches EOF, ctx
//     is canceled, or a non-recoverable decode error occurs.
//   - Err must only be called after the channel has been observed closed.
//     The Go memory model guarantees the terminal error is visible to
//     readers after channel-close synchronization.
//   - If the source implements io.Closer, it is closed on ctx cancel so
//     that an in-flight blocking Decode unblocks promptly. Callers that
//     pass a non-closer reader must close it themselves to abort decoding.
//
// If decoding succeeds and the source ends cleanly with EOF, Err returns nil.
func (d *Decoder) Stream(ctx context.Context) (events <-chan common.Event, errFn func() error) {
	out := make(chan common.Event)
	var termErr error

	go func() {
		defer close(out)

		if closer, ok := d.src.(io.Closer); ok {
			watcherDone := make(chan struct{})
			defer close(watcherDone)
			go func() {
				select {
				case <-ctx.Done():
					_ = closer.Close()
				case <-watcherDone:
				}
			}()
		}

		dec := json.NewDecoder(bufio.NewReader(d.src))
		for {
			var raw map[string]any
			if err := dec.Decode(&raw); err != nil {
				if ctx.Err() != nil {
					termErr = ctx.Err()
					return
				}
				if errors.Is(err, io.EOF) {
					return
				}
				termErr = fmt.Errorf("decode: %w", err)
				return
			}

			ev := common.NewEventFromMap(raw)
			select {
			case out <- ev:
			case <-ctx.Done():
				termErr = ctx.Err()
				return
			}
		}
	}()

	return out, func() error { return termErr }
}
