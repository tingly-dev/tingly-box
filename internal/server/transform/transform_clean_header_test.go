package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// =============================================
// NormalizeSteganographyText tests
// =============================================

func TestNormalizeSteganographyText_Apostrophe_U2019(t *testing.T) {
	// U+2019 RIGHT SINGLE QUOTATION MARK → standard U+0027
	input := "Today’s date is 2026-06-30."
	want := "Today's date is 2026-06-30."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(U+2019) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_Apostrophe_U02BC(t *testing.T) {
	// U+02BC MODIFIER LETTER APOSTROPHE → standard U+0027
	input := "Todayʼs date is 2026-06-30."
	want := "Today's date is 2026-06-30."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(U+02BC) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_Apostrophe_U2018(t *testing.T) {
	// U+2018 LEFT SINGLE QUOTATION MARK → standard U+0027
	input := "Today‘s date is 2026-06-30."
	want := "Today's date is 2026-06-30."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(U+2018) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_DateSeparator(t *testing.T) {
	// YYYY/MM/DD → YYYY-MM-DD
	input := "Today's date is 2026/06/30."
	want := "Today's date is 2026-06-30."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(date slash) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_Combined(t *testing.T) {
	// Full steganographic attack: both apostrophe substitution + date separator
	input := "Today’s date is 2026/06/30."
	want := "Today's date is 2026-06-30."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(combined) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_NonTodayApostrophesNotAffected(t *testing.T) {
	// Non-"Today" look-alike apostrophes are intentionally NOT normalized.
	// This is a targeted countermeasure that only fixes the known "Today's"
	// pattern to avoid side effects on legitimate Unicode punctuation.
	input := "Why didn’t they go? She couldnʼt decide. It wasn‘t clear."
	want := "Why didn’t they go? She couldnʼt decide. It wasn‘t clear."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(non-today apostrophes) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_MultipleDates(t *testing.T) {
	// Multiple dates with slashes
	input := "Dates: 2026/06/30 and 2025/12/01 and 2024/01/15."
	want := "Dates: 2026-06-30 and 2025-12-01 and 2024-01-15."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(multiple dates) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_NormalText(t *testing.T) {
	// Text without any steganographic markers should be unchanged
	input := "Today's date is 2026-06-30. This is fine."
	want := "Today's date is 2026-06-30. This is fine."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(normal) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_EmptyString(t *testing.T) {
	if got := NormalizeSteganographyText(""); got != "" {
		t.Errorf("NormalizeSteganographyText(empty) = %q, want %q", got, "")
	}
}

func TestNormalizeSteganographyText_NonDateSlash(t *testing.T) {
	// A slash that is NOT part of a date pattern should be preserved
	input := "The path is /usr/local/bin and the ratio is 1000/1."
	want := "The path is /usr/local/bin and the ratio is 1000/1."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(non-date slash) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_RealWorldSystemPrompt(t *testing.T) {
	// Real-world-like system prompt with steganographic markers.
	// Only "Today's" is normalized; "Don't" (non-Today pattern) is preserved.
	input := "The current date is 2026/06/30. Today’s weather is sunny. Don’t forget to be helpful."
	want := "The current date is 2026-06-30. Today's weather is sunny. Don’t forget to be helpful."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(real world) = %q, want %q", got, want)
	}
}

func TestNormalizeSteganographyText_SingleQuoteNotAffected(t *testing.T) {
	// Standard single quotes (U+0027) should not be changed
	input := "It's a test. Don't change this."
	want := "It's a test. Don't change this."
	if got := NormalizeSteganographyText(input); got != want {
		t.Errorf("NormalizeSteganographyText(standard quotes) = %q, want %q", got, want)
	}
}

// =============================================
// CleanSystemMessages tests
// =============================================

func TestCleanSystemMessages_RemovesBillingHeader(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "x-anthropic-billing-header: cc_version=1.0; cc_entrypoint=cli; cch=abc123;"},
		{Text: "The current date is 2026-06-30. You are Claude."},
	}
	result := CleanSystemMessages(blocks)
	if len(result) != 1 {
		t.Fatalf("CleanSystemMessages() returned %d blocks, want 1", len(result))
	}
	if result[0].Text != "The current date is 2026-06-30. You are Claude." {
		t.Errorf("CleanSystemMessages() = %q, want %q", result[0].Text, "The current date is 2026-06-30. You are Claude.")
	}
}

func TestCleanSystemMessages_NormalizesSteganography(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "Today’s date is 2026/06/30. Be helpful."},
	}
	result := CleanSystemMessages(blocks)
	if len(result) != 1 {
		t.Fatalf("CleanSystemMessages() returned %d blocks, want 1", len(result))
	}
	want := "Today's date is 2026-06-30. Be helpful."
	if result[0].Text != want {
		t.Errorf("CleanSystemMessages() = %q, want %q", result[0].Text, want)
	}
}

func TestCleanSystemMessages_BillingAndSteganography(t *testing.T) {
	// Both billing header and steganography in the same request
	blocks := []anthropic.TextBlockParam{
		{Text: "x-anthropic-billing-header: cc_version=1.0; cch=abc;"},
		{Text: "Today’s date is 2026/06/30."},
		{Text: "You are Claude."},
	}
	result := CleanSystemMessages(blocks)
	if len(result) != 2 {
		t.Fatalf("CleanSystemMessages() returned %d blocks, want 2", len(result))
	}
	if result[0].Text != "Today's date is 2026-06-30." {
		t.Errorf("CleanSystemMessages()[0] = %q, want %q", result[0].Text, "Today's date is 2026-06-30.")
	}
	if result[1].Text != "You are Claude." {
		t.Errorf("CleanSystemMessages()[1] = %q, want %q", result[1].Text, "You are Claude.")
	}
}

func TestCleanSystemMessages_EmptyBlocks(t *testing.T) {
	result := CleanSystemMessages(nil)
	if result != nil {
		t.Errorf("CleanSystemMessages(nil) = %v, want nil", result)
	}

	result = CleanSystemMessages([]anthropic.TextBlockParam{})
	if len(result) != 0 {
		t.Errorf("CleanSystemMessages(empty) = %v, want empty", result)
	}
}

func TestCleanSystemMessages_NormalTextPreserved(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "You are a helpful assistant."},
		{Text: "The current date is 2026-06-30."},
	}
	result := CleanSystemMessages(blocks)
	if len(result) != 2 {
		t.Fatalf("CleanSystemMessages() returned %d blocks, want 2", len(result))
	}
	if result[0].Text != "You are a helpful assistant." {
		t.Errorf("CleanSystemMessages()[0] = %q, want %q", result[0].Text, "You are a helpful assistant.")
	}
	if result[1].Text != "The current date is 2026-06-30." {
		t.Errorf("CleanSystemMessages()[1] = %q, want %q", result[1].Text, "The current date is 2026-06-30.")
	}
}

func TestCleanSystemMessages_PreservesClaudeCodePreambleByDefault(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "You are Claude Code, Anthropic's official CLI for Claude."},
		{Text: "Project instructions must be preserved."},
	}
	result := CleanSystemMessages(blocks)
	if len(result) != 2 {
		t.Fatalf("CleanSystemMessages() returned %d blocks, want 2", len(result))
	}
	if result[0].Text != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Errorf("CleanSystemMessages()[0] = %q, want %q", result[0].Text, "You are Claude Code, Anthropic's official CLI for Claude.")
	}
}

