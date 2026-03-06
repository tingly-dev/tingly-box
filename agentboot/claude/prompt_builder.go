package claude

import (
	"fmt"
	"sync"
	"time"
)

// StreamPromptBuilder helps build stream prompts
type StreamPromptBuilder struct {
	messages chan any
	closed   bool
	mu       sync.Mutex
}

// NewStreamPromptBuilder creates a new stream prompt builder
func NewStreamPromptBuilder() *StreamPromptBuilder {
	return &StreamPromptBuilder{
		messages: make(chan any, 100),
		closed:   false,
	}
}

// Add adds a message to the stream
func (b *StreamPromptBuilder) Add(msg map[string]interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("stream builder is closed")
	}

	select {
	case b.messages <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout adding message to stream")
	}
}

// AddText adds a text message to the stream
func (b *StreamPromptBuilder) AddText(text string) error {
	msg := map[string]interface{}{
		"type":    "text",
		"content": text,
	}
	return b.Add(msg)
}

// AddUserMessage adds a user message to the stream
func (b *StreamPromptBuilder) AddUserMessage(content string) error {
	msg := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": content,
		},
	}
	return b.Add(msg)
}

// Close closes the stream and returns the channel for use with Query
func (b *StreamPromptBuilder) Close() StreamPrompt {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.closed {
		b.closed = true
		close(b.messages)
	}

	return b.messages
}

// Messages returns the underlying message channel
func (b *StreamPromptBuilder) Messages() StreamPrompt {
	return b.messages
}

// IsClosed returns true if the builder is closed
func (b *StreamPromptBuilder) IsClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}
