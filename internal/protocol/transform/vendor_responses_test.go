package transform

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newResponsesRequestWithPromptCache() *responses.ResponseNewParams {
	return &responses.ResponseNewParams{
		Model:                "gpt-5.6",
		PromptCacheKey:       openai.String("stable-affinity-key"),
		PromptCacheOptions:   responses.ResponseNewParamsPromptCacheOptions{Mode: "explicit"},
		PromptCacheRetention: responses.ResponseNewParamsPromptCacheRetention("24h"),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam{
			{OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleUser,
				Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: responses.ResponseInputMessageContentListParam{
					{OfInputText: &responses.ResponseInputTextParam{Text: "hi", PromptCacheBreakpoint: responses.NewResponseInputTextPromptCacheBreakpointParam()}},
				}},
			}},
		}},
	}
}

// VendorTransform used to pass Responses requests through untouched, so the
// OpenAI-only prompt-cache fields reached every Responses-compatible vendor
// (#1561).
func TestVendorTransform_Responses_PromptCacheFields(t *testing.T) {
	tests := []struct {
		providerURL string
		keep        bool
	}{
		{"https://integrate.api.nvidia.com/v1", false},
		{"https://api.openai.com/v1", true},
	}
	for _, tt := range tests {
		t.Run(tt.providerURL, func(t *testing.T) {
			ctx := withProvider(&TransformContext{Request: newResponsesRequestWithPromptCache()}, tt.providerURL)

			require.NoError(t, NewVendorTransform().Apply(ctx))

			req, ok := ctx.Request.(*responses.ResponseNewParams)
			require.True(t, ok)
			b, err := json.Marshal(req)
			require.NoError(t, err)
			body := string(b)
			for _, field := range []string{`"prompt_cache_options"`, `"prompt_cache_retention"`, `"prompt_cache_breakpoint"`} {
				if tt.keep {
					assert.Contains(t, body, field)
				} else {
					assert.NotContains(t, body, field)
				}
			}
			assert.Contains(t, body, `"prompt_cache_key":"stable-affinity-key"`)
		})
	}
}