func TestCleanSystemMessages_CodexModeStripsClaudeCodePreamblePrefix(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "You are Claude Code, Anthropic's official CLI for Claude.\n\nFollow the repository conventions."},
	}
	result := cleanSystemMessages(blocks, true)
	if len(result) != 1 {
		t.Fatalf("CleanSystemMessages() returned %d blocks, want 1", len(result))
	}
	if result[0].Text != "Follow the repository conventions." {
		t.Errorf("CleanSystemMessages()[0] = %q, want %q", result[0].Text, "Follow the repository conventions.")
	}
}

func TestCleanSystemMessages_CodexModeRemovesClaudeCodeFileSearchPreamble(t *testing.T) {
	blocks := []anthropic.TextBlockParam{
		{Text: "You are a file search specialist for Claude Code, Anthropic's official CLI for Claude. You excel at thoroughly navigating and exploring codebases.\n\nUse ripgrep first."},
	}
	result := cleanSystemMessages(blocks, true)
	if len(result) != 1 {
		t.Fatalf("CleanSystemMessages() returned %d blocks, want 1", len(result))
	}
	if result[0].Text != "Use ripgrep first." {
		t.Errorf("CleanSystemMessages()[0] = %q, want %q", result[0].Text, "Use ripgrep first.")
	}
}

