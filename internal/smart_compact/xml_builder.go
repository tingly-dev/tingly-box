package smart_compact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// keyToolsPreserve lists tool names whose call request AND result must survive
// flattening with positional correctness (call first, result adjacent, inside
// the assistant turn that issued them). Internal only — not exposed to users.
//
// Rationale: as a Claude Code compaction adapter, claude-code-compact flattens
// the conversation into a narrative (information flattening — see
// .design/smart-compact-design.md "核心心智模型"). Most tool I/O is summarized
// away, but some tools carry facts the model must keep verbatim — facts that
// cannot be re-derived from the environment after compaction:
//   - Task:           subagent conclusions (independent reasoning product)
//   - AskUserQuestion: user decisions/choices that constrain the work
//   - WebFetch:       externally fetched information (costly/volatile to re-fetch)
//
// Candidates listed below are intentionally disabled (`false`). They are kept in
// the map as documentation of the decision so future enablement is a one-line
// flip. Each is disabled because its output is re-derivable from the environment
// or is redundant with an enabled tool:
//   - WebSearch:   search results can be re-run; WebFetch already covers the
//     "external fact" case for the fetched page.
//   - TodoWrite / TaskCreate / TaskUpdate: intent/progress. Currently disabled —
//     CC plans are usually mirrored into the live task list which
//     the model re-reads; enable if compaction is observed to lose
//     track of in-progress work.
//   - Bash:        output is re-runnable. The command text can be a load-bearing
//     decision (e.g. `git checkout`), but env state is better
//     re-probed via Read than trusted from a stale transcript.
//   - ExitPlanMode: an approved plan is a load-bearing decision, but CC mirrors
//     plans into TodoWrite, so it is covered transitively.
//   - Read/Glob/Grep/LS/Edit/Write/NotebookEdit/Skill/Workflow/MCP: pure
//     retrieval or execution whose artifacts live in the filesystem
//     / environment — summarizing them away is correct.
var keyToolsPreserve = map[string]bool{
	// Enabled.
	"Task":            true,
	"AskUserQuestion": true,
	"WebFetch":        true,

	// Disabled candidates (kept for documentation; flip to true to enable).
	"WebSearch":    false,
	"TodoWrite":    false,
	"TaskCreate":   false,
	"TaskUpdate":   false,
	"Bash":         false,
	"ExitPlanMode": false,
}

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
//
// Single-pass, in original message order. Tool handling follows the
// information-flattening model:
//   - tool_result text is NOT folded into <user> text (it is emitted via its
//     tool's inline/summary path only).
//   - key tools (keyToolsPreserve) are inlined as <tool name=…>…</tool> followed
//     immediately by their <tool_result>…</tool_result>, inside the assistant
//     turn that issued the call.
//   - non-key tools contribute file paths to a per-turn <tool_calls> block
//     (fixes the old global-once bug where collectedFiles was cleared).
func messagesV1ToXML(messages []anthropic.MessageParam, pathUtil *PathUtil, xmlBuilder *strings.Builder) {
	// Index tool_result text by tool_use_id so a key tool's result can be placed
	// adjacent to its call when the call is emitted.
	resultTextByID := collectV1ResultText(messages)

	for _, msg := range messages {
		role := string(msg.Role)

		if role == "user" {
			text := extractV1UserText(&msg)
			if text != "" {
				xmlBuilder.WriteString(fmt.Sprintf("<user>\n%s\n</user>\n\n", text))
			}
		} else if role == "assistant" {
			xmlBuilder.WriteString("<assistant>\n")
			xmlBuilder.WriteString(extractV1AssistantText(&msg))

			// Per-turn file collection for non-key tools (replaces global collection).
			var nonKeyFiles []string

			for _, block := range msg.Content {
				if block.OfToolUse == nil {
					continue
				}
				name := block.OfToolUse.Name
				id := block.OfToolUse.ID

				if keyToolsPreserve[name] {
					// Inline key tool: call first, then its result adjacent.
					xmlBuilder.WriteString(fmt.Sprintf("<tool name=%q>%s</tool>\n", name, serializeToolInput(block.OfToolUse.Input)))
					resultText, hadResult := resultTextByID[id]
					if hadResult {
						xmlBuilder.WriteString(fmt.Sprintf("<tool_result>%s</tool_result>\n", resultText))
					}
				} else {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						nonKeyFiles = append(nonKeyFiles, pathUtil.ExtractFromMap(inputMap)...)
					}
				}
			}

			nonKeyFiles = deduplicate(nonKeyFiles)
			if len(nonKeyFiles) > 0 {
				xmlBuilder.WriteString("<tool_calls>\n")
				for _, file := range nonKeyFiles {
					xmlBuilder.WriteString(fmt.Sprintf("<file>\n%s\n</file>\n\n", file))
				}
				xmlBuilder.WriteString("</tool_calls>\n")
			}

			xmlBuilder.WriteString("</assistant>\n\n")
		}
	}
}

