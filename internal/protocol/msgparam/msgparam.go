// Package msgparam provides v1-style role/content accessors over the
// anthropic-sdk-go v2 role-union message params (MessageParam and
// BetaMessageParam). The v2 SDK split MessageParam into
// UserMessageParam/AssistantMessageParam/SystemMessageParam unions and
// dropped the Role/Content fields; the protocol pipeline still reasons
// about messages generically, so it goes through these helpers.
package msgparam

import (
	"github.com/anthropics/anthropic-sdk-go/v2"
)

// New builds a MessageParam for the given role ("user", "assistant" or
// "system") with the given content blocks. An unknown role yields a user
// message.
func New(role string, content []anthropic.ContentBlockParamUnion) anthropic.MessageParam {
	switch role {
	case "assistant":
		return anthropic.NewAssistantMessage(content...)
	case "system":
		blocks := make([]anthropic.TextBlockParam, 0, len(content))
		for _, block := range content {
			if block.OfText != nil {
				blocks = append(blocks, *block.OfText)
			}
		}
		return anthropic.MessageParam{OfSystem: &anthropic.SystemMessageParam{Content: blocks}}
	default:
		return anthropic.NewUserMessage(content...)
	}
}

// Role returns the message's role as the classic wire string
// ("user", "assistant" or "system").
func Role(m anthropic.MessageParam) string {
	switch {
	case m.OfAssistant != nil:
		return "assistant"
	case m.OfSystem != nil:
		return "system"
	default:
		return "user"
	}
}

// Content returns the message's content blocks regardless of role.
func Content(m anthropic.MessageParam) []anthropic.ContentBlockParamUnion {
	switch {
	case m.OfUser != nil:
		return m.OfUser.Content
	case m.OfAssistant != nil:
		return m.OfAssistant.Content
	case m.OfSystem != nil:
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.OfSystem.Content))
		for i := range m.OfSystem.Content {
			// Keep mutations (for example credential masking) attached to the
			// original message, just as for user and assistant text blocks.
			blocks = append(blocks, anthropic.ContentBlockParamUnion{OfText: &m.OfSystem.Content[i]})
		}
		return blocks
	default:
		return nil
	}
}

// NewBeta is New for BetaMessageParam. Beta system messages carry a
// different content union (text / tool-addition / tool-removal only), so a
// "system" role keeps just the text blocks.
func NewBeta(role string, content []anthropic.BetaContentBlockParamUnion) anthropic.BetaMessageParam {
	switch role {
	case "assistant":
		return anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{Content: content}}
	case "system":
		blocks := make([]anthropic.BetaSystemMessageParamContentUnion, 0, len(content))
		for _, block := range content {
			if block.OfText != nil {
				blocks = append(blocks, anthropic.BetaSystemMessageParamContentUnion{OfText: block.OfText})
			}
		}
		return anthropic.NewBetaSystemMessage(anthropic.BetaSystemMessageOutputConfigParam{}, blocks...)
	default:
		return anthropic.NewBetaUserMessage(content...)
	}
}

// WithContent returns m with its content blocks replaced (user and
// assistant messages only; system messages keep their content).
func WithContent(m anthropic.MessageParam, content []anthropic.ContentBlockParamUnion) anthropic.MessageParam {
	switch {
	case m.OfUser != nil:
		m.OfUser.Content = content
	case m.OfAssistant != nil:
		m.OfAssistant.Content = content
	}
	return m
}

// WithBetaContent returns m with its content blocks replaced (assistant and
// user messages only; system messages keep their content).
func WithBetaContent(m anthropic.BetaMessageParam, content []anthropic.BetaContentBlockParamUnion) anthropic.BetaMessageParam {
	switch {
	case m.OfAssistant != nil:
		m.OfAssistant.Content = content
	case m.OfUser != nil:
		m.OfUser.Content = content
	}
	return m
}

// BetaRole is Role for BetaMessageParam.
func BetaRole(m anthropic.BetaMessageParam) string {
	switch {
	case m.OfAssistant != nil:
		return "assistant"
	case m.OfSystem != nil:
		return "system"
	default:
		return "user"
	}
}

// BetaContent is Content for BetaMessageParam. System messages' tool
// addition/removal blocks have no BetaContentBlockParamUnion
// representation and are returned as text blocks only.
func BetaContent(m anthropic.BetaMessageParam) []anthropic.BetaContentBlockParamUnion {
	switch {
	case m.OfUser != nil:
		return m.OfUser.Content
	case m.OfAssistant != nil:
		return m.OfAssistant.Content
	case m.OfSystem != nil:
		blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(m.OfSystem.Content))
		for _, block := range m.OfSystem.Content {
			if block.OfText != nil {
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{OfText: block.OfText})
			}
		}
		return blocks
	default:
		return nil
	}
}