// =============================================
// CleanBetaSystemMessages tests
// =============================================

func TestCleanBetaSystemMessages_RemovesBillingHeader(t *testing.T) {
	blocks := []anthropic.BetaTextBlockParam{
		{Text: "x-anthropic-billing-header: cc_version=1.0; cch=abc;"},
		{Text: "You are Claude."},
	}
	result := CleanBetaSystemMessages(blocks)
	if len(result) != 1 {
		t.Fatalf("CleanBetaSystemMessages() returned %d blocks, want 1", len(result))
	}
	if result[0].Text != "You are Claude." {
		t.Errorf("CleanBetaSystemMessages() = %q, want %q", result[0].Text, "You are Claude.")
	}
}

func TestCleanBetaSystemMessages_NormalizesSteganography(t *testing.T) {
	blocks := []anthropic.BetaTextBlockParam{
		{Text: "Todayʼs date is 2026/06/30."},
	}
	result := CleanBetaSystemMessages(blocks)
	if len(result) != 1 {
		t.Fatalf("CleanBetaSystemMessages() returned %d blocks, want 1", len(result))
	}
	want := "Today's date is 2026-06-30."
	if result[0].Text != want {
		t.Errorf("CleanBetaSystemMessages() = %q, want %q", result[0].Text, want)
	}
}

func TestCleanBetaSystemMessages_EmptyBlocks(t *testing.T) {
	result := CleanBetaSystemMessages(nil)
	if result != nil {
		t.Errorf("CleanBetaSystemMessages(nil) = %v, want nil", result)
	}

	result = CleanBetaSystemMessages([]anthropic.BetaTextBlockParam{})
	if len(result) != 0 {
		t.Errorf("CleanBetaSystemMessages(empty) = %v, want empty", result)
	}
}

func TestCleanBetaSystemMessages_PreservesClaudeCodePreambleByDefault(t *testing.T) {
	blocks := []anthropic.BetaTextBlockParam{
		{Text: "You are Claude Code, Anthropic's official CLI for Claude."},
		{Text: "Project instructions must be preserved."},
	}
	result := CleanBetaSystemMessages(blocks)
	if len(result) != 2 {
		t.Fatalf("CleanBetaSystemMessages() returned %d blocks, want 2", len(result))
	}
	if result[0].Text != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Errorf("CleanBetaSystemMessages()[0] = %q, want %q", result[0].Text, "You are Claude Code, Anthropic's official CLI for Claude.")
	}
}

// =============================================
// CleanMessages tests
// =============================================

