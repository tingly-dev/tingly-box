package request

import (
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	// OpenAI tool call ID max length (40 characters per OpenAI API spec)
	maxToolCallIDLength = 40
)

type handler func(map[string]interface{}) map[string]interface{}

// truncateToolCallID ensures tool call ID doesn't exceed OpenAI's 40 character limit
// OpenAI API requires tool_call.id to be <= 40 characters
func truncateToolCallID(id string) string {
	if len(id) <= maxToolCallIDLength {
		return id
	}
	// Truncate to max length and add a suffix to indicate truncation
	return id[:maxToolCallIDLength]
}

// mapAnthropicToolIDs reserves existing short IDs before assigning distinct
// bounded IDs to long ones. Both calls and results share this request map.
func mapAnthropicToolIDs(messages []anthropic.MessageParam) map[string]string {
	var ids []string
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil {
				ids = append(ids, block.OfToolUse.ID)
			} else if block.OfToolResult != nil {
				ids = append(ids, block.OfToolResult.ToolUseID)
			}
		}
	}
	mapped := make(map[string]string, len(ids))
	used := make(map[string]bool, len(ids))
	for _, id := range ids {
		if len(id) <= maxToolCallIDLength {
			mapped[id], used[id] = id, true
		}
	}
	for _, id := range ids {
		if _, ok := mapped[id]; ok {
			continue
		}
		for n := 0; ; n++ {
			sum := sha256.Sum256([]byte(id + "\x00" + strconv.Itoa(n)))
			candidate := fmt.Sprintf("%x", sum)[:maxToolCallIDLength]
			if !used[candidate] {
				mapped[id], used[candidate] = candidate, true
				break
			}
		}
	}
	return mapped
}
