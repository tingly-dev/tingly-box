package stream

// FilterSpecialFields removes special fields that have dedicated content blocks
// e.g., reasoning_content is handled as thinking block, not merged into text_delta
func FilterSpecialFields(extras map[string]interface{}) map[string]interface{} {
	if extras == nil || len(extras) == 0 {
		return extras
	}
	result := make(map[string]interface{})
	for k, v := range extras {
		if k != OpenaiFieldReasoningContent {
			result[k] = v
		}
	}
	return result
}