func TestCleanMessages_NormalizesSteganographyInFirstMessage(t *testing.T) {
	// The steganographic date string lives in a <system-reminder> block
	// in the first user message's text content.
	messages := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>\nToday’s date is 2026/06/30.\n</system-reminder>"}},
			},
		},
	}
	result := CleanMessages(messages)
	if len(result) != 1 {
		t.Fatalf("CleanMessages() returned %d messages, want 1", len(result))
	}
	if result[0].Content[0].OfText == nil {
		t.Fatal("CleanMessages() first content block is not text")
	}
	want := "<system-reminder>\nToday's date is 2026-06-30.\n</system-reminder>"
	if result[0].Content[0].OfText.Text != want {
		t.Errorf("CleanMessages() = %q, want %q", result[0].Content[0].OfText.Text, want)
	}
}

func TestCleanMessages_MultipleMessages(t *testing.T) {
	// First message has <system-reminder> → should be normalized.
	// Second and third lack <system-reminder> → should NOT be normalized.
	messages := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>\nToday’s date is 2026/06/30.\n</system-reminder>"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "I will help you with that."}},
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "What about 2025/12/01?"}},
			},
		},
	}
	result := CleanMessages(messages)
	if len(result) != 3 {
		t.Fatalf("CleanMessages() returned %d messages, want 3", len(result))
	}
	// First message: has <system-reminder>, should be normalized
	if result[0].Content[0].OfText.Text != "<system-reminder>\nToday's date is 2026-06-30.\n</system-reminder>" {
		t.Errorf("CleanMessages()[0] = %q, want %q", result[0].Content[0].OfText.Text, "<system-reminder>\nToday's date is 2026-06-30.\n</system-reminder>")
	}
	// Second message: no <system-reminder>, should NOT be modified
	if result[1].Content[0].OfText.Text != "I will help you with that." {
		t.Errorf("CleanMessages()[1] = %q, want %q", result[1].Content[0].OfText.Text, "I will help you with that.")
	}
	// Third message: no <system-reminder>, should NOT be modified
	if result[2].Content[0].OfText.Text != "What about 2025/12/01?" {
		t.Errorf("CleanMessages()[2] = %q, want %q", result[2].Content[0].OfText.Text, "What about 2025/12/01?")
	}
}

func TestCleanMessages_NonTextBlocksSkipped(t *testing.T) {
	// Non-text blocks (e.g., tool_use, image) should not cause panics
	messages := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>\nToday’s date is 2026/06/30.\n</system-reminder>"}},
				{}, // Empty block, no OfText and no other type
			},
		},
	}
	result := CleanMessages(messages)
	if len(result) != 1 {
		t.Fatalf("CleanMessages() returned %d messages, want 1", len(result))
	}
	if result[0].Content[0].OfText == nil {
		t.Fatal("CleanMessages() first text block was incorrectly set to nil")
	}
	want := "<system-reminder>\nToday's date is 2026-06-30.\n</system-reminder>"
	if result[0].Content[0].OfText.Text != want {
		t.Errorf("CleanMessages() = %q, want %q", result[0].Content[0].OfText.Text, want)
	}
}

func TestCleanMessages_EmptyMessages(t *testing.T) {
	result := CleanMessages(nil)
	if result != nil {
		t.Errorf("CleanMessages(nil) = %v, want nil", result)
	}
	result = CleanMessages([]anthropic.MessageParam{})
	if len(result) != 0 {
		t.Errorf("CleanMessages(empty) = %v, want empty", result)
	}
}

func TestCleanMessages_EmptyContent(t *testing.T) {
	// Message with no content blocks should be safe
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{}},
	}
	result := CleanMessages(messages)
	if len(result) != 1 {
		t.Fatalf("CleanMessages() returned %d messages, want 1", len(result))
	}
}

// =============================================
// CleanBetaMessages tests
// =============================================

func TestCleanBetaMessages_NormalizesSteganography(t *testing.T) {
	messages := []anthropic.BetaMessageParam{
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "<system-reminder>\nTodayʼs date is 2026/06/30.\n</system-reminder>"}},
			},
		},
	}
	result := CleanBetaMessages(messages)
	if len(result) != 1 {
		t.Fatalf("CleanBetaMessages() returned %d messages, want 1", len(result))
	}
	if result[0].Content[0].OfText == nil {
		t.Fatal("CleanBetaMessages() first content block is not text")
	}
	want := "<system-reminder>\nToday's date is 2026-06-30.\n</system-reminder>"
	if result[0].Content[0].OfText.Text != want {
		t.Errorf("CleanBetaMessages() = %q, want %q", result[0].Content[0].OfText.Text, want)
	}
}

