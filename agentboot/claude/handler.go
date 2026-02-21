package claude

import (
	"strings"
	"sync"
)

// DefaultMessageHandler implements MessageHandler with formatted text output
type DefaultMessageHandler struct {
	formatter     Formatter
	outputBuilder *strings.Builder
	mu            sync.Mutex
	completion    *ResultCompletion
}

// NewDefaultMessageHandler creates a new default message handler
func NewDefaultMessageHandler(formatter Formatter) *DefaultMessageHandler {
	return &DefaultMessageHandler{
		formatter:     formatter,
		outputBuilder: &strings.Builder{},
	}
}

// NewDefaultTextHandler creates a handler with default text formatter
func NewDefaultTextHandler() *DefaultMessageHandler {
	return NewDefaultMessageHandler(NewTextFormatter())
}

// OnMessage implements MessageHandler
func (h *DefaultMessageHandler) OnMessage(msg Message) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	formatted := h.formatter.Format(msg)
	h.outputBuilder.WriteString(formatted)

	// Ensure newline separation
	if !strings.HasSuffix(formatted, "\n") {
		h.outputBuilder.WriteString("\n")
	}

	return nil
}

// OnError implements MessageHandler
func (h *DefaultMessageHandler) OnError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.outputBuilder.WriteString("[ERROR] ")
	h.outputBuilder.WriteString(err.Error())
	h.outputBuilder.WriteString("\n")
}

// OnComplete implements MessageHandler
func (h *DefaultMessageHandler) OnComplete(result *ResultCompletion) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.completion = result

	if result != nil {
		h.outputBuilder.WriteString("\n[COMPLETE] ")
		if result.Success {
			h.outputBuilder.WriteString("SUCCESS")
		} else {
			h.outputBuilder.WriteString("FAILED")
		}
		if result.Error != "" {
			h.outputBuilder.WriteString(": ")
			h.outputBuilder.WriteString(result.Error)
		}
		h.outputBuilder.WriteString("\n")
	}
}

// GetOutput returns the accumulated formatted output
func (h *DefaultMessageHandler) GetOutput() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.outputBuilder.String()
}

// GetCompletion returns the completion result if available
func (h *DefaultMessageHandler) GetCompletion() *ResultCompletion {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.completion
}

// Reset clears the handler state
func (h *DefaultMessageHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.outputBuilder.Reset()
	h.completion = nil
}

// SetFormatter sets a new formatter
func (h *DefaultMessageHandler) SetFormatter(formatter Formatter) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.formatter = formatter
}

// GetFormatter returns the current formatter
func (h *DefaultMessageHandler) GetFormatter() Formatter {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.formatter
}

// String returns a string representation of the output
func (h *DefaultMessageHandler) String() string {
	return h.GetOutput()
}
