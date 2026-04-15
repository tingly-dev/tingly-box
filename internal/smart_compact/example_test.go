package smart_compact

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestExample_PrintAllStrategies(t *testing.T) {
	input := []anthropic.BetaMessageParam{
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("read main.go and fix the bug"),
			},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("I'll read the file first."),
				anthropic.NewBetaToolUseBlock("tool-1", map[string]any{"path": "main.go"}, "read_file"),
			},
		},
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaToolResultBlock("tool-1", "func main() { panic(\"bug\") }", false),
			},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("Found the bug. Writing the fix."),
				anthropic.NewBetaToolUseBlock("tool-2", map[string]any{"path": "main.go", "content": "func main() {}"}, "write_file"),
			},
		},
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaToolResultBlock("tool-2", "ok", false),
				anthropic.NewBetaTextBlock("<command>compact</command>"),
			},
		},
	}

	printStrategy := func(name string, result []anthropic.BetaMessageParam) {
		fmt.Printf("\n═══════════════════════════════════════\n")
		fmt.Printf("  Strategy: %s\n", name)
		fmt.Printf("  Output messages: %d\n", len(result))
		fmt.Printf("═══════════════════════════════════════\n")
		for i, msg := range result {
			fmt.Printf("[%d] role=%s  blocks=%d\n", i, msg.Role, len(msg.Content))
			for j, block := range msg.Content {
				switch {
				case block.OfText != nil:
					fmt.Printf("    [%d] text: %s\n", j, truncate(block.OfText.Text, 120))
				case block.OfCompaction != nil:
					fmt.Printf("    [%d] compaction: %s\n", j, truncate(block.OfCompaction.Content.Value, 120))
				case block.OfToolUse != nil:
					inputJSON, _ := json.Marshal(block.OfToolUse.Input)
					fmt.Printf("    [%d] tool_use  id=%s name=%s input=%s\n", j, block.OfToolUse.ID, block.OfToolUse.Name, inputJSON)
				case block.OfToolResult != nil:
					fmt.Printf("    [%d] tool_result tool_use_id=%s\n", j, block.OfToolResult.ToolUseID)
				case block.OfDocument != nil:
					src := block.OfDocument.Source
					if src.OfText != nil {
						fmt.Printf("    [%d] document(text): %s\n", j, truncate(src.OfText.Data, 120))
					}
				default:
					fmt.Printf("    [%d] (other block)\n", j)
				}
			}
		}
	}

	printStrategy("compaction", NewXMLCompactionStrategy().CompressBeta(input))
	printStrategy("replay", NewConversationReplayStrategy().CompressBeta(input))
	printStrategy("document", NewConversationDocumentStrategy().CompressBeta(input))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
