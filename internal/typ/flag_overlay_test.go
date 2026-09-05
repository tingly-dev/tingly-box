package typ

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestValidateFlagOverlay(t *testing.T) {
	cases := []struct {
		name    string
		overlay FlagOverlay
		wantErr string
	}{
		{"empty", nil, ""},
		{"bool ok", FlagOverlay{"skip_usage": raw("true")}, ""},
		{"string ok", FlagOverlay{"custom_user_agent": raw(`"x/1"`)}, ""},
		{"enum ok", FlagOverlay{"thinking_effort": raw(`"high"`)}, ""},
		{"enum empty is inactive", FlagOverlay{"thinking_effort": raw(`""`)}, ""},
		{"headers ok", FlagOverlay{"extra_headers": raw(`{"X-A":"1"}`)}, ""},
		{"unknown key", FlagOverlay{"nope": raw("true")}, `unknown flag "nope"`},
		{"bool wrong type", FlagOverlay{"skip_usage": raw(`"yes"`)}, "expected a boolean"},
		{"enum bad value", FlagOverlay{"thinking_effort": raw(`"turbo"`)}, "not one of the allowed options"},
		{"int negative", FlagOverlay{"session_affinity": raw("-1")}, "non-negative integer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFlagOverlay(tc.overlay)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestApplyFlagOverlay(t *testing.T) {
	base := RuleFlags{SkipUsage: true, ClaudeCodeCompat: true, ThinkingEffort: ThinkingEffortLevel("low")}

	t.Run("absent keys untouched, present keys replace", func(t *testing.T) {
		out, err := ApplyFlagOverlay(base, FlagOverlay{
			"use_max_completion_tokens": raw("true"),
			"thinking_effort":           raw(`"high"`),
		})
		require.NoError(t, err)
		assert.True(t, out.SkipUsage, "untouched")
		assert.True(t, out.ClaudeCodeCompat, "untouched")
		assert.True(t, out.UseMaxCompletionTokens)
		assert.Equal(t, ThinkingEffortLevel("high"), out.ThinkingEffort)
	})

	t.Run("explicit false clears an enabled flag", func(t *testing.T) {
		out, err := ApplyFlagOverlay(base, FlagOverlay{"skip_usage": raw("false")})
		require.NoError(t, err)
		assert.False(t, out.SkipUsage)
		assert.True(t, out.ClaudeCodeCompat)
	})

	t.Run("invalid overlay returns input unchanged", func(t *testing.T) {
		out, err := ApplyFlagOverlay(base, FlagOverlay{"skip_usage": raw(`"x"`)})
		require.Error(t, err)
		assert.Equal(t, base, out)
	})

	t.Run("empty overlay is identity", func(t *testing.T) {
		out, err := ApplyFlagOverlay(base, nil)
		require.NoError(t, err)
		assert.Equal(t, base, out)
	})
}

func TestFlagOverlayHeaderRoundTrip(t *testing.T) {
	in := FlagOverlay{"skip_usage": raw("false"), "thinking_effort": raw(`"max"`)}
	enc, err := EncodeFlagOverlay(in)
	require.NoError(t, err)
	assert.NotContains(t, enc, "=", "unpadded base64url")

	out, err := DecodeFlagOverlay(enc)
	require.NoError(t, err)
	assert.JSONEq(t, `false`, string(out["skip_usage"]))
	assert.JSONEq(t, `"max"`, string(out["thinking_effort"]))

	empty, err := EncodeFlagOverlay(nil)
	require.NoError(t, err)
	assert.Equal(t, "", empty)

	_, err = DecodeFlagOverlay("!!!not-base64!!!")
	assert.Error(t, err)
}
