package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// readNonEmptyLines reads a file and returns non-empty lines
func readNonEmptyLines(filePath string) ([]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	var nonEmpty []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	return nonEmpty, nil
}

// unmarshalAs re-marshals a map into a typed struct via JSON round-trip
func unmarshalAs[T any](baseEvent map[string]interface{}) T {
	var result T
	data, _ := json.Marshal(baseEvent)
	_ = json.Unmarshal(data, &result)
	return result
}

// parseSessionFile extracts metadata from a .jsonl file
// Strategy: read first line (init/user) and last line (result/error)
func (s *Store) parseSessionFile(filePath string) (*common.SessionMetadata, error) {
	nonEmptyLines, err := readNonEmptyLines(filePath)
	if err != nil {
		return nil, err
	}

	var metadata common.SessionMetadata
	metadata.ProjectPath = s.decodeProjectPath(filePath)

	if len(nonEmptyLines) == 0 {
		return &metadata, nil
	}

	// Parse header to find session info and first user message
	// Skip non-message events (queue-operation, file-history-snapshot, etc.)
	// Limit header scanning to avoid reading too many lines
	maxHeaderLines := 10
	headerScanLimit := min(len(nonEmptyLines), maxHeaderLines)

	for i := 0; i < headerScanLimit; i++ {
		var event map[string]interface{}
		if json.Unmarshal([]byte(nonEmptyLines[i]), &event) != nil {
			continue
		}

		eventType := extractString(event, "type")

		// Extract session ID from any event that has it
		if metadata.SessionID == "" {
			metadata.SessionID = extractString(event, "sessionId")
			if metadata.SessionID == "" {
				metadata.SessionID = extractString(event, "session_id")
			}
		}

		// Extract timestamp for start time
		if metadata.StartTime.IsZero() {
			if ts := extractString(event, "timestamp"); ts != "" {
				if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
					metadata.StartTime = t
				}
			}
		}

		// Found first user message - extract it
		if eventType == "user" && metadata.FirstMessage == "" {
			if msg, ok := event["message"].(map[string]interface{}); ok {
				metadata.FirstMessage = extractMessageContent(msg)
			}
			// Break after finding first user message
			break
		}
	}

	// Parse last events for context and result
	// Scan backwards from the end to find the last user and assistant messages
	// This ensures we get the actual last messages even in long sessions
	foundUser := false
	foundAssistant := false

	for i := len(nonEmptyLines) - 1; i >= 0; i-- {
		// Stop if we've found both messages
		if foundUser && foundAssistant {
			break
		}

		var event map[string]interface{}
		if json.Unmarshal([]byte(nonEmptyLines[i]), &event) != nil {
			continue
		}

		eventType := extractString(event, "type")

		// Extract last user message (only set once we find it going backwards)
		if eventType == "user" && !foundUser {
			if msg, ok := event["message"].(map[string]interface{}); ok {
				metadata.LastUserMessage = extractMessageContent(msg)
				foundUser = true
			}
		}

		// Extract last assistant message (only set once we find it going backwards)
		if eventType == "assistant" && !foundAssistant {
			if msg, ok := event["message"].(map[string]interface{}); ok {
				metadata.LastAssistantMessage = extractMessageContent(msg)
				foundAssistant = true
			}
		}
	}

	// Parse the last line for result/error
	if len(nonEmptyLines) > 0 {
		lastLine := nonEmptyLines[len(nonEmptyLines)-1]
		var lastEvent map[string]interface{}
		if json.Unmarshal([]byte(lastLine), &lastEvent) == nil {
			eventType := extractString(lastEvent, "type")
			if eventType == "result" {
				subtype := extractString(lastEvent, "subtype")
				if subtype == "error" {
					metadata.Status = common.SessionStatusError
					metadata.Error = extractString(lastEvent, "error")
				} else {
					metadata.Status = common.SessionStatusComplete
					metadata.LastResult = extractString(lastEvent, "result")
				}

				// Extract metrics
				metadata.TotalCostUSD = extractFloat(lastEvent, "total_cost_usd")
				metadata.DurationMS = extractInt64(lastEvent, "duration_ms")
				metadata.NumTurns = extractInt(lastEvent, "num_turns")

				// Parse timestamp
				if ts := extractString(lastEvent, "timestamp"); ts != "" {
					if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
						metadata.EndTime = t
					}
				}

				// Extract usage info
				if usage, ok := lastEvent["usage"].(map[string]interface{}); ok {
					metadata.InputTokens = extractInt(usage, "input_tokens")
					metadata.OutputTokens = extractInt(usage, "output_tokens")
					metadata.CacheReadTokens = extractInt(usage, "cache_read_input_tokens")
				}
			}
		}
	}

	return &metadata, nil
}

