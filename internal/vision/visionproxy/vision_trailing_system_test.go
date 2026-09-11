package visionproxy

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// claudeCodeShapedBeta reproduces the message list Claude Code actually sends
// when it returns a tool result: a `<system-reminder>` arrives as its own
// system-role message both before the turn and, crucially, AFTER the
// tool_result carrying the image.
//
//	0 user      the prompt
//	1 system    <system-reminder>
//	2 assistant tool_use
//	3 user      tool_result + image   <- the turn in flight
//	4 system    <system-reminder>
func claudeCodeShapedBeta(b64 string) *anthropic.BetaMessageNewParams {
	sys := func(text string) anthropic.BetaMessageParam {
		return anthropic.BetaMessageParam{
			Role: anthropic.BetaMessageParamRoleSystem,
			Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: text}},
			},
		}
	}
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "what registration is on the aircraft?"}},
			}},
			sys("<system-reminder>context</system-reminder>"),
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{
				{OfToolUse: &anthropic.BetaToolUseBlockParam{ID: "toolu_1", Name: "Read"}},
			}},
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{
				{OfToolResult: &anthropic.BetaToolResultBlockParam{
					ToolUseID: "toolu_1",
					Content: []anthropic.BetaToolResultBlockParamContentUnion{
						{OfImage: &anthropic.BetaImageBlockParam{
							Source: anthropic.BetaImageBlockParamSourceUnion{
								OfBase64: &anthropic.BetaBase64ImageSourceParam{
									Data:      b64,
									MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
								},
							},
						}},
					},
				}},
			}},
			sys("<system-reminder>tokens left</system-reminder>"),
		},
	}
}

// TestVisionProxy_Beta_TrailingSystemMessage_StillDescribesCurrentTurn is the
// regression guard for the failure this fix addresses: Claude Code's image
// never reached any model because a trailing system message made the image
// test as history and it was replaced with the "omitted from history" marker.
func TestVisionProxy_Beta_TrailingSystemMessage_StillDescribesCurrentTurn(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("a V-tail aircraft, registration N40J")
	p := mkProcessor(t, fake, prov)

	req := claudeCodeShapedBeta(tinyPNGBase64)
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}

	require.NoError(t, p.Process(context.Background(), req, svcs, typ.SessionID{Value: "trailing-system-test"}))

	require.Equal(t, 1, fake.callCount(),
		"the image of the turn in flight must be described, not elided as history")
	require.Equal(t, 0, countImages(req))

	text := collectText(req)
	require.Contains(t, text, "registration N40J")
	require.NotContains(t, text, "omitted from history",
		"a trailing system message must not turn the current turn into history")
}

// TestVisionProxy_Beta_TrailingSystem_KeepsRealHistoryElided confirms the fix
// moves the anchor without disabling it: an image from an earlier user turn is
// still replaced with the cheap marker rather than sent upstream.
func TestVisionProxy_Beta_TrailingSystem_KeepsRealHistoryElided(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("latest description")
	p := mkProcessor(t, fake, prov)

	req := claudeCodeShapedBeta(tinyPNGBase64)
	// Splice a genuinely historical image in as the opening user turn.
	historical := anthropic.BetaMessageParam{
		Role: anthropic.BetaMessageParamRoleUser,
		Content: []anthropic.BetaContentBlockParamUnion{
			anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
				Data:      tinyPNGBase64,
				MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
			}),
		},
	}
	req.Messages = append([]anthropic.BetaMessageParam{historical}, req.Messages...)

	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	require.NoError(t, p.Process(context.Background(), req, svcs, typ.SessionID{Value: "trailing-system-test"}))

	require.Equal(t, 1, fake.callCount(), "only the current turn is described")
	require.Equal(t, 0, countImages(req))

	text := collectText(req)
	require.Contains(t, text, "latest description")
	require.Contains(t, text, "omitted from history", "the older image stays elided")
}

// TestVisionProxy_OpenAI_TrailingSystemMessage covers the same client shape on
// the Chat Completions ingress, where a trailing system message would
// otherwise push the user image out of the latest slot.
func TestVisionProxy_OpenAI_TrailingSystemMessage(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("chat latest desc")
	p := mkProcessor(t, fake, prov)

	req := &openai.ChatCompletionNewParams{
		Model: "gpt-4o-mini",
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
						{OfImageURL: &openai.ChatCompletionContentPartImageParam{
							ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
								URL: "data:" + tinyPNGMediaType + ";base64," + tinyPNGBase64,
							},
						}},
					},
				},
			}},
			{OfSystem: &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String("<system-reminder>tokens left</system-reminder>"),
				},
			}},
		},
	}

	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	require.NoError(t, p.Process(context.Background(), req, svcs, typ.SessionID{Value: "trailing-system-test"}))

	require.Equal(t, 1, fake.callCount(),
		"a trailing system message must not elide the user's image")
	require.Equal(t, 0, countImages(req))
	require.Contains(t, collectText(req), "chat latest desc")
}

// TestLatestImageAnchor_FallsBackWhenNothingQualifies pins the guard that keeps
// prior behaviour for message lists with no image-bearing role at all.
func TestLatestImageAnchor_FallsBackWhenNothingQualifies(t *testing.T) {
	require.Equal(t, 2, latestImageAnchor(3, func(int) bool { return false }),
		"no qualifying message keeps the anchor at len-1")
	require.Equal(t, -1, latestImageAnchor(0, func(int) bool { return false }),
		"an empty list yields the empty-slice anchor")
	require.Equal(t, 1, latestImageAnchor(4, func(i int) bool { return i == 1 }),
		"the last qualifying index wins over later non-qualifying ones")
}
