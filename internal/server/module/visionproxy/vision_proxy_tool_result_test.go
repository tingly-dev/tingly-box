package visionproxy

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
)

// Regression for the case the original processBeta/processV1 missed:
// images nested inside a tool_result block were not seen at all because
// the walker only inspected top-level OfImage. Tool-returning agents
// (Claude Code's screenshot / read-image / many MCP tools) deliver images
// exactly this way, so the proxy looked broken from the user's side.

func TestVisionProxy_AnthropicBeta_ToolResultImage_Described(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("a screenshot of a terminal")
	p := mkProcessor(t, fake, prov)

	imgInTR := anthropic.BetaToolResultBlockParamContentUnion{
		OfImage: &anthropic.BetaImageBlockParam{
			Source: anthropic.BetaImageBlockParamSourceUnion{
				OfBase64: &anthropic.BetaBase64ImageSourceParam{
					Data:      tinyPNGBase64,
					MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
				},
			},
		},
	}
	req := &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("downstream-text-model"),
		Messages: []anthropic.BetaMessageParam{{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "here's the screenshot"}},
				{OfToolResult: &anthropic.BetaToolResultBlockParam{
					ToolUseID: "tool-1",
					Content:   []anthropic.BetaToolResultBlockParamContentUnion{imgInTR},
				}},
			},
		}},
	}
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 1, fake.callCount(), "vision client called for the tool_result-nested image")
	require.Equal(t, 0, countImages(req), "no image blocks remain anywhere (top-level OR inside tool_result)")
	require.Contains(t, collectText(req), "a screenshot of a terminal", "description spliced into the tool_result content")
}

func TestVisionProxy_AnthropicV1_ToolResultImage_Described(t *testing.T) {
	prov := mkProvider("anthropic-v1")
	fake := newFakeVisionClient("a cat photo")
	p := mkProcessor(t, fake, prov)

	imgInTR := anthropic.ToolResultBlockParamContentUnion{
		OfImage: &anthropic.ImageBlockParam{
			Source: anthropic.ImageBlockParamSourceUnion{
				OfBase64: &anthropic.Base64ImageSourceParam{
					Data:      tinyPNGBase64,
					MediaType: anthropic.Base64ImageSourceMediaType(tinyPNGMediaType),
				},
			},
		},
	}
	req := &anthropic.MessageNewParams{
		Model: anthropic.Model("downstream-text-model"),
		Messages: []anthropic.MessageParam{{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					Content:   []anthropic.ToolResultBlockParamContentUnion{imgInTR},
				}},
			},
		}},
	}
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 1, fake.callCount())
	require.Equal(t, 0, countImages(req))
	require.Contains(t, collectText(req), "a cat photo")
}

// Historical images inside a tool_result must still be stripped (with the
// fixed marker, no vision call), exactly like top-level historical images.
func TestVisionProxy_AnthropicBeta_HistoricalToolResultImage_StrippedNoCall(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("LATEST")
	p := mkProcessor(t, fake, prov)

	historicalImg := anthropic.BetaToolResultBlockParamContentUnion{
		OfImage: &anthropic.BetaImageBlockParam{
			Source: anthropic.BetaImageBlockParamSourceUnion{
				OfBase64: &anthropic.BetaBase64ImageSourceParam{
					Data:      tinyPNGBase64,
					MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
				},
			},
		},
	}
	req := &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("downstream-text-model"),
		Messages: []anthropic.BetaMessageParam{
			// historical turn carrying a tool_result image
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfToolResult: &anthropic.BetaToolResultBlockParam{
						ToolUseID: "tool-old",
						Content:   []anthropic.BetaToolResultBlockParamContentUnion{historicalImg},
					}},
				},
			},
			// latest turn — text only, no vision call expected
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "follow-up question"}},
				},
			},
		},
	}
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 0, fake.callCount(), "no vision call for historical images")
	require.Equal(t, 0, countImages(req))
	require.True(t, strings.Contains(collectText(req), imageHistoricalText), "historical marker present")
}
