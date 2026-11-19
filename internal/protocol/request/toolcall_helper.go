package request

const (
	// OpenAI tool call ID max length (40 characters per OpenAI API spec)
	maxToolCallIDLength = 40
)

// truncateToolCallID ensures tool call ID doesn't exceed OpenAI's 40 character limit
// OpenAI API requires tool_call.id to be <= 40 characters
func truncateToolCallID(id string) string {
	if len(id) <= maxToolCallIDLength {
		return id
	}
	// Truncate to max length and add a suffix to indicate truncation
	return id[:maxToolCallIDLength]
}