func TestCleanBetaMessages_NonSystemReminderNotModified(t *testing.T) {
	// Message without <system-reminder> should NOT be normalized
	messages := []anthropic.BetaMessageParam{
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "Todayʼs date is 2026/06/30."}},
			},
		},
	}
	result := CleanBetaMessages(messages)
	if len(result) != 1 {
		t.Fatalf("CleanBetaMessages() returned %d messages, want 1", len(result))
	}
	if result[0].Content[0].OfText == nil {
		t.Fatal("CleanBetaMessages() first content block is not text")
	}
	// Should remain unchanged because there's no <system-reminder> tag
	want := "Todayʼs date is 2026/06/30."
	if result[0].Content[0].OfText.Text != want {
		t.Errorf("CleanBetaMessages() = %q, want %q", result[0].Content[0].OfText.Text, want)
	}
}

func TestCleanBetaMessages_EmptyMessages(t *testing.T) {
	result := CleanBetaMessages(nil)
	if result != nil {
		t.Errorf("CleanBetaMessages(nil) = %v, want nil", result)
	}
	result = CleanBetaMessages([]anthropic.BetaMessageParam{})
	if len(result) != 0 {
		t.Errorf("CleanBetaMessages(empty) = %v, want empty", result)
	}
}

// =============================================
// CleanHeaderTransform Apply tests
// =============================================

func TestCleanHeaderTransform_Name(t *testing.T) {
	tr := NewCleanHeaderTransform()
	if name := tr.Name(); name != "clean_header" {
		t.Errorf("CleanHeaderTransform.Name() = %q, want %q", name, "clean_header")
	}
}

func TestCleanHeaderTransform_Apply_AnthropicV1(t *testing.T) {
	tr := NewCleanHeaderTransform()
	req := &anthropic.MessageNewParams{
		System: []anthropic.TextBlockParam{
			{Text: "x-anthropic-billing-header: cc_version=1.0; cch=abc;"},
			{Text: "Today’s date is 2026/06/30."},
		},
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>\nToday’s date is 2026/06/30.\n</system-reminder>"}},
				},
			},
		},
	}
	ctx := &protocoltransform.TransformContext{Request: req}
	if err := tr.Apply(ctx); err != nil {
		t.Fatalf("CleanHeaderTransform.Apply() error = %v", err)
	}
	// Check system blocks
	if len(req.System) != 1 {
		t.Fatalf("CleanHeaderTransform.Apply() System returned %d blocks, want 1", len(req.System))
	}
	wantSys := "Today's date is 2026-06-30."
	if req.System[0].Text != wantSys {
		t.Errorf("CleanHeaderTransform.Apply() System = %q, want %q", req.System[0].Text, wantSys)
	}
	// Check messages
	if len(req.Messages) != 1 {
		t.Fatalf("CleanHeaderTransform.Apply() Messages returned %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Content[0].OfText == nil {
		t.Fatal("CleanHeaderTransform.Apply() first content block is not text")
	}
	wantMsg := "<system-reminder>\nToday's date is 2026-06-30.\n</system-reminder>"
	if req.Messages[0].Content[0].OfText.Text != wantMsg {
		t.Errorf("CleanHeaderTransform.Apply() Messages = %q, want %q", req.Messages[0].Content[0].OfText.Text, wantMsg)
	}
}

