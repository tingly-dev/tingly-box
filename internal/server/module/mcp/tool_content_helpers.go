package mcp

import (
	"github.com/anthropics/anthropic-sdk-go"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// ToolContentsToAnthropicV1 converts []ToolContent to the Anthropic V1 tool result content slice.
// Text items become TextBlockParam; image items become ImageBlockParam.
func ToolContentsToAnthropicV1(contents []coretool.ToolContent) []anthropic.ToolResultBlockParamContentUnion {
	return toolContentsToAnthropicV1(contents)
}

// ToolContentsToAnthropicBeta converts []ToolContent to the Anthropic Beta tool result content slice.
func ToolContentsToAnthropicBeta(contents []coretool.ToolContent) []anthropic.BetaToolResultBlockParamContentUnion {
	return toolContentsToAnthropicBeta(contents)
}

// toolContentsToAnthropicV1 converts []ToolContent to the Anthropic V1 tool result content slice.
// Text items become TextBlockParam; image items become ImageBlockParam.
func toolContentsToAnthropicV1(contents []coretool.ToolContent) []anthropic.ToolResultBlockParamContentUnion {
	out := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(contents))
	for _, c := range contents {
		switch c.Type {
		case coretool.ContentTypeImage:
			out = append(out, anthropic.ToolResultBlockParamContentUnion{
				OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							MediaType: anthropic.Base64ImageSourceMediaType(c.MIMEType),
							Data:      c.Data,
						},
					},
				},
			})
		default:
			out = append(out, anthropic.ToolResultBlockParamContentUnion{
				OfText: &anthropic.TextBlockParam{Text: c.Text},
			})
		}
	}
	return out
}

// toolContentsToAnthropicBeta converts []ToolContent to the Anthropic Beta tool result content slice.
func toolContentsToAnthropicBeta(contents []coretool.ToolContent) []anthropic.BetaToolResultBlockParamContentUnion {
	out := make([]anthropic.BetaToolResultBlockParamContentUnion, 0, len(contents))
	for _, c := range contents {
		switch c.Type {
		case coretool.ContentTypeImage:
			out = append(out, anthropic.BetaToolResultBlockParamContentUnion{
				OfImage: &anthropic.BetaImageBlockParam{
					Source: anthropic.BetaImageBlockParamSourceUnion{
						OfBase64: &anthropic.BetaBase64ImageSourceParam{
							MediaType: anthropic.BetaBase64ImageSourceMediaType(c.MIMEType),
							Data:      c.Data,
						},
					},
				},
			})
		default:
			out = append(out, anthropic.BetaToolResultBlockParamContentUnion{
				OfText: &anthropic.BetaTextBlockParam{Text: c.Text},
			})
		}
	}
	return out
}
