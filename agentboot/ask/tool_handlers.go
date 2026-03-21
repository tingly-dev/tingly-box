package ask

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
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
func (h *AskUserQuestionHandler) BuildPrompt(req Request) string {
	var text strings.Builder

	text.WriteString("❓ *Question*\n\n")

	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		text.WriteString("_No questions provided_\n")
		logrus.WithFields(map[string]interface{}{
			"has_questions":  ok,
			"questions_type": fmt.Sprintf("%T", req.Input["questions"]),
		}).Debug("BuildPrompt: questions parsing result")
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

		// Show options with clear mapping to buttons
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
				// Format to match button labels (Option 1, Option 2, etc.)
				if desc != "" {
					text.WriteString(fmt.Sprintf("  *Option %d*: %s - %s\n", j+1, label, desc))
				} else {
					text.WriteString(fmt.Sprintf("  *Option %d*: %s\n", j+1, label))
				}
			}
		}

		text.WriteString("\n")
	}

	text.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	text.WriteString("*Click a button below to select*")

	return text.String()
}

// ParseResponse parses the user's selection into the answers format
func (h *AskUserQuestionHandler) ParseResponse(req Request, response Response) (Result, error) {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "No questions in request",
		}, nil
	}

	// Parse the selection
	answers := make(map[string]interface{})

	// Handle selection by index or label
	selection := strings.TrimSpace(response.Data)
	if selection == "" {
		return Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "No selection provided",
		}, nil
	}

	// Try to match against first question (most common case)
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

	return Result{
		ID:           req.ID,
		Approved:     true,
		UpdatedInput: updatedInput,
		Reason:       "User selected option",
	}, nil
}

// parseSelection attempts to parse the user's selection
// Returns (index, label) - if index is -1, label contains the raw input
// selection can be:
//   - 1-based number (user types "1" for first option)
//   - 0-based index from callback (e.g., "0", "1")
//   - label text (exact or case-insensitive match)
func (h *AskUserQuestionHandler) parseSelection(selection string, options []interface{}) (int, string) {
	// Try to parse as number
	var index int
	if _, err := fmt.Sscanf(selection, "%d", &index); err == nil {
		// Check if it's a valid 0-based index first (from callback)
		if index >= 0 && index < len(options) {
			// Could be 0-based index from callback, verify by checking if it matches
			return index, ""
		}
		// Try as 1-based index (user input), convert to 0-based
		index--
		if index >= 0 && index < len(options) {
			return index, ""
		}
	}

	// Try to match by label (case-insensitive)
	selectionLower := strings.ToLower(selection)
	for i, opt := range options {
		if option, ok := opt.(map[string]interface{}); ok {
			if label, ok := option["label"].(string); ok {
				if strings.ToLower(label) == selectionLower {
					return i, ""
				}
			}
		}
	}

	// Return the raw input as label
	return -1, selection
}

// DefaultToolHandler is the fallback handler for tools without specific handlers
type DefaultToolHandler struct{}

// NewDefaultToolHandler creates a new DefaultToolHandler
func NewDefaultToolHandler() *DefaultToolHandler {
	return &DefaultToolHandler{}
}

// CanHandle returns true for all tools (acts as fallback)
func (h *DefaultToolHandler) CanHandle(toolName string, input map[string]interface{}) bool {
	// Don't handle tools that have specific handlers
	// This acts as a fallback only
	return toolName != "AskUserQuestion"
}

// Description returns the handler description
func (h *DefaultToolHandler) Description() string {
	return "Default handler for simple approve/deny decisions"
}

// BuildPrompt creates a simple permission prompt
func (h *DefaultToolHandler) BuildPrompt(req Request) string {
	return BuildDefaultPrompt(req)
}

// ParseResponse parses a simple approve/deny response
func (h *DefaultToolHandler) ParseResponse(req Request, response Response) (Result, error) {
	return ParseDefaultResponse(req, response)
}

// BuildDefaultPrompt creates the default permission prompt text
func BuildDefaultPrompt(req Request) string {
	text := "🔐 *Tool Permission Request*\n\n"
	text += "Tool: `" + req.ToolName + "`\n"

	// Show relevant input details
	text += "Args: \n"
	for key, value := range req.Input {
		if s, ok := value.(string); ok && s != "" {
			if len(s) > 20 {
				s = s[:20] + "..."
			}
			text += fmt.Sprintf("%s: `%s`\n", key, value)
		}
	}

	if req.Reason != "" {
		text += "\nReason: " + req.Reason + "\n"
	}

	logrus.Infof("prompt: %s", text)
	return text
}

// ParseDefaultResponse parses standard allow/deny responses
func ParseDefaultResponse(req Request, response Response) (Result, error) {
	switch response.Data {
	case "allow", "yes", "y", "1":
		return Result{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
			Reason:       "User approved",
		}, nil
	case "always":
		return Result{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
			Remember:     true,
			Reason:       "User approved (always)",
		}, nil
	case "deny", "no", "n", "0":
		return Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "User denied",
		}, nil
	default:
		return Result{
			ID:       req.ID,
			Approved: false,
			Reason:   "Unknown response",
		}, nil
	}
}

// ParseTextResponse parses user text input as a permission response
// Returns: (approved, remember, isValid)
func ParseTextResponse(text string) (approved bool, remember bool, isValid bool) {
	text = normalizeText(text)

	switch text {
	case "1", "y", "yes":
		return true, false, true
	case "0", "n", "no":
		return false, false, true
	case "a", "always":
		return true, true, true
	default:
		return false, false, false
	}
}

// normalizeText normalizes user input for comparison
func normalizeText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ToLower(text)
	return text
}

// truncateText truncates text to maxLen with ellipsis
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// escapeMarkdown escapes special characters for Telegram Markdown format
// Based on practical escaping scheme that covers common cases
func escapeMarkdown(text string) string {
	// Order matters for some replacements
	text = strings.ReplaceAll(text, "_", "\\_")
	text = strings.ReplaceAll(text, "-", "\\-")
	text = strings.ReplaceAll(text, "~", "\\~")
	text = strings.ReplaceAll(text, "`", "\\`")
	text = strings.ReplaceAll(text, ".", "\\.")
	text = strings.ReplaceAll(text, "<", "\\<")
	text = strings.ReplaceAll(text, ">", "\\>")
	return text
}