func TestCleanHeaderTransform_Apply_AnthropicV1CodexResponsesStripsClaudeCodePreamble(t *testing.T) {
	tr := NewCleanHeaderTransform()
	req := &anthropic.MessageNewParams{
		System: []anthropic.TextBlockParam{
			{Text: "You are Claude Code, Anthropic's official CLI for Claude."},
			{Text: "You are Claude Code, Anthropic's official CLI for Claude.\n\nFollow the repository conventions."},
		},
	}
	ctx := &protocoltransform.TransformContext{
		Request:   req,
		TargetAPI: protocol.TypeOpenAIResponses,
		Provider: &typ.Provider{
			AuthType:    typ.AuthTypeOAuth,
			OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerCodex},
		},
	}
	if err := tr.Apply(ctx); err != nil {
		t.Fatalf("CleanHeaderTransform.Apply() error = %v", err)
	}
	if len(req.System) != 1 {
		t.Fatalf("CleanHeaderTransform.Apply() System returned %d blocks, want 1", len(req.System))
	}
	if req.System[0].Text != "Follow the repository conventions." {
		t.Errorf("CleanHeaderTransform.Apply() System = %q, want %q", req.System[0].Text, "Follow the repository conventions.")
	}
}

func TestCleanHeaderTransform_Apply_AnthropicV1NonCodexPreservesClaudeCodePreamble(t *testing.T) {
	tr := NewCleanHeaderTransform()
	req := &anthropic.MessageNewParams{
		System: []anthropic.TextBlockParam{
			{Text: "You are Claude Code, Anthropic's official CLI for Claude."},
		},
	}
	ctx := &protocoltransform.TransformContext{
		Request:   req,
		TargetAPI: protocol.TypeOpenAIResponses,
		Provider: &typ.Provider{
			AuthType:    typ.AuthTypeOAuth,
			OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerOpenAI},
		},
	}
	if err := tr.Apply(ctx); err != nil {
		t.Fatalf("CleanHeaderTransform.Apply() error = %v", err)
	}
	if len(req.System) != 1 {
		t.Fatalf("CleanHeaderTransform.Apply() System returned %d blocks, want 1", len(req.System))
	}
	if req.System[0].Text != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Errorf("CleanHeaderTransform.Apply() System = %q, want %q", req.System[0].Text, "You are Claude Code, Anthropic's official CLI for Claude.")
	}
}

func TestCleanHeaderTransform_Apply_AnthropicBeta(t *testing.T) {
	tr := NewCleanHeaderTransform()
	req := &anthropic.BetaMessageNewParams{
		System: []anthropic.BetaTextBlockParam{
			{Text: "x-anthropic-billing-header: cc_version=1.0; cch=abc;"},
			{Text: "Today‘s date is 2026/06/30."},
		},
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "<system-reminder>\nToday‘s date is 2026/06/30.\n</system-reminder>"}},
				},
			},
		},
	}
	ctx := &protocoltransform.TransformContext{Request: req}
	if err := tr.Apply(ctx); err != nil {
		t.Fatalf("CleanHeaderTransform.Apply() error = %v", err)
	}
	// Check system blocks
	if len(req.System) != 1 {
		t.Fatalf("CleanHeaderTransform.Apply() System returned %d blocks, want 1", len(req.System))
	}
	wantSys := "Today's date is 2026-06-30."
	if req.System[0].Text != wantSys {
		t.Errorf("CleanHeaderTransform.Apply() System = %q, want %q", req.System[0].Text, wantSys)
	}
	// Check messages
	if len(req.Messages) != 1 {
		t.Fatalf("CleanHeaderTransform.Apply() Messages returned %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Content[0].OfText == nil {
		t.Fatal("CleanHeaderTransform.Apply() first content block is not text")
	}
	wantMsg := "<system-reminder>\nToday's date is 2026-06-30.\n</system-reminder>"
	if req.Messages[0].Content[0].OfText.Text != wantMsg {
		t.Errorf("CleanHeaderTransform.Apply() Messages = %q, want %q", req.Messages[0].Content[0].OfText.Text, wantMsg)
	}
}
