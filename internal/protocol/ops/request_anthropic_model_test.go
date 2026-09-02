package ops

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAnthropicModelThinkingCaps(t *testing.T) {
	// Expectations mirror internal/protocol/catalog/claude.models.json — the
	// caps are catalog-driven, not hardcoded per model name.
	tests := []struct {
		name     string
		model    string
		adaptive bool
		budget   bool
		effort   []string // supported effort levels; empty = no effort support
	}{
		{
			name:     "Claude Opus 4.7 is adaptive-only",
			model:    "claude-opus-4-7",
			adaptive: true,
			budget:   false,
			effort:   []string{"low", "medium", "high", "xhigh", "max"},
		},
		{
			name:     "Claude Opus 4.6",
			model:    "claude-opus-4-6",
			adaptive: true,
			budget:   true,
			effort:   []string{"low", "medium", "high", "max"},
		},
		{
			name:     "Claude Opus 4.6 uppercase",
			model:    "CLAUDE-OPUS-4-6",
			adaptive: true,
			budget:   true,
			effort:   []string{"low", "medium", "high", "max"},
		},
		{
			name:     "Bedrock-decorated Sonnet 4.6 resolves via substring",
			model:    "us.anthropic.claude-sonnet-4-6-v1:0",
			adaptive: true,
			budget:   true,
			effort:   []string{"low", "medium", "high", "max"},
		},
		{
			name:     "Claude Opus 4.5 has effort low..high but no adaptive",
			model:    "claude-opus-4-5-20251101",
			adaptive: false,
			budget:   true,
			effort:   []string{"low", "medium", "high"},
		},
		{
			name:     "Undated Opus 4.5 resolves via family key",
			model:    "claude-opus-4-5",
			adaptive: false,
			budget:   true,
			effort:   []string{"low", "medium", "high"},
		},
		{
			name:     "Claude Haiku 4.5 is budget-only",
			model:    "claude-haiku-4-5-20251001",
			adaptive: false,
			budget:   true,
		},
		{
			name:     "Claude 3 Haiku has no thinking at all",
			model:    "claude-3-haiku-20240307",
			adaptive: false,
			budget:   false,
		},
		{
			name:     "Claude 3.5 Sonnet predates extended thinking",
			model:    "claude-3-5-sonnet-20241022",
			adaptive: false,
			budget:   false,
		},
		{
			name:     "Uncataloged model gets the legacy budget-only profile",
			model:    "claude-9-experimental",
			adaptive: false,
			budget:   true,
		},
		{
			name:   "Empty model gets legacy profile",
			model:  "",
			budget: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := anthropicModelThinkingCaps(tt.model)
			assert.Equal(t, tt.adaptive, caps.ThinkingAdaptive, "adaptive")
			assert.Equal(t, tt.budget, caps.ThinkingEnabled, "budget")
			assert.Equal(t, len(tt.effort) > 0, caps.SupportsEffort(), "supportsEffort")
			for _, level := range tt.effort {
				assert.True(t, caps.EffortLevels[level], "level %s should be supported", level)
			}
			assert.Equal(t, strings.Contains(strings.Join(tt.effort, ","), "xhigh"), caps.EffortLevels["xhigh"])
		})
	}
}

func TestClampAnthropicEffort(t *testing.T) {
	caps46 := anthropicModelThinkingCaps("claude-opus-4-6")           // low/medium/high/max
	caps47 := anthropicModelThinkingCaps("claude-opus-4-7")           // low/medium/high/xhigh/max
	caps45 := anthropicModelThinkingCaps("claude-opus-4-5")           // low/medium/high
	capsOld := anthropicModelThinkingCaps("claude-sonnet-4-20250514") // no effort

	assert.Equal(t, anthropic.OutputConfigEffortMax, clampAnthropicEffort(anthropic.OutputConfigEffortMax, caps46))
	assert.Equal(t, anthropic.OutputConfigEffortHigh, clampAnthropicEffort(anthropic.OutputConfigEffortXhigh, caps46),
		"xhigh steps down to the nearest supported level")
	assert.Equal(t, anthropic.OutputConfigEffortXhigh, clampAnthropicEffort(anthropic.OutputConfigEffortXhigh, caps47),
		"Opus 4.7 preserves its native xhigh level")
	assert.Equal(t, anthropic.OutputConfigEffortHigh, clampAnthropicEffort(anthropic.OutputConfigEffortMax, caps45),
		"Opus 4.5 has no max, steps down to high")
	assert.Equal(t, anthropic.OutputConfigEffortLow, clampAnthropicEffort("minimal", caps46),
		"minimal enters the ladder at low")
	assert.Equal(t, anthropic.OutputConfigEffort(""), clampAnthropicEffort(anthropic.OutputConfigEffortHigh, capsOld),
		"models without effort support get the field stripped")
}

