package token

import (
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tiktoken-go/tokenizer"
)

// EstimateInputTokens estimates input tokens from OpenAI request using tiktoken
func EstimateInputTokens(req *openai.ChatCompletionNewParams) (int, error) {
	enc, err := getCodec(tokenizer.O200kBase)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	// Helper function to count tokens with fallback to character/4 estimate
	countOrEstimate := func(text string) int {
		c, err := enc.Count(text)
		if err != nil {
			return len(text) / 4
		}
		return c
	}

	return estimateInputTokensWith(req, countOrEstimate), nil
}

// EstimateInputTokensSimple is a cheap len/4 approximation of EstimateInputTokens
// (no tiktoken), for the streaming hot path where the caller pre-computes the
// fallback estimate up front instead of calling the exact estimator per stream.
func EstimateInputTokensSimple(req *openai.ChatCompletionNewParams) int {
	return estimateInputTokensWith(req, func(text string) int { return len(text) / 4 })
}

// estimateInputTokensWith walks the request's billable text once, counting each
// piece with the supplied strategy. Shared by the exact and approximate variants.
func estimateInputTokensWith(req *openai.ChatCompletionNewParams, countOrEstimate func(string) int) int {
	totalTokens := 0

	// Count tokens in messages
	for _, msg := range req.Messages {
		// Count role
		if role := msg.GetRole(); role != nil {
			totalTokens += countOrEstimate(*role)
		}

		// Count content
		content := msg.GetContent()
		switch content.AsAny().(type) {
		case *string:
			if s := content.AsAny().(*string); s != nil {
				totalTokens += countOrEstimate(*s)
			}
		case *[]openai.ChatCompletionContentPartTextParam:
			if parts := content.AsAny().(*[]openai.ChatCompletionContentPartTextParam); parts != nil {
				for _, part := range *parts {
					totalTokens += countOrEstimate(part.Text)
				}
			}
		case *[]openai.ChatCompletionContentPartUnionParam:
			if parts := content.AsAny().(*[]openai.ChatCompletionContentPartUnionParam); parts != nil {
				for _, part := range *parts {
					if part.OfText != nil {
						totalTokens += countOrEstimate(part.OfText.Text)
					}
				}
			}
		}
	}

	// Count tokens in tools
	for _, tool := range req.Tools {
		toolJSON, err := json.Marshal(tool)
		if err != nil {
			// If serialization fails, estimate based on tool components
			if tool.OfFunction != nil {
				totalTokens += countOrEstimate(string(tool.OfFunction.Type))
				totalTokens += countOrEstimate(tool.OfFunction.Function.Name)
				if tool.OfFunction.Function.Description.Valid() {
					totalTokens += countOrEstimate(tool.OfFunction.Function.Description.Value)
				}
			}
			if tool.OfCustom != nil {
				totalTokens += countOrEstimate(string(tool.OfCustom.Type))
			}
		} else {
			totalTokens += countOrEstimate(string(toolJSON))
		}
	}

	// Add some overhead for the request format
	totalTokens += 3

	return totalTokens
}

func EstimateMessagesTokens(messages []openai.ChatCompletionMessageParamUnion) int64 {
	var total int64 = 0
	for _, msg := range messages {
		total += 5
		total += EstimateMessageTokens(msg)
	}
	return total
}

func EstimateMessageTokens(msg openai.ChatCompletionMessageParamUnion) int64 {
	if msg.OfUser != nil {
		c := msg.OfUser.Content
		if !param.IsOmitted(c.OfString) {
			return EstimateTokensString(c.OfString.Value)
		}
	}
	if msg.OfAssistant != nil {
		c := msg.OfAssistant.Content
		if !param.IsOmitted(c.OfString) {
			return EstimateTokensString(c.OfString.Value)
		}
	}
	if msg.OfSystem != nil {
		c := msg.OfSystem.Content
		if !param.IsOmitted(c.OfString) {
			return EstimateTokensString(c.OfString.Value)
		}
	}
	if msg.OfDeveloper != nil {
		c := msg.OfDeveloper.Content
		if !param.IsOmitted(c.OfString) {
			return EstimateTokensString(c.OfString.Value)
		}
	}
	return 0
}

// EstimateOutputTokens estimates output tokens from accumulated content
func EstimateOutputTokens(content string) int {
	enc, err := getCodec(tokenizer.O200kBase)
	if err != nil {
		return len(content) / 4
	}
	count, err := enc.Count(content)
	if err != nil {
		return len(content) / 4
	}
	return count
}