// parseSessionEvents parses events from file with offset/limit
func (s *Store) parseSessionEvents(filePath string, offset, limit int) ([]common.SessionEvent, error) {
	nonEmptyLines, err := readNonEmptyLines(filePath)
	if err != nil {
		return nil, err
	}

	var events []common.SessionEvent
	skipped := 0

	for _, line := range nonEmptyLines {
		// Apply offset
		if skipped < offset {
			skipped++
			continue
		}

		// Apply limit
		if limit > 0 && len(events) >= limit {
			break
		}

		var event common.SessionEvent

		// First, parse as base event
		var baseEvent map[string]interface{}
		if err := json.Unmarshal([]byte(line), &baseEvent); err != nil {
			continue // Skip invalid lines
		}

		// Extract common fields
		event.Type = extractString(baseEvent, "type")
		event.Subtype = extractString(baseEvent, "subtype")

		if ts := extractString(baseEvent, "timestamp"); ts != "" {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				event.Timestamp = t
			}
		}

		// Parse event-specific data
		event.Data = s.parseEventData(baseEvent)

		events = append(events, event)
	}

	return events, nil
}

// parseSessionEventsFromEnd parses last N events from file
func (s *Store) parseSessionEventsFromEnd(filePath string, n int) ([]common.SessionEvent, error) {
	if n <= 0 {
		return []common.SessionEvent{}, nil
	}

	nonEmptyLines, err := readNonEmptyLines(filePath)
	if err != nil {
		return nil, err
	}

	// Calculate offset to get last n events
	offset := len(nonEmptyLines) - n
	if offset < 0 {
		offset = 0
	}

	return s.parseSessionEvents(filePath, offset, n)
}

// parseEventData parses event-specific data based on event type
func (s *Store) parseEventData(baseEvent map[string]interface{}) common.EventData {
	eventType := extractString(baseEvent, "type")

	switch eventType {
	case "user":
		return s.parseUserEvent(baseEvent)
	case "assistant":
		return s.parseAssistantEvent(baseEvent)
	case "tool_use":
		return s.parseToolUseEvent(baseEvent)
	case "tool_result":
		return s.parseToolResultEvent(baseEvent)
	case "result":
		return s.parseResultEvent(baseEvent)
	case "system":
		return s.parseSystemEvent(baseEvent)
	case "file-history-snapshot":
		return s.parseFileHistorySnapshot(baseEvent)
	default:
		return nil
	}
}

func (s *Store) parseUserEvent(baseEvent map[string]interface{}) common.EventData {
	return unmarshalAs[common.UserMessageEvent](baseEvent)
}

func (s *Store) parseAssistantEvent(baseEvent map[string]interface{}) common.EventData {
	return unmarshalAs[common.AssistantMessageEvent](baseEvent)
}

func (s *Store) parseToolUseEvent(baseEvent map[string]interface{}) common.EventData {
	return unmarshalAs[common.ToolUseEvent](baseEvent)
}

func (s *Store) parseToolResultEvent(baseEvent map[string]interface{}) common.EventData {
	return unmarshalAs[common.ToolResultEvent](baseEvent)
}

func (s *Store) parseResultEvent(baseEvent map[string]interface{}) common.EventData {
	return unmarshalAs[common.ResultEvent](baseEvent)
}

func (s *Store) parseSystemEvent(baseEvent map[string]interface{}) common.EventData {
	return unmarshalAs[common.SystemEvent](baseEvent)
}

func (s *Store) parseFileHistorySnapshot(baseEvent map[string]interface{}) common.EventData {
	return unmarshalAs[common.FileHistorySnapshotEvent](baseEvent)
}

// Helper functions for extracting values from maps
func extractString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func extractInt(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		}
	}
	return 0
}

func extractInt64(m map[string]interface{}, key string) int64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		}
	}
	return 0
}

func extractFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// decodeProjectPath extracts project path from session file path
func (s *Store) decodeProjectPath(sessionFilePath string) string {
	// Extract relative path from projectsDir
	relPath, err := filepath.Rel(s.projectsDir, sessionFilePath)
	if err != nil {
		return ""
	}

	// Get directory (session ID file's parent)
	projectDir := filepath.Dir(relPath)
	if projectDir == "." || projectDir == "" {
		return ""
	}

	// Decode: -root-tingly-polish -> /root/tingly-polish
	encoded := filepath.Base(projectDir)
	return DecodeProjectPath(encoded)
}

// extractMessageContent extracts text content from a message object
// Handles both simple string content and array of content blocks
func extractMessageContent(msg map[string]interface{}) string {
	// Try content as array first (newer format)
	if content, ok := msg["content"].([]interface{}); ok && len(content) > 0 {
		var textParts []string
		for _, block := range content {
			if blockMap, ok := block.(map[string]interface{}); ok {
				blockType := extractString(blockMap, "type")
				// Extract text from text blocks
				if blockType == "text" {
					text := extractString(blockMap, "text")
					if text != "" {
						textParts = append(textParts, text)
					}
				}
				// For tool_result blocks, extract the content field
				if blockType == "tool_result" {
					if content := extractString(blockMap, "content"); content != "" {
						textParts = append(textParts, content)
					}
				}
			}
		}
		result := strings.Join(textParts, "")
		if result != "" {
			return result
		}
	}

	// Try content as simple string (older format)
	if contentStr := extractString(msg, "content"); contentStr != "" {
		return contentStr
	}

	return ""
}