// messagesBetaToXML converts beta messages to XML format, writing into xmlBuilder.
// See messagesV1ToXML for the flattening model and tool handling.
func messagesBetaToXML(messages []anthropic.BetaMessageParam, pathUtil *PathUtil, xmlBuilder *strings.Builder) {
	resultTextByID := collectBetaResultText(messages)

	for _, msg := range messages {
		role := string(msg.Role)

		if role == "user" {
			text := extractBetaUserText(&msg)
			if text != "" {
				xmlBuilder.WriteString(fmt.Sprintf("<user>\n%s\n</user>\n\n", text))
			}
		} else if role == "assistant" {
			xmlBuilder.WriteString("<assistant>\n")
			xmlBuilder.WriteString(extractBetaAssistantText(&msg))

			var nonKeyFiles []string

			for _, block := range msg.Content {
				if block.OfToolUse == nil {
					continue
				}
				name := block.OfToolUse.Name
				id := block.OfToolUse.ID

				if keyToolsPreserve[name] {
					xmlBuilder.WriteString(fmt.Sprintf("<tool name=%q>%s</tool>\n", name, serializeToolInput(block.OfToolUse.Input)))
					resultText, hadResult := resultTextByID[id]
					if hadResult {
						xmlBuilder.WriteString(fmt.Sprintf("<tool_result>%s</tool_result>\n", resultText))
					}
				} else {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						nonKeyFiles = append(nonKeyFiles, pathUtil.ExtractFromMap(inputMap)...)
					}
				}
			}

			nonKeyFiles = deduplicate(nonKeyFiles)
			if len(nonKeyFiles) > 0 {
				xmlBuilder.WriteString("<tool_calls>\n")
				for _, file := range nonKeyFiles {
					xmlBuilder.WriteString(fmt.Sprintf("<file>\n%s\n</file>\n\n", file))
				}
				xmlBuilder.WriteString("</tool_calls>\n")
			}

			xmlBuilder.WriteString("</assistant>\n\n")
		}
	}
}

// collectV1ResultText indexes tool_result text content by tool_use_id.
// Preserves is_error via the returned string when set.
func collectV1ResultText(messages []anthropic.MessageParam) map[string]string {
	result := map[string]string{}
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			id := block.OfToolResult.ToolUseID
			if id == "" {
				continue
			}
			var text strings.Builder
			for _, b := range block.OfToolResult.Content {
				if b.OfText != nil {
					text.WriteString(b.OfText.Text)
					text.WriteString("\n")
				}
			}
			body := strings.TrimRight(text.String(), "\n")
			if block.OfToolResult.IsError.Value {
				body = fmt.Sprintf("[error] %s", body)
			}
			result[id] = body
		}
	}
	return result
}

// collectBetaResultText indexes tool_result text content by tool_use_id (beta).
func collectBetaResultText(messages []anthropic.BetaMessageParam) map[string]string {
	result := map[string]string{}
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			id := block.OfToolResult.ToolUseID
			if id == "" {
				continue
			}
			var text strings.Builder
			for _, b := range block.OfToolResult.Content {
				if b.OfText != nil {
					text.WriteString(b.OfText.Text)
					text.WriteString("\n")
				}
			}
			body := strings.TrimRight(text.String(), "\n")
			if block.OfToolResult.IsError.Value {
				body = fmt.Sprintf("[error] %s", body)
			}
			result[id] = body
		}
	}
	return result
}

// extractV1UserText extracts only the user's own text (OfText). tool_result text
// is intentionally excluded — it is emitted via the corresponding tool's path so
// the call/result stay positionally correct.
func extractV1UserText(msg *anthropic.MessageParam) string {
	var text strings.Builder
	for _, block := range msg.Content {
		if block.OfText != nil {
			text.WriteString(block.OfText.Text)
			text.WriteString("\n")
		}
	}
	return strings.TrimRight(text.String(), "\n")
}

// extractV1AssistantText extracts assistant text blocks (OfText). tool_use blocks
// are handled separately (key tools inlined, non-key summarized) and are NOT
// included here.
func extractV1AssistantText(msg *anthropic.MessageParam) string {
	var text strings.Builder
	for _, block := range msg.Content {
		if block.OfText != nil {
			text.WriteString(block.OfText.Text)
			text.WriteString("\n")
		}
	}
	s := text.String()
	if s == "" {
		return ""
	}
	return s
}

// extractBetaUserText extracts only the user's own text (beta). See extractV1UserText.
func extractBetaUserText(msg *anthropic.BetaMessageParam) string {
	var text strings.Builder
	for _, block := range msg.Content {
		if block.OfText != nil {
			text.WriteString(block.OfText.Text)
			text.WriteString("\n")
		}
	}
	return strings.TrimRight(text.String(), "\n")
}

// extractBetaAssistantText extracts assistant text blocks (beta). See extractV1AssistantText.
func extractBetaAssistantText(msg *anthropic.BetaMessageParam) string {
	var text strings.Builder
	for _, block := range msg.Content {
		if block.OfText != nil {
			text.WriteString(block.OfText.Text)
			text.WriteString("\n")
		}
	}
	return text.String()
}

// serializeToolInput renders a tool_use Input into a stable, readable string for
// inline display. Prefers canonical JSON (sorted keys) for fidelity; falls back
// to a generic format for non-marshallable inputs.
func serializeToolInput(input any) string {
	// Input may arrive as json.RawMessage / []byte (unmarshalled map) or an
	// already-decoded value. Canonicalize via json round-trip.
	var raw []byte
	switch v := input.(type) {
	case []byte:
		raw = v
	case json.RawMessage:
		raw = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		raw = b
	}

	var m any
	if err := json.Unmarshal(raw, &m); err != nil {
		return string(raw)
	}
	canonical, err := json.Marshal(m)
	if err != nil {
		return string(raw)
	}
	return string(canonical)
}
