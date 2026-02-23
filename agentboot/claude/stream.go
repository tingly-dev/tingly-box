package claude

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/events"
)

// MessageHandler is the interface for real-time message processing
type MessageHandler interface {
	// OnMessage is called when a complete message is available
	OnMessage(msg Message) error

	// OnError is called when an error occurs
	OnError(err error)

	// OnComplete is called when execution completes
	OnComplete(result *ResultCompletion)
}

// ResultCompletion contains the final result information
type ResultCompletion struct {
	Success    bool
	DurationMS int64
	Usage      *UsageInfo
	SessionID  string
	Error      string
}

// MessageHandlerFunc is a function adapter for MessageHandler
type MessageHandlerFunc func(msg Message) error

// OnMessage implements MessageHandler
func (f MessageHandlerFunc) OnMessage(msg Message) error {
	return f(msg)
}

// OnError implements MessageHandler
func (f MessageHandlerFunc) OnError(err error) {
	logrus.Errorf("MessageHandler error: %v", err)
}

// OnComplete implements MessageHandler
func (f MessageHandlerFunc) OnComplete(result *ResultCompletion) {
	logrus.Infof("Execution complete: success=%v", result.Success)
}

// Listener is a simpler interface for receiving messages without error handling
type Listener interface {
	OnMessage(msg Message)
}

// ListenerFunc is a function adapter for Listener
type ListenerFunc func(msg Message)

// OnMessage implements Listener
func (f ListenerFunc) OnMessage(msg Message) {
	f(msg)
}

// StreamHandler provides real-time message delivery via channels
type StreamHandler struct {
	mu             sync.RWMutex
	messageChan    chan Message
	errorChan      chan error
	done           chan struct{}
	accumulator    *MessageAccumulator
	result         *ResultCompletion
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	closed         bool
	completionOnce sync.Once
}

// NewStreamHandler creates a new stream handler with specified buffer size
func NewStreamHandler(bufferSize int) *StreamHandler {
	ctx, cancel := context.WithCancel(context.Background())

	return &StreamHandler{
		messageChan: make(chan Message, bufferSize),
		errorChan:   make(chan error, 10),
		done:        make(chan struct{}),
		accumulator: NewMessageAccumulator(),
		ctx:         ctx,
		cancel:      cancel,
		closed:      false,
	}
}

// NewStreamHandlerWithListener creates a stream handler that forwards to a listener
func NewStreamHandlerWithListener(bufferSize int, listener Listener) *StreamHandler {
	handler := NewStreamHandler(bufferSize)

	handler.wg.Add(1)
	go func() {
		defer handler.wg.Done()
		for {
			select {
			case <-handler.ctx.Done():
				return
			case msg, ok := <-handler.messageChan:
				if !ok {
					return
				}
				listener.OnMessage(msg)
			}
		}
	}()

	return handler
}

// Messages returns the read-only message channel
func (s *StreamHandler) Messages() <-chan Message {
	return s.messageChan
}

// Errors returns the read-only error channel
func (s *StreamHandler) Errors() <-chan error {
	return s.errorChan
}

// HandleEvent processes an event and may emit messages
func (s *StreamHandler) HandleEvent(event events.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("stream handler is closed")
	}

	newMessages, hasResult, resultSuccess := s.accumulator.AddEvent(event)

	// Send new messages to channel
	for _, msg := range newMessages {
		select {
		case s.messageChan <- msg:
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-s.done:
			return errors.New("stream handler closed")
		}
	}

	// Handle result message
	if hasResult {
		s.completionOnce.Do(func() {
			s.result = &ResultCompletion{
				Success:   resultSuccess,
				SessionID: s.accumulator.GetSessionID(),
			}
		})
	}

	return nil
}

// GetMessages returns all accumulated messages
func (s *StreamHandler) GetMessages() []Message {
	return s.accumulator.GetMessages()
}

// GetSessionID returns the session ID
func (s *StreamHandler) GetSessionID() string {
	return s.accumulator.GetSessionID()
}

// GetResult returns the completion result if available
func (s *StreamHandler) GetResult() *ResultCompletion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.result
}

// Close closes the stream handler
func (s *StreamHandler) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.cancel()
	close(s.done)

	// Wait for listener goroutines to finish
	s.wg.Wait()

	// Close channels
	close(s.messageChan)
	close(s.errorChan)

	return nil
}

// IsClosed returns true if the handler is closed
func (s *StreamHandler) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// Flush drains any remaining messages from the channel
func (s *StreamHandler) Flush() {
	for {
		select {
		case <-s.messageChan:
		default:
			return
		}
	}
}

// StreamingExecutor handles streaming execution with handlers
type StreamingExecutor struct {
	handler MessageHandler
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	once    sync.Once
}

// NewStreamingExecutor creates a new streaming executor
func NewStreamingExecutor(ctx context.Context, handler MessageHandler) *StreamingExecutor {
	execCtx, cancel := context.WithCancel(ctx)

	return &StreamingExecutor{
		handler: handler,
		ctx:     execCtx,
		cancel:  cancel,
	}
}

// HandleEvent implements the event handling for streaming execution
func (e *StreamingExecutor) HandleEvent(event events.Event) error {
	select {
	case <-e.ctx.Done():
		return e.ctx.Err()
	default:
	}

	// Parse event into message
	accumulator := NewMessageAccumulator()
	messages, hasResult, resultSuccess := accumulator.AddEvent(event)

	// Forward messages to handler
	for _, msg := range messages {
		if err := e.handler.OnMessage(msg); err != nil {
			e.handler.OnError(err)
			return err
		}
	}

	// Handle completion
	if hasResult {
		e.once.Do(func() {
			e.handler.OnComplete(&ResultCompletion{
				Success:   resultSuccess,
				SessionID: accumulator.GetSessionID(),
			})
		})
	}

	return nil
}