func TestApplyAnthropicModelTransform_V1_Claude3Haiku_EnabledDisables(t *testing.T) {
	// claude-3-haiku has no extended thinking at all per the catalog — an
	// enabled(budget) request must degrade to disabled, not pass through.
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-haiku-20240307"),
		MaxTokens: int64(4096),
		Thinking:  anthropic.ThinkingConfigParamOfEnabled(4096),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-3-haiku-20240307")

	assert.Nil(t, result.Thinking.OfEnabled)
	assert.NotNil(t, result.Thinking.OfDisabled)
}

func TestApplyAnthropicModelTransform_V1_EffortWithoutThinkingPreserved(t *testing.T) {
	// Effort controls the whole response and does not require a thinking block.
	// A native client may therefore send output_config.effort by itself.
	req := &anthropic.MessageNewParams{
		Model:        anthropic.Model("claude-opus-5"),
		MaxTokens:    int64(4096),
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortMedium},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-opus-5")

	assert.Equal(t, anthropic.OutputConfigEffortMedium, result.OutputConfig.Effort)
}

func TestApplyAnthropicModelTransform_Beta_EffortWithoutThinkingPreserved(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model:        anthropic.Model("claude-opus-5"),
		MaxTokens:    int64(4096),
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortMedium},
		Messages: []anthropic.BetaMessageParam{
			{Role: "user", Content: []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: "Hello"}}}},
		},
	}

	result := ApplyAnthropicBetaModelTransform(req, "claude-opus-5")

	assert.Equal(t, anthropic.BetaOutputConfigEffortMedium, result.OutputConfig.Effort)
}

func TestApplyAnthropicModelTransform_V1_AdaptiveWithEffort_FallsBackToBudget(t *testing.T) {
	// Budget-only model + adaptive request carrying an effort level: the effort
	// converts to enabled(budget) instead of disabling thinking outright.
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-5-20250929"),
		MaxTokens: int64(64000),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortHigh},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-sonnet-4-5-20250929")

	assert.Nil(t, result.Thinking.OfAdaptive)
	if assert.NotNil(t, result.Thinking.OfEnabled, "adaptive+effort should fall back to enabled(budget)") {
		assert.Equal(t, typ.ThinkingBudgetMapping[typ.ThinkingEffortHigh], result.Thinking.OfEnabled.BudgetTokens)
	}
	assert.Equal(t, anthropic.OutputConfigEffort(""), result.OutputConfig.Effort,
		"effort must be stripped for models without effort support")
}

func TestApplyAnthropicModelTransform_V1_AdaptiveBudgetFallbackCappedByMaxTokens(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-haiku-4-5-20251001"),
		MaxTokens: int64(2048),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortMax},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-haiku-4-5-20251001")

	if assert.NotNil(t, result.Thinking.OfEnabled) {
		assert.Equal(t, int64(2047), result.Thinking.OfEnabled.BudgetTokens,
			"fallback budget must be strictly below max_tokens")
	}
}

