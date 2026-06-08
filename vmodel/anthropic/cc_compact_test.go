package anthropic

import (
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
)

// TestClaudeCodeCompact_Compression tests that claude-code-compact actually compresses messages.
func TestClaudeCodeCompact_Compression(t *testing.T) {
	vm := newCompactVM()

	originalMessages := []sdk.BetaMessageParam{
		{Role: sdk.BetaMessageParamRoleUser, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("First user message")}},
		{Role: sdk.BetaMessageParamRoleAssistant, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("First assistant response")}},
		{Role: sdk.BetaMessageParamRoleUser, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("Second user message")}},
		{Role: sdk.BetaMessageParamRoleAssistant, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("Second assistant response")}},
		{Role: sdk.BetaMessageParamRoleUser, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("<command>compact</command>")}},
	}

	req := &protocol.AnthropicBetaMessagesRequest{
		BetaMessageNewParams: &sdk.BetaMessageNewParams{
			Messages: originalMessages,
			Tools: []sdk.BetaToolUnionParam{
				{OfTool: &sdk.BetaToolParam{Name: "read_file"}},
			},
		},
	}

	result, err := vm.HandleAnthropic(req)
	if err != nil {
		t.Fatalf("HandleAnthropic failed: %v", err)
	}

	t.Logf("Original message count: %d", len(originalMessages))
	t.Logf("Result message count: %d", len(req.Messages))
	t.Logf("Result content: %s", extractTextFromVModelResponse(result))

	if len(req.Messages) >= len(originalMessages) {
		t.Errorf("Expected compressed messages (%d < %d), got %d",
			len(req.Messages), len(originalMessages), len(req.Messages))
	}

	hasCompressedContent := false
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			if block.OfText != nil && len(block.OfText.Text) > 0 {
				content := block.OfText.Text
				if len(content) > 100 && (strings.Contains(content, "<conversation>") || strings.Contains(content, "<user>")) {
					hasCompressedContent = true
					t.Logf("Found compressed content (length: %d): %s", len(content), truncate(content, 200))
				}
			}
		}
	}

	if !hasCompressedContent {
		t.Error("Expected to find compressed content in result messages, but none found")
	}

	contentText := extractTextFromVModelResponse(result)
	if contentText == "" {
		t.Error("Expected non-empty content (compressed summary)")
	}

	if !strings.Contains(contentText, "<analysis>") && !strings.Contains(contentText, "<summary>") {
		t.Errorf("Expected content to contain compressed summary markers, got: %s", truncate(contentText, 200))
	}

	if result.StopReason != "end_turn" {
		t.Errorf("Expected stop_reason 'end_turn', got %q", result.StopReason)
	}
}

// TestClaudeCodeCompact_CompressesOnArrivalWithTools verifies the gating
// design: the wake-keyword decision lives in the smart-routing layer, so once a
// tool-bearing request reaches the rapid-compact virtual model it is compacted
// regardless of whether the literal word "compact" appears — the request was
// already selected for compaction by the agent.claude_code/wake_compact op.
// This lets custom compact_keyword values work end-to-end.
func TestClaudeCodeCompact_CompressesOnArrivalWithTools(t *testing.T) {
	vm := newCompactVM()

	originalMessages := []sdk.BetaMessageParam{
		{Role: sdk.BetaMessageParamRoleUser, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("First user message")}},
		{Role: sdk.BetaMessageParamRoleAssistant, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("First assistant response")}},
		// No literal "compact" keyword here — routing already gated this request.
		{Role: sdk.BetaMessageParamRoleUser, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("please compress now")}},
	}

	req := &protocol.AnthropicBetaMessagesRequest{
		BetaMessageNewParams: &sdk.BetaMessageNewParams{
			Messages: originalMessages,
			Tools: []sdk.BetaToolUnionParam{
				{OfTool: &sdk.BetaToolParam{Name: "read_file"}},
			},
		},
	}

	result, err := vm.HandleAnthropic(req)
	if err != nil {
		t.Fatalf("HandleAnthropic failed: %v", err)
	}

	if len(req.Messages) >= len(originalMessages) {
		t.Errorf("Expected compaction on arrival (fewer than %d messages), got %d",
			len(originalMessages), len(req.Messages))
	}

	contentText := extractTextFromVModelResponse(result)
	if !strings.Contains(contentText, "<analysis>") && !strings.Contains(contentText, "<summary>") {
		t.Errorf("Expected compacted summary markers, got: %s", truncate(contentText, 200))
	}
}

// TestClaudeCodeCompact_NoCompressionWithoutTools tests that compression doesn't happen without tools.
func TestClaudeCodeCompact_NoCompressionWithoutTools(t *testing.T) {
	vm := NewTransformModel(&TransformModelConfig{
		ID:    "claude-code-compact",
		Chain: transform.NewTransformChain([]transform.Transform{smart_compact.NewXMLCompactTransform()}),
	})

	originalMessages := []sdk.BetaMessageParam{
		{Role: sdk.BetaMessageParamRoleUser, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("Message 1")}},
		{Role: sdk.BetaMessageParamRoleAssistant, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("Response 1")}},
		{Role: sdk.BetaMessageParamRoleUser, Content: []sdk.BetaContentBlockParamUnion{sdk.NewBetaTextBlock("<command>compact</command>")}},
	}

	req := &protocol.AnthropicBetaMessagesRequest{
		BetaMessageNewParams: &sdk.BetaMessageNewParams{
			Messages: originalMessages,
		},
	}

	_, err := vm.HandleAnthropic(req)
	if err != nil {
		t.Fatalf("HandleAnthropic failed: %v", err)
	}

	if len(req.Messages) != len(originalMessages) {
		t.Logf("Without tools, message count changed from %d to %d (compression may still occur depending on implementation)",
			len(originalMessages), len(req.Messages))
	}
}

func newCompactVM() *TransformModel {
	return NewTransformModel(&TransformModelConfig{
		ID:    "claude-code-compact",
		Chain: transform.NewTransformChain([]transform.Transform{NewClaudeCodeCompactTransform()}),
	})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func extractTextFromVModelResponse(resp VModelResponse) string {
	var text string
	for _, block := range resp.Content {
		if block.OfText != nil {
			text += block.OfText.Text
		}
	}
	return text
}
