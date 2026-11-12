package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go/v2"
)

// VModelResponse describes what the virtual model wants to respond.
type VModelResponse struct {
	Content    []sdk.BetaContentBlockParamUnion
	StopReason sdk.BetaStopReason
}
