package ops

import (
	"regexp"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
)

func TestClampAnthropicEffort(t *testing.T) {
	assert.Equal(t, anthropic.OutputConfigEffortLow, clampAnthropicEffort("minimal"),
		"minimal enters the ladder at low")
	assert.Equal(t, anthropic.OutputConfigEffortHigh, clampAnthropicEffort(anthropic.OutputConfigEffortXhigh),
		"xhigh steps down to high")
	assert.Equal(t, anthropic.OutputConfigEffortLow, clampAnthropicEffort(anthropic.OutputConfigEffortLow))
	assert.Equal(t, anthropic.OutputConfigEffortMedium, clampAnthropicEffort(anthropic.OutputConfigEffortMedium))
	assert.Equal(t, anthropic.OutputConfigEffortHigh, clampAnthropicEffort(anthropic.OutputConfigEffortHigh))
	assert.Equal(t, anthropic.OutputConfigEffortMax, clampAnthropicEffort(anthropic.OutputConfigEffortMax),
		"max passes through unclamped — no per-model cap without a catalog")
	assert.Equal(t, anthropic.OutputConfigEffort(""), clampAnthropicEffort(""))
}


func TestApplyAnthropicModelTransform_V1_DisabledThinkingStripsStaleEffort(t *testing.T) {
	// Regression: a client (or a stale UI selector) can send thinking=disabled
	// together with a leftover output_config.effort. Effort only makes sense
	// paired with active thinking, so it must be scrubbed even on a model that
	// otherwise supports effort — not just clamped to a supported level.
	req := &anthropic.MessageNewParams{
		Model:        anthropic.Model("claude-opus-4-5-20251101"),
		MaxTokens:    int64(4096),
		Thinking:     anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortHigh},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "claude-opus-4-5-20251101")

	assert.Equal(t, anthropic.OutputConfigEffort(""), result.OutputConfig.Effort)
}

func TestApplyAnthropicModelTransform_Beta_DisabledThinkingStripsStaleEffort(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model:        anthropic.Model("claude-opus-4-5-20251101"),
		MaxTokens:    int64(4096),
		Thinking:     anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortHigh},
		Messages: []anthropic.BetaMessageParam{
			{Role: "user", Content: []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: "Hello"}}}},
		},
	}

	result := ApplyAnthropicBetaModelTransform(req, "claude-opus-4-5-20251101")

	assert.Equal(t, anthropic.BetaOutputConfigEffort(""), result.OutputConfig.Effort)
}

func TestApplyAnthropicModelTransform_V1_ThinkingPassesThroughRegardlessOfModel(t *testing.T) {
	// No model-specific caps are consulted anymore: thinking config is passed
	// through unchanged for any model, known or not.
	for _, model := range []string{"claude-opus-4-7", "claude-3-5-haiku-20241022", "some-unknown-model"} {
		t.Run(model, func(t *testing.T) {
			req := &anthropic.MessageNewParams{
				Model:     anthropic.Model(model),
				MaxTokens: int64(4096),
				Thinking: anthropic.ThinkingConfigParamUnion{
					OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
				},
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
				},
			}

			result := ApplyAnthropicV1ModelTransform(req, model)

			assert.NotNil(t, result.Thinking.OfAdaptive, "adaptive thinking must survive untouched")
			assert.Nil(t, result.Thinking.OfEnabled)
		})
	}
}

func TestApplyAnthropicModelTransform_V1_EffortClampedAndSentForAnyModel(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("some-unknown-model"),
		MaxTokens: int64(4096),
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: "minimal"},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		},
	}

	result := ApplyAnthropicV1ModelTransform(req, "some-unknown-model")

	assert.Equal(t, anthropic.OutputConfigEffortLow, result.OutputConfig.Effort,
		"effort is clamped and sent even for a model this package knows nothing about")
}

func TestApplyAnthropicModelTransform_V1_Haiku_Enabled(t *testing.T) {
	// Test case: Haiku model with enabled thinking should keep thinking
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

	assert.NotNil(t, result)
	assert.NotNil(t, result.Thinking.OfEnabled, "Thinking.OfEnabled should be preserved")
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

func TestGenHex5_LengthAndChars(t *testing.T) {
	hexPattern := regexp.MustCompile(`^[0-9a-f]{5}$`)
	for i := 0; i < 100; i++ {
		result := GenHex5()
		assert.Len(t, result, 5, "GenHex5 should return exactly 5 chars")
		assert.True(t, hexPattern.MatchString(result), "GenHex5 should return lowercase hex: %q", result)
	}
}

func TestGenHex5_IsRandom(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		seen[GenHex5()] = true
	}
	// 20 bits = 1048576 possible values, 100 samples should have high uniqueness
	assert.Greater(t, len(seen), 90, "GenHex5 should produce mostly unique values")
}

func TestClaudeCodeVersion(t *testing.T) {
	assert.Equal(t, "2.1.86", ClaudeCodeVersion)
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
