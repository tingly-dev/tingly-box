package claude

import (
	"context"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/agentboot"
)

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
func (r *ResultCollector) OnMessage(msg any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if m, ok := msg.(Message); ok {
		r.messages = append(r.messages, m)
	}

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
func (r *ResultCollector) OnComplete(completion *agentboot.CompletionResult) {
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

func (f *ResultCollector) OnApproval(context.Context, agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	return agentboot.PermissionResult{Approved: true}, nil
}

func (f *ResultCollector) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	return agentboot.AskResult{ID: req.ID, Approved: true}, nil
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
