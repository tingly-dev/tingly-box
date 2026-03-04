package mitm

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
)

// StdinSource reads JSON messages from os.Stdin
type StdinSource struct {
	reader *bufio.Reader
}

func NewStdinSource() *StdinSource {
	return &StdinSource{
		reader: bufio.NewReader(os.Stdin),
	}
}

func (s *StdinSource) Read(ctx context.Context) (any, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}

		var msg any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		return msg, nil
	}
}

// ChannelSource reads messages from a channel
type ChannelSource struct {
	ch <-chan any
}

func NewChannelSource(ch <-chan any) *ChannelSource {
	return &ChannelSource{ch: ch}
}

func (s *ChannelSource) Read(ctx context.Context) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-s.ch:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	}
}

// ChanSource is a bidirectional channel source that supports both reading and writing.
// It implements InputSource and provides a Write method for injecting messages.
type ChanSource struct {
	ch chan any
}

// NewChanSource creates a new ChanSource with the given buffer size.
func NewChanSource(buffer int) *ChanSource {
	return &ChanSource{ch: make(chan any, buffer)}
}

// Read implements InputSource. It blocks until a message is available or context is done.
func (s *ChanSource) Read(ctx context.Context) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-s.ch:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	}
}

// Write injects a message into the source. It's non-blocking - if the channel is full,
// the message is dropped.
func (s *ChanSource) Write(msg any) {
	select {
	case s.ch <- msg:
	default:
		// Channel full, drop message
	}
}

// WriteWait injects a message into the source, waiting until context is done if full.
func (s *ChanSource) WriteWait(ctx context.Context, msg any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.ch <- msg:
		return nil
	}
}

// C returns the underlying channel for external use.
func (s *ChanSource) C() chan<- any {
	return s.ch
}

// Close closes the underlying channel.
func (s *ChanSource) Close() {
	close(s.ch)
}