// Close closes the streaming executor
func (e *StreamingExecutor) Close() error {
	e.cancel()
	e.wg.Wait()
	return nil
}

// MultiHandler aggregates multiple message handlers
type MultiHandler struct {
	handlers []MessageHandler
	mu       sync.RWMutex
}

// NewMultiHandler creates a handler that forwards to multiple handlers
func NewMultiHandler(handlers ...MessageHandler) *MultiHandler {
	return &MultiHandler{
		handlers: handlers,
	}
}

// AddHandler adds a new handler to the multi-handler
func (m *MultiHandler) AddHandler(handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

// OnMessage implements MessageHandler
func (m *MultiHandler) OnMessage(msg Message) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var firstErr error
	for _, h := range m.handlers {
		if err := h.OnMessage(msg); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// OnError implements MessageHandler
func (m *MultiHandler) OnError(err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, h := range m.handlers {
		h.OnError(err)
	}
}

// OnComplete implements MessageHandler
func (m *MultiHandler) OnComplete(result *ResultCompletion) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, h := range m.handlers {
		h.OnComplete(result)
	}
}

// CallbackHandler converts a channel-based handler to a callback-based handler
type CallbackHandler struct {
	onMessage  func(Message) error
	onError    func(error)
	onComplete func(*ResultCompletion)
}

// NewCallbackHandler creates a handler from callback functions
func NewCallbackHandler(
	onMessage func(Message) error,
	onError func(error),
	onComplete func(*ResultCompletion),
) *CallbackHandler {
	return &CallbackHandler{
		onMessage:  onMessage,
		onError:    onError,
		onComplete: onComplete,
	}
}

// OnMessage implements MessageHandler
func (h *CallbackHandler) OnMessage(msg Message) error {
	if h.onMessage != nil {
		return h.onMessage(msg)
	}
	return nil
}

// OnError implements MessageHandler
func (h *CallbackHandler) OnError(err error) {
	if h.onError != nil {
		h.onError(err)
	}
}

// OnComplete implements MessageHandler
func (h *CallbackHandler) OnComplete(result *ResultCompletion) {
	if h.onComplete != nil {
		h.onComplete(result)
	}
}

// ResultCollector collects messages and builds an agentboot.Result
// It implements MessageHandler for use with ExecuteWithHandler
type ResultCollector struct {
	mu          sync.Mutex
	messages    []Message
	accumulator *MessageAccumulator
	result      *agentboot.Result
	complete    bool
}

// NewResultCollector creates a new result collector
func NewResultCollector() *ResultCollector {
	return &ResultCollector{
		messages:    make([]Message, 0),
		accumulator: NewMessageAccumulator(),
		result: &agentboot.Result{
			Metadata: make(map[string]interface{}),
		},
	}
}

// OnMessage implements MessageHandler
func (r *ResultCollector) OnMessage(msg Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.messages = append(r.messages, msg)

	// Update result based on message type
	switch m := msg.(type) {
	case *AssistantMessage:
		// Extract text from assistant messages
		for _, block := range m.Message.Content {
			r.result.Output += block.Text
		}
		r.result.Format = agentboot.OutputFormatStreamJSON
	case *ResultMessage:
		// Extract final result
		if m.Result != "" {
			r.result.Output = m.Result
		}
		r.result.ExitCode = 0
		if !m.IsSuccess() {
			r.result.ExitCode = 1
			r.result.Error = m.Result
		}
		if m.DurationMS > 0 {
			r.result.Duration = 0 // Will be set by launcher
			r.result.Metadata["duration_ms"] = m.DurationMS
		}
		if m.SessionID != "" {
			r.result.Metadata["session_id"] = m.SessionID
		}
		r.complete = true
	}

	return nil
}

// OnError implements MessageHandler
func (r *ResultCollector) OnError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.result.Error = err.Error()
	r.result.ExitCode = 1
}

// OnComplete implements MessageHandler
func (r *ResultCollector) OnComplete(completion *ResultCompletion) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if completion != nil {
		if !completion.Success {
			r.result.ExitCode = 1
		}
		if completion.Error != "" {
			r.result.Error = completion.Error
		}
		if completion.SessionID != "" {
			r.result.Metadata["session_id"] = completion.SessionID
		}
	}
	r.complete = true
}

// Result returns the collected result
func (r *ResultCollector) Result() *agentboot.Result {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Build events from messages
	events := make([]agentboot.Event, len(r.messages))
	for i, msg := range r.messages {
		events[i] = agentboot.Event{
			Type:      msg.GetType(),
			Data:      msg.GetRawData(),
			Timestamp: msg.GetTimestamp(),
		}
	}
	r.result.Events = events

	return r.result
}

// GetMessages returns all collected messages
func (r *ResultCollector) GetMessages() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]Message, len(r.messages))
	copy(result, r.messages)
	return result
}

// IsComplete returns true if collection is complete
func (r *ResultCollector) IsComplete() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.complete
}

// BuildTextOutput constructs the text output from collected messages
func (r *ResultCollector) BuildTextOutput() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var output strings.Builder

	for _, msg := range r.messages {
		switch m := msg.(type) {
		case *AssistantMessage:
			for _, block := range m.Message.Content {
				output.WriteString(block.Text)
			}
		case *ResultMessage:
			if m.Result != "" {
				output.WriteString(m.Result)
			}
		}
	}

	return output.String()
}