func TestApplyAnthropicModelTransform_Beta_AdaptiveBudgetFallbackUsesStrictBound(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model:        anthropic.Model("claude-haiku-4-5-20251001"),
		MaxTokens:    2048,
		Thinking:     anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortMax},
	}

	result := ApplyAnthropicBetaModelTransform(req, "claude-haiku-4-5-20251001")

	if assert.NotNil(t, result.Thinking.OfEnabled) {
		assert.Equal(t, int64(2047), result.Thinking.OfEnabled.BudgetTokens)
	}
}

func TestApplyAnthropicModelTransform_AdaptiveBudgetFallbackDisablesWhenImpossible(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:        anthropic.Model("claude-haiku-4-5-20251001"),
		MaxTokens:    1024,
		Thinking:     anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-haiku-4-5-20251001")

	assert.Nil(t, result.Thinking.OfEnabled)
	assert.NotNil(t, result.Thinking.OfDisabled)
}

func TestApplyAnthropicModelTransform_V1_Opus47_BudgetConvertsToAdaptive(t *testing.T) {
	// Adaptive-only model (Opus 4.7) + enabled(budget) request: budget converts
	// to adaptive + effort derived from the budget tier.
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-7"),
		MaxTokens: int64(64000),
		Thinking:  anthropic.ThinkingConfigParamOfEnabled(31999),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-opus-4-7")

	assert.Nil(t, result.Thinking.OfEnabled, "enabled(budget) is not supported on Opus 4.7")
	assert.NotNil(t, result.Thinking.OfAdaptive, "budget request should convert to adaptive")
	assert.Equal(t, anthropic.OutputConfigEffortMax, result.OutputConfig.Effort,
		"a 32K budget tiers to effort=max")
}

func TestApplyAnthropicModelTransform_V1_Opus47_ExplicitEffortWins(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:        anthropic.Model("claude-opus-4-7"),
		MaxTokens:    int64(64000),
		Thinking:     anthropic.ThinkingConfigParamOfEnabled(31999),
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-opus-4-7")

	assert.NotNil(t, result.Thinking.OfAdaptive)
	assert.Equal(t, anthropic.OutputConfigEffortLow, result.OutputConfig.Effort,
		"an explicit effort level wins over the budget-derived tier")
}

func TestApplyAnthropicModelTransform_V1_Opus45_EffortMaxClampsToHigh(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:        anthropic.Model("claude-opus-4-5-20251101"),
		MaxTokens:    int64(64000),
		Thinking:     anthropic.ThinkingConfigParamOfEnabled(20480),
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortMax},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-opus-4-5-20251101")

	assert.NotNil(t, result.Thinking.OfEnabled, "budget thinking stays on Opus 4.5")
	assert.Equal(t, anthropic.OutputConfigEffortHigh, result.OutputConfig.Effort,
		"Opus 4.5's effort ladder stops at high")
}

func TestApplyAnthropicModelTransform_Beta_Opus47_BudgetConvertsToAdaptive(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-opus-4-7"),
		MaxTokens: int64(64000),
		Thinking:  anthropic.BetaThinkingConfigParamOfEnabled(4096),
		Messages: []anthropic.BetaMessageParam{
			{Role: "user", Content: []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: "Hello"}}}},
		},
	}

	result := ApplyAnthropicBetaModelTransform(req, "claude-opus-4-7")

	assert.Nil(t, result.Thinking.OfEnabled)
	assert.NotNil(t, result.Thinking.OfAdaptive)
	assert.Equal(t, anthropic.BetaOutputConfigEffortLow, result.OutputConfig.Effort,
		"a 4K budget tiers to effort=low")
}

func TestApplyAnthropicModelTransform_V1_Opus46_Adaptive(t *testing.T) {
	// Test case: Opus 4.6 model with adaptive thinking should keep thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-opus-4-6"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-opus-4-6")

	assert.NotNil(t, result)
	assert.NotNil(t, result.Thinking.OfAdaptive, "Thinking.OfAdaptive should be preserved for Opus 4.6")
}

func TestApplyAnthropicModelTransform_V1_Sonnet46_Adaptive(t *testing.T) {
	// Test case: Sonnet 4.6 model with adaptive thinking should keep thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-sonnet-4-6")

	assert.NotNil(t, result)
	assert.NotNil(t, result.Thinking.OfAdaptive, "Thinking.OfAdaptive should be preserved for Sonnet 4.6")
}

