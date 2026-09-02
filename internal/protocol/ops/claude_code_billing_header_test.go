package ops

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildClaudeCodeBillingHeader_Fresh(t *testing.T) {
	got := BuildClaudeCodeBillingHeader("2.1.258.8ee", "")
	assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000;", got)
}

func TestBuildClaudeCodeBillingHeader_ReplacesClientOwnedFieldsKeepsSessionFields(t *testing.T) {
	inbound := "x-anthropic-billing-header: cc_version=2.1.240.abc; cc_entrypoint=sdk-cli; cc_workload=cron; cc_is_subagent=true; cc_prev_req=req_011CeAbCdEf; cc_prompt_id=0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b;"
	got := BuildClaudeCodeBillingHeader("2.1.258.8ee", inbound)
	assert.Equal(t,
		"x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000; cc_workload=cron; cc_is_subagent=true; cc_prev_req=req_011CeAbCdEf; cc_prompt_id=0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b;",
		got)
}

func TestBuildClaudeCodeBillingHeader_OrderIsCanonicalNotInbound(t *testing.T) {
	inbound := "x-anthropic-billing-header: cc_prompt_id=0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b; cc_is_subagent=true; cc_version=1; cc_entrypoint=x;"
	got := BuildClaudeCodeBillingHeader("2.1.258.8ee", inbound)
	assert.Equal(t,
		"x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000; cc_is_subagent=true; cc_prompt_id=0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b;",
		got)
}

func TestBuildClaudeCodeBillingHeader_DropsInvalidAndUnknownFields(t *testing.T) {
	inbound := "x-anthropic-billing-header: cc_version=2; cc_entrypoint=cli; cch=abcde; cc_is_subagent=false; cc_prev_req=nope; cc_prompt_id=not-a-uuid; cc_workload=bad value; cc_evil=1; garbage"
	got := BuildClaudeCodeBillingHeader("2.1.258.8ee", inbound)
	assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000;", got)
}

func TestBuildClaudeCodeBillingHeader_FirstDuplicateWins(t *testing.T) {
	inbound := "x-anthropic-billing-header: cc_workload=cron; cc_workload=other;"
	got := BuildClaudeCodeBillingHeader("2.1.258.8ee", inbound)
	assert.Contains(t, got, " cc_workload=cron;")
	assert.NotContains(t, got, "other")
}

func TestParseBillingHeaderFields(t *testing.T) {
	fields := parseBillingHeaderFields("  x-anthropic-billing-header: a=1;b = 2 ;;novalue; c=x=y;")
	assert.Equal(t, [][2]string{{"a", "1"}, {"b", "2"}, {"c", "x=y"}}, fields)
	assert.Empty(t, parseBillingHeaderFields(""))
}

func TestIsBillingHeaderText(t *testing.T) {
	assert.True(t, IsBillingHeaderText("x-anthropic-billing-header: cc_version=1;"))
	assert.True(t, IsBillingHeaderText("  \nx-anthropic-billing-header:"))
	assert.False(t, IsBillingHeaderText("You are Claude Code"))
	assert.False(t, IsBillingHeaderText("the x-anthropic-billing-header: is mentioned mid-text"))
}

// Captured from the 2.1.258 binary: prompt "say hi" → cc_version=2.1.258.8ee.
// The fingerprint hashes message bytes 4, 7 and 20 (or '0' when absent) with
// the salt and the version.
func TestComputeFingerprint_MatchesLiveCapture(t *testing.T) {
	assert.Equal(t, "8ee", computeFingerprint("say hi", "2.1.258"))
	assert.Equal(t, "2.1.258.8ee", computeCCVersion("say hi"))
}

// 2.1.258 folds its <system-reminder> meta messages into the first user
// message ahead of the prompt; the fingerprint must skip them (the live
// capture only reproduces with the prompt text, not the reminder).
func TestExtractFirstUserMessageText_SkipsSystemReminders(t *testing.T) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(
			anthropic.NewTextBlock("<system-reminder>\nAvailable agent types...\n</system-reminder>"),
			anthropic.NewTextBlock("<system-reminder>\nToday's date is 2026-09-02.\n</system-reminder>"),
			anthropic.NewTextBlock("say hi"),
		),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("second")),
	}
	assert.Equal(t, "say hi", extractFirstUserMessageText(messages))

	beta := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(
			anthropic.NewBetaTextBlock("  <system-reminder>x</system-reminder>"),
			anthropic.NewBetaTextBlock("say hi"),
		),
	}
	assert.Equal(t, "say hi", extractFirstBetaUserMessageText(beta))
}

func TestExtractFirstUserMessageText_ReminderOnlyMessageIsSkipped(t *testing.T) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("<system-reminder>only</system-reminder>")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("ok")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("real prompt")),
	}
	assert.Equal(t, "real prompt", extractFirstUserMessageText(messages))

	// A first user message with no text at all fingerprints as "" (the CLI
	// returns "" when the non-meta message has no text block).
	imageOnly := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewImageBlockBase64("image/png", "AAAA")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("later")),
	}
	assert.Equal(t, "", extractFirstUserMessageText(imageOnly))
	assert.Equal(t, "", extractFirstUserMessageText(nil))
}

func TestApplyAnthropicBetaMetadataTransform_BillingHeaderIsRebuiltInPlace(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model: "claude-sonnet-4-6",
		System: []anthropic.BetaTextBlockParam{
			{Text: "x-anthropic-billing-header: cc_version=2.1.240.abc; cc_entrypoint=sdk-cli; cc_is_subagent=true;"},
			{Text: "You are Claude Code, Anthropic's official CLI for Claude."},
		},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("say hi")),
		},
		Metadata: anthropic.BetaMetadataParam{
			UserID: param.NewOpt(`{"device_id":"d","account_uuid":"a","session_id":"s","parent_session_id":"p"}`),
		},
	}
	out := ApplyAnthropicBetaMetadataTransform(req, map[string]any{"device": "dev", "user_id": "acct"})
	require.Len(t, out.System, 2, "billing header replaced in place, not prepended")
	assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000; cc_is_subagent=true;", out.System[0].Text)
	assert.Equal(t, "You are Claude Code, Anthropic's official CLI for Claude.", out.System[1].Text)
	assert.Equal(t, `{"device_id":"dev","account_uuid":"acct","session_id":"s","parent_session_id":"p"}`, out.Metadata.UserID.String())
}

func TestApplyAnthropicV1MetadataTransform_BillingHeaderPrepended(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:  "claude-sonnet-4-6",
		System: []anthropic.TextBlockParam{{Text: "You are Claude Code, Anthropic's official CLI for Claude."}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("say hi")),
		},
	}
	out := ApplyAnthropicV1MetadataTransform(req, map[string]any{"device": "dev", "user_id": "acct"})
	require.Len(t, out.System, 2)
	assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=cli; cch=00000;", out.System[0].Text)
	assert.Contains(t, out.Metadata.UserID.String(), `"device_id":"dev"`)
	assert.NotContains(t, out.Metadata.UserID.String(), "parent_session_id")
}
