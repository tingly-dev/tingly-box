package smart_compact

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// buildConversationXML converts v1 messages into an XML conversation string.
func buildConversationXML(messages []anthropic.MessageParam, pathUtil *PathUtil) string {
	var xmlBuilder strings.Builder
	xmlBuilder.WriteString("<conversation>")
	messagesV1ToXML(messages, pathUtil, &xmlBuilder)
	xmlBuilder.WriteString("</conversation>")
	return xmlBuilder.String()
}

// buildBetaConversationXML converts beta messages into an XML conversation string.
func buildBetaConversationXML(messages []anthropic.BetaMessageParam, pathUtil *PathUtil) string {
	var xmlBuilder strings.Builder
	xmlBuilder.WriteString("<conversation>")
	messagesBetaToXML(messages, pathUtil, &xmlBuilder)
	xmlBuilder.WriteString("</conversation>")
	return xmlBuilder.String()
}

// messagesV1ToXML converts v1 messages to XML format, writing into xmlBuilder.
func messagesV1ToXML(messages []anthropic.MessageParam, pathUtil *PathUtil, xmlBuilder *strings.Builder) {
	var collectedFiles []string

	// First pass: collect files from tool_use blocks
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						files := pathUtil.ExtractFromMap(inputMap)
						collectedFiles = append(collectedFiles, files...)
					}
				}
			}
		}
	}

	// Deduplicate files
	collectedFiles = deduplicate(collectedFiles)

	// Build XML
	for _, msg := range messages {
		role := string(msg.Role)

		if role == "user" {
			text := extractV1Text(&msg)
			if text != "" {
				xmlBuilder.WriteString(fmt.Sprintf("<user>\n%s\n</user>\n\n", text))
			}
		} else if role == "assistant" {
			text := extractV1Text(&msg)
			xmlBuilder.WriteString(fmt.Sprintf("<assistant>\n%s\n</assistant>\n\n", text))

			if len(collectedFiles) > 0 {
				xmlBuilder.WriteString("<tool_calls>\n")
				for _, file := range collectedFiles {
					xmlBuilder.WriteString(fmt.Sprintf("<file>\n%s\n</file>\n\n", file))
				}
				xmlBuilder.WriteString("</tool_calls>\n")
			}

			// Clear files after first assistant
			collectedFiles = nil
		}
	}
}

// messagesBetaToXML converts beta messages to XML format, writing into xmlBuilder.
func messagesBetaToXML(messages []anthropic.BetaMessageParam, pathUtil *PathUtil, xmlBuilder *strings.Builder) {
	var collectedFiles []string

	// First pass: collect files from tool_use blocks
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						files := pathUtil.ExtractFromMap(inputMap)
						collectedFiles = append(collectedFiles, files...)
					}
				}
			}
		}
	}

	// Deduplicate files
	collectedFiles = deduplicate(collectedFiles)

	// Build XML
	for _, msg := range messages {
		role := string(msg.Role)

		if role == "user" {
			text := extractBetaText(&msg)
			if text != "" {
				xmlBuilder.WriteString(fmt.Sprintf("<user>\n%s\n</user>\n\n", text))
			}
		} else if role == "assistant" {
			text := extractBetaText(&msg)
			xmlBuilder.WriteString(fmt.Sprintf("<assistant>\n%s\n</assistant>\n\n", text))

			if len(collectedFiles) > 0 {
				xmlBuilder.WriteString("<tool_calls>\n")
				for _, file := range collectedFiles {
					xmlBuilder.WriteString(fmt.Sprintf("<file>\n%s\n</file>\n\n", file))
				}
				xmlBuilder.WriteString("</tool_calls>\n")
			}

			// Clear files after first assistant
			collectedFiles = nil
		}
	}
}

// extractV1Text extracts text content from a v1 message (user text and tool result text).
func extractV1Text(msg *anthropic.MessageParam) string {
	var text strings.Builder
	for _, block := range msg.Content {
		if block.OfText != nil {
			text.WriteString(block.OfText.Text)
			text.WriteString("\n")
		} else if block.OfToolResult != nil {
			for _, b := range block.OfToolResult.Content {
				if b.OfText != nil {
					text.WriteString(b.OfText.Text)
					text.WriteString("\n")
				}
			}
		}
	}
	return text.String()
}

// extractBetaText extracts text content from a beta message (user text and tool result text).
func extractBetaText(msg *anthropic.BetaMessageParam) string {
	var text strings.Builder
	for _, block := range msg.Content {
		if block.OfText != nil {
			text.WriteString(block.OfText.Text)
			text.WriteString("\n")
		} else if block.OfToolResult != nil {
			for _, b := range block.OfToolResult.Content {
				if b.OfText != nil {
					text.WriteString(b.OfText.Text)
					text.WriteString("\n")
				}
			}
		}
	}
	return text.String()
}
