package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// buildAnthropicPreChain constructs the pre-request transform chain for Anthropic V1 and Beta handlers.
// Each transform is only added when its corresponding flag is active, so transforms
// never need to re-check the same condition inside Apply().
func buildAnthropicPreChain(
	scenarioConfig *typ.ScenarioConfig,
	defaultMaxTokens, maxAllowed int,
) []transform.Transform {
	var chain []transform.Transform

	if scenarioConfig != nil {
		flags := &scenarioConfig.Flags
		if flags.ThinkingMode != "" {
			chain = append(chain, NewThinkingModeTransform(scenarioConfig))
		}
		if flags.CleanHeader {
			chain = append(chain, NewCleanHeaderTransform())
		}
	}

	chain = append(chain, NewMaxTokensTransform(defaultMaxTokens, maxAllowed))
	return chain
}

// scenarioFlagsOrNil returns the scenario flags or nil.
func scenarioFlagsOrNil(scenarioConfig *typ.ScenarioConfig) *typ.ScenarioFlags {
	if scenarioConfig != nil {
		return &scenarioConfig.Flags
	}
	return nil
}

// executeAnthropicV1PreChain builds and runs the pre-transform chain for Anthropic V1 requests.
// Returns an error that should be mapped to HTTP 400.
func executeAnthropicV1PreChain(
	req *anthropic.MessageNewParams,
	scenarioConfig *typ.ScenarioConfig,
	defaultMaxTokens, maxAllowed int,
	isStreaming bool,
) error {
	transforms := buildAnthropicPreChain(scenarioConfig, defaultMaxTokens, maxAllowed)
	ctx := transform.NewTransformContext(
		req,
		transform.WithScenarioFlags(scenarioFlagsOrNil(scenarioConfig)),
		transform.WithStreaming(isStreaming),
	)
	if len(transforms) == 0 {
		return nil
	}
	_, err := transform.NewTransformChain(transforms).Execute(ctx)
	return err
}

// executeAnthropicBetaPreChain builds and runs the pre-transform chain for Anthropic Beta requests.
// Returns an error that should be mapped to HTTP 400.
func executeAnthropicBetaPreChain(
	req *anthropic.BetaMessageNewParams,
	scenarioConfig *typ.ScenarioConfig,
	defaultMaxTokens, maxAllowed int,
	isStreaming bool,
) error {
	transforms := buildAnthropicPreChain(scenarioConfig, defaultMaxTokens, maxAllowed)
	ctx := transform.NewTransformContext(
		req,
		transform.WithScenarioFlags(scenarioFlagsOrNil(scenarioConfig)),
		transform.WithStreaming(isStreaming),
	)
	if len(transforms) == 0 {
		return nil
	}
	_, err := transform.NewTransformChain(transforms).Execute(ctx)
	return err
}
