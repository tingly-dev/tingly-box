package permission

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// AskUserQuestionHandler handles the AskUserQuestion tool which presents
// multiple choice questions to the user
type AskUserQuestionHandler struct{}

// NewAskUserQuestionHandler creates a new AskUserQuestionHandler
func NewAskUserQuestionHandler() *AskUserQuestionHandler {
	return &AskUserQuestionHandler{}
}

// CanHandle returns true for AskUserQuestion tool
func (h *AskUserQuestionHandler) CanHandle(toolName string, input map[string]interface{}) bool {
	return toolName == "AskUserQuestion"
}

// Description returns the handler description
func (h *AskUserQuestionHandler) Description() string {
	return "Handler for AskUserQuestion tool with multi-option selection"
}

// BuildPrompt creates a prompt showing all questions and options
func (h *AskUserQuestionHandler) BuildPrompt(req agentboot.PermissionRequest) string {
	var text strings.Builder

	text.WriteString("❓ *Question*\n\n")

	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		text.WriteString("_No questions provided_\n")
		return text.String()
	}

	for i, q := range questions {
		question, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		questionText, _ := question["question"].(string)
		header, _ := question["header"].(string)

		if header != "" {
			text.WriteString(fmt.Sprintf("*%s*\n", header))
		}
		text.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, questionText))

		// Show options
		options, ok := question["options"].([]interface{})
		if ok && len(options) > 0 {
			text.WriteString("Options:\n")
			for j, opt := range options {
				option, ok := opt.(map[string]interface{})
				if !ok {
					continue
				}
				label, _ := option["label"].(string)
				desc, _ := option["description"].(string)
				if desc != "" {
					text.WriteString(fmt.Sprintf("  %d️⃣ %s - %s\n", j+1, label, desc))
				} else {
					text.WriteString(fmt.Sprintf("  %d️⃣ %s\n", j+1, label))
				}
			}
		}

		text.WriteString("\n")
	}

	text.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	text.WriteString("*Reply with the option number or label*")

	return text.String()
}

// ParseResponse parses the user's selection into the answers format
func (h *AskUserQuestionHandler) ParseResponse(req agentboot.PermissionRequest, response UserResponse) (agentboot.PermissionResult, error) {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   "No questions in request",
		}, nil
	}

	// Parse the selection
	answers := make(map[string]interface{})

	// Handle selection by index or label
	selection := strings.TrimSpace(response.Data)
	if selection == "" {
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   "No selection provided",
		}, nil
	}

	// Try to match against first question (most common case)
	// In the future, this could handle multiple questions
	if len(questions) > 0 {
		question, ok := questions[0].(map[string]interface{})
		if ok {
			options, ok := question["options"].([]interface{})
			if ok {
				selectedIndex, selectedLabel := h.parseSelection(selection, options)

				if selectedIndex >= 0 && selectedIndex < len(options) {
					// Store the selected option label as the answer
					if opt, ok := options[selectedIndex].(map[string]interface{}); ok {
						if label, ok := opt["label"].(string); ok {
							// Answers format: key is question identifier, value is selected label
							answers["0"] = label
						}
					}
				} else if selectedLabel != "" {
					// User typed the label directly
					answers["0"] = selectedLabel
				}
			}
		}
	}

	// Build updated input with answers
	updatedInput := make(map[string]interface{})
	for k, v := range req.Input {
		updatedInput[k] = v
	}
	updatedInput["answers"] = answers

	return agentboot.PermissionResult{
		Approved:     true,
		UpdatedInput: updatedInput,
		Reason:       "User selected option",
	}, nil
}

// parseSelection attempts to parse the user's selection
// Returns (index, label) - if index is -1, label contains the raw input
func (h *AskUserQuestionHandler) parseSelection(selection string, options []interface{}) (int, string) {
	// Try to parse as number
	var index int
	if _, err := fmt.Sscanf(selection, "%d", &index); err == nil {
		// Convert 1-based to 0-based index
		index--
		if index >= 0 && index < len(options) {
			return index, ""
		}
	}

	// Try to match by label (case-insensitive)
	selection = strings.ToLower(selection)
	for i, opt := range options {
		if option, ok := opt.(map[string]interface{}); ok {
			if label, ok := option["label"].(string); ok {
				if strings.ToLower(label) == selection {
					return i, ""
				}
			}
		}
	}

	// Return the raw input as label
	return -1, selection
}
