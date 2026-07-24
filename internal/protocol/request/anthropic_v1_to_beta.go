package request

import (
	"encoding/json"

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
// Returns nil (rather than erroring) if req is nil or the round-trip fails,
// so callers doing best-effort context extraction (smart-routing) degrade
// gracefully instead of panicking.
func ConvertAnthropicV1ToBetaRequest(req *anthropic.MessageNewParams) *anthropic.BetaMessageNewParams {
	if req == nil {
		return nil
	}

	b, err := json.Marshal(req)
	if err != nil {
		logrus.WithError(err).Warn("ConvertAnthropicV1ToBetaRequest: marshal v1 request failed")
		return nil
	}

	var beta anthropic.BetaMessageNewParams
	if err := json.Unmarshal(b, &beta); err != nil {
		logrus.WithError(err).Warn("ConvertAnthropicV1ToBetaRequest: unmarshal into beta shape failed")
		return nil
	}
	return &beta
}
