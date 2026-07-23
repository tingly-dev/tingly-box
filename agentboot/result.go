package agentboot

import (
	"strings"
	"time"
)

// Result represents the result of an agent execution.
type Result struct {
	Output   string // Agent output (text mode)
	ExitCode int    // Process exit code
	Error    string // Error message if failed
	Duration time.Duration
	Format   OutputFormat           // Output format used
	Events   []Event                // Stream events (stream-json mode)
	Metadata map[string]interface{} // Additional metadata
}

// TextOutput returns the full text output from the result.
func (r *Result) TextOutput() string {
	if r == nil {
		return ""
	}

	switch r.Format {
	case OutputFormatStreamJSON:
		var output strings.Builder
		for _, event := range r.Events {
			// Handle SDK stream types
			if event.Type == "assistant" {
				// Real CLI shape: message is an object whose content is an
				// array of blocks; concatenate the text blocks.
				if msg, ok := event.Data["message"].(map[string]any); ok {
					if content, ok := msg["content"].([]any); ok {
						for _, block := range content {
							bm, ok := block.(map[string]any)
							if !ok || bm["type"] != "text" {
								continue
							}
							if txt, ok := bm["text"].(string); ok {
								output.WriteString(txt)
							}
						}
					}
				} else if message, ok := event.Data["message"].(string); ok {
					// Legacy/simple shape: message is already a string.
					output.WriteString(message)
				}
			} else if event.Type == "text_delta" {
				// Legacy: text_delta events
				if delta, ok := event.Data["delta"].(string); ok {
					output.WriteString(delta)
				}
			} else if event.Type == "text" {
				// Legacy: text events
				if text, ok := event.Data["text"].(string); ok {
					output.WriteString(text)
				}
			}
		}
		return output.String()
	case OutputFormatText:
		return r.Output
	default:
		return r.Output
	}
}

// IsSuccess returns true if the execution was successful.
func (r *Result) IsSuccess() bool {
	return r != nil && r.ExitCode == 0 && r.Error == ""
}

// GetMessagesByType returns all events of a specific type.
func (r *Result) GetMessagesByType(messageType string) []Event {
	if r == nil {
		return nil
	}

	var result []Event
	for _, event := range r.Events {
		if event.Type == messageType {
			result = append(result, event)
		}
	}
	return result
}

// GetMessageChain returns all events in order, excluding result/system events.
func (r *Result) GetMessageChain() []Event {
	if r == nil {
		return nil
	}

	var result []Event
	for _, event := range r.Events {
		// Skip system and result types for message chain
		if event.Type != "system" && event.Type != "result" && !strings.HasPrefix(event.Type, "control_") {
			result = append(result, event)
		}
	}
	return result
}

// GetAssistantMessages returns all assistant message events.
func (r *Result) GetAssistantMessages() []Event {
	return r.GetMessagesByType("assistant")
}

// GetUserMessages returns all user message events.
func (r *Result) GetUserMessages() []Event {
	return r.GetMessagesByType("user")
}

// GetSessionID extracts the session ID from metadata or events.
func (r *Result) GetSessionID() string {
	if r == nil {
		return ""
	}

	// Check metadata first
	if sessionID, ok := r.Metadata["session_id"].(string); ok {
		return sessionID
	}

	// Look in events for session_id
	for _, event := range r.Events {
		if sessionID, ok := event.Data["session_id"].(string); ok && sessionID != "" {
			return sessionID
		}
	}

	return ""
}

// GetCostUSD extracts the total cost from result events if available.
func (r *Result) GetCostUSD() float64 {
	if r == nil {
		return 0
	}

	for _, event := range r.Events {
		if event.Type == "result" {
			if cost, ok := event.Data["total_cost_usd"].(float64); ok {
				return cost
			}
		}
	}

	return 0
}
