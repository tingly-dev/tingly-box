package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrEncoderClosed is returned when Encode is called after Close.
var ErrEncoderClosed = errors.New("protocol: encoder closed")

// Encoder writes control responses (and other outbound messages) as
// newline-delimited JSON to a destination io.Writer. It is safe for
// concurrent use; Encode and Close are serialized so that no JSON value is
// truncated by a concurrent stdin shutdown.
type Encoder struct {
	mu     sync.Mutex
	dst    io.Writer
	closed bool
}

// NewEncoder constructs an Encoder targeting w.
func NewEncoder(w io.Writer) *Encoder { return &Encoder{dst: w} }

// Encode marshals v to JSON and writes it to the destination, terminated
// by a newline.
func (e *Encoder) Encode(v any) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrEncoderClosed
	}
	enc := json.NewEncoder(e.dst)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// Close closes the destination when it implements io.Closer. It is
// idempotent and waits for an in-flight Encode to finish first.
func (e *Encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if closer, ok := e.dst.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
