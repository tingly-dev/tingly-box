package request

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// ConvertAnthropicV1ToBetaRequest projects an Anthropic v1 request onto the
// Beta request type. Anthropic v1 is a structural subset of Beta, so the wire
// representation is the conversion contract and preserves every v1 field.
//
// This compatibility wrapper keeps the historical nil-on-failure behavior for
// context-extraction callers. Protocol boundaries that need an actionable
// error should use ConvertAnthropicV1ToBetaRequestWithError.
func ConvertAnthropicV1ToBetaRequest(req *anthropic.MessageNewParams) *anthropic.BetaMessageNewParams {
	converted, _ := ConvertAnthropicV1ToBetaRequestWithError(req)
	return converted
}

// ConvertAnthropicV1ToBetaRequestWithError performs the same wire conversion
// and reports malformed or non-JSON parameter values to the caller.
func ConvertAnthropicV1ToBetaRequestWithError(req *anthropic.MessageNewParams) (*anthropic.BetaMessageNewParams, error) {
	if req == nil {
		return nil, nil
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic v1 request: %w", err)
	}
	var beta anthropic.BetaMessageNewParams
	if err := json.Unmarshal(data, &beta); err != nil {
		return nil, fmt.Errorf("unmarshal Anthropic v1 request as Beta: %w", err)
	}
	return &beta, nil
}
