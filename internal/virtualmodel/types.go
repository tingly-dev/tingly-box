package virtualmodel

// Model represents a virtual model in the models list (OpenAI-compatible format).
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ToolCallConfig defines a tool call to be returned by the virtual model.
type ToolCallConfig struct {
	Name      string                 `json:"name" yaml:"name"`
	Arguments map[string]interface{} `json:"arguments" yaml:"arguments"`
}

// ToolCallDisplayContent extracts display text from tool call arguments.
// It checks for "message" and "question" keys, returning the first non-empty value found.
func ToolCallDisplayContent(args map[string]interface{}) string {
	if msg, ok := args["message"].(string); ok {
		return msg
	}
	if question, ok := args["question"].(string); ok {
		return question
	}
	return ""
}
