package request

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

// ConvertAnthropicV1ToBetaRequest projects an Anthropic v1 MessageNewParams onto
// the Beta MessageNewParams shape. Beta's wire format is a strict structural
// superset of v1's (same field names and shapes; Beta only adds optional
// fields), so a v1 request's own JSON round-trips losslessly into the Beta
// struct — every block type (text, image, tool_use, tool_result, documents,
// cache_control, ...) survives without hand-maintained per-type conversion.
// This previously did a field-by-field Go copy that silently dropped
// tool_result content and image data; the round-trip has no such gaps and
// needs no updates when the SDK adds new block types.
//
// This compatibility wrapper keeps the historical nil-on-failure behavior for
// context-extraction callers. Protocol boundaries that need an actionable
// error should use ConvertAnthropicV1ToBetaRequestWithError.
func ConvertAnthropicV1ToBetaRequest(ctx context.Context, req *anthropic.MessageNewParams) *anthropic.BetaMessageNewParams {
	converted, err := ConvertAnthropicV1ToBetaRequestWithError(req)
	if err != nil {
		logrus.WithContext(ctx).WithError(err).Warn("ConvertAnthropicV1ToBetaRequest: wire conversion failed")
		return nil
	}
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