func TestApplyAnthropicModelTransform_V1_Haiku_Adaptive(t *testing.T) {
	// Test case: Haiku model with adaptive thinking should remove thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-3-5-haiku-20241022")

	assert.NotNil(t, result)
	assert.True(t, result.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil for Haiku")
	assert.True(t, result.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil for Haiku")
}

func TestApplyAnthropicModelTransform_V1_Sonnet35_Adaptive(t *testing.T) {
	// Test case: Sonnet 3.5 model with adaptive thinking should remove thinking (not 4.6)
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-3-5-sonnet-20241022")

	assert.NotNil(t, result)
	assert.True(t, result.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil for Sonnet 3.5")
	assert.True(t, result.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil for Sonnet 3.5")
}

func TestApplyAnthropicModelTransform_V1_Opus37_Adaptive(t *testing.T) {
	// Test case: Opus 3.7 model with adaptive thinking should remove thinking (not 4.6)
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-7-opus-20250214"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-3-7-opus-20250214")

	assert.NotNil(t, result)
	assert.True(t, result.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil for Opus 3.7")
	assert.True(t, result.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil for Opus 3.7")
}

func TestApplyAnthropicModelTransform_V1_Haiku45_Enabled(t *testing.T) {
	// Haiku 4.5 supports budget thinking — enabled passes through untouched.
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-haiku-4-5-20251001"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-haiku-4-5-20251001")

	assert.NotNil(t, result)
	assert.NotNil(t, result.Thinking.OfEnabled, "Thinking.OfEnabled should be preserved")
}

func TestApplyAnthropicModelTransform_V1_Haiku35_EnabledDisables(t *testing.T) {
	// Haiku 3.5 predates extended thinking (introduced with Sonnet 3.7) — an
	// enabled(budget) request degrades to disabled per the catalog.
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-3-5-haiku-20241022")

	assert.Nil(t, result.Thinking.OfEnabled)
	assert.NotNil(t, result.Thinking.OfDisabled)
}

func TestApplyAnthropicModelTransform_V1_NoThinking(t *testing.T) {
	// Test case: No thinking configured
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: int64(4096),
		Thinking:  anthropic.ThinkingConfigParamUnion{},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-3-5-haiku-20241022")

	assert.NotNil(t, result)
	assert.True(t, result.Thinking.OfAdaptive == nil, "Thinking.OfAdaptive should be nil")
	assert.True(t, result.Thinking.OfEnabled == nil, "Thinking.OfEnabled should be nil")
}

func TestApplyAnthropicModelTransform_NilRequest(t *testing.T) {
	// Test case: nil request
	result := ApplyAnthropicV1ModelTransform(nil, "claude-3-5-haiku-20241022")
	assert.Nil(t, result)
}

func TestFilterThinkingBlocksInMessages(t *testing.T) {
	// Test case: Filter thinking blocks from messages
	messages := []anthropic.MessageParam{
		{
			Role: "user",
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Hello"),
			},
		},
		{
			Role: "assistant",
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Thinking..."),
				// Note: Creating a thinking block requires proper construction
				// This test demonstrates the structure; actual implementation may vary
			},
		},
	}

	// The filter should remove messages with only thinking blocks
	result := filterThinkingBlocksInMessages(messages)
	assert.NotNil(t, result)
	// User message should be preserved
	assert.True(t, len(result) >= 1)
}

func TestApplyAnthropicMetadataTransform(t *testing.T) {
	// Test case: Haiku model with enabled thinking should keep thinking
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{},
		},
		System: []anthropic.TextBlockParam{
			{
				Text: "x-anthropic-billing-header",
			},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	deviceID := "ddd"
	accountID := "uuu"

	result := ApplyAnthropicV1MetadataTransform(req, map[string]any{
		"device":  deviceID,
		"user_id": accountID,
	})

	m := MetadataUserID{
		DeviceID:    deviceID,
		AccountUUID: accountID,
		SessionID:   "",
	}

	t.Logf("%#v", m)

	assert.NotNil(t, result)
	t.Logf("%#v", result.Metadata.UserID)
	t.Logf("%#v", result.System[0].Text)
	assert.True(t, strings.Contains(result.Metadata.UserID.String(), deviceID))
	assert.True(t, strings.Contains(result.Metadata.UserID.String(), accountID))
	assert.True(t, strings.Contains(result.Metadata.UserID.String(), "session_id"))
}

func TestClaudeCodeVersion(t *testing.T) {
	assert.Equal(t, "2.1.258", ClaudeCodeVersion)
	assert.Equal(t, constant.ClaudeCodeVersion, ClaudeCodeVersion)
}

func TestComputeFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		messageText string
		wantLen     int
		wantPrefix  string
	}{
		{
			name:        "short message 'hi' - all indices fallback to '0'",
			messageText: "hi",
			wantLen:     3,
			wantPrefix:  "", // just verify length and hex format
		},
		{
			name:        "empty string",
			messageText: "",
			wantLen:     3,
		},
		{
			name:        "exactly 5 chars",
			messageText: "hello",
			wantLen:     3,
		},
		{
			name:        "long message",
			messageText: "this is a longer message that exceeds index 20",
			wantLen:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := computeFingerprint(tt.messageText, ClaudeCodeVersion)

			// Must be exactly 3 hex chars
			assert.Len(t, fp, 3, "fingerprint must be 3 chars")
			assert.Regexp(t, `^[0-9a-f]{3}$`, fp, "fingerprint must be lowercase hex")

			// Deterministic: same input always produces same output
			fp2 := computeFingerprint(tt.messageText, ClaudeCodeVersion)
			assert.Equal(t, fp, fp2, "fingerprint must be deterministic")

			if tt.wantPrefix != "" {
				assert.True(t, strings.HasPrefix(fp, tt.wantPrefix), "fingerprint should start with %q, got %q", tt.wantPrefix, fp)
			}

			// Log for manual inspection
			t.Logf("messageText=%q len=%d chars=[%c%c%c] fingerprint=%s cc_version=%s.%s",
				tt.messageText, len(tt.messageText),
				func() byte {
					if 4 < len(tt.messageText) {
						return tt.messageText[4]
					}
					return '0'
				}(),
				func() byte {
					if 7 < len(tt.messageText) {
						return tt.messageText[7]
					}
					return '0'
				}(),
				func() byte {
					if 20 < len(tt.messageText) {
						return tt.messageText[20]
					}
					return '0'
				}(),
				fp, ClaudeCodeVersion, fp)
		})
	}
}

