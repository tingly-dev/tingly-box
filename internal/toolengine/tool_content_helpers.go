package toolengine

import (
	"github.com/anthropics/anthropic-sdk-go"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

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
