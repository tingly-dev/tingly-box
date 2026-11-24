package translation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai"
)

func TestTranslationModelPreservesMeasuredUsageOnIncompleteOutput(t *testing.T) {
	measured := ai.NewTokenUsage(10, 20)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	translator := ModelTranslator{Kind: LLM, Model: "any-local-model", Complete: func(got context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, *ai.TokenUsage, error) {
		require.Same(t, ctx, got)
		require.Equal(t, context.Canceled, got.Err())
		var response openai.ChatCompletion
		require.NoError(t, json.Unmarshal([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"partial"}}]}`), &response))
		return &response, measured, nil
	}}
	out, err := translator.Translate(ctx, TextInput{Text: "hello", SourceLang: "auto", TargetLang: "en-GB"})
	require.Error(t, err)
	require.Empty(t, out.Result.Text)
	require.Same(t, measured, out.Tokens)
}

func TestTranslationBackendLanguageBoundaries(t *testing.T) {
	_, err := NativeLanguage("libretranslate", "zh-Hant")
	require.Error(t, err)
	_, err = NativeLanguage("libretranslate", "pt-BR")
	require.Error(t, err)
	lang, err := NativeLanguage("deepl", "en-GB")
	require.NoError(t, err)
	require.Equal(t, "EN-GB", lang)
	require.True(t, ModelTranslator{Kind: LLM}.Supports("auto", "pt-BR"))
	require.False(t, ModelTranslator{Kind: HunyuanMT}.Supports("auto", "pt-BR"))
}