func TestComputeCCVersion(t *testing.T) {
	tests := []struct {
		name        string
		messageText string
	}{
		{name: "hi", messageText: "hi"},
		{name: "empty", messageText: ""},
		{name: "long message", messageText: "this is a longer message that exceeds index 20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ccVersion := computeCCVersion(tt.messageText)

			// Must start with base version
			assert.True(t, strings.HasPrefix(ccVersion, ClaudeCodeVersion+"."),
				"cc_version must start with %s., got %s", ClaudeCodeVersion, ccVersion)

			// Suffix must be exactly 3 hex chars
			suffix := strings.TrimPrefix(ccVersion, ClaudeCodeVersion+".")
			assert.Len(t, suffix, 3, "suffix must be 3 chars")
			assert.Regexp(t, `^[0-9a-f]{3}$`, suffix, "suffix must be lowercase hex")

			t.Logf("cc_version=%s", ccVersion)
		})
	}
}

func TestExtractFirstUserMessageText(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			{Role: "system", Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: "system msg"}}}},
			{Role: "user", Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: "hello world"}}}},
			{Role: "assistant", Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: "hi"}}}},
			{Role: "user", Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: "second user msg"}}}},
		},
	}

	text := extractFirstUserMessageText(req.Messages)
	assert.Equal(t, "hello world", text, "should extract first user message text")
}
