package request

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
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
func ConvertAnthropicV1ToBetaRequest(req *anthropic.MessageNewParams) *anthropic.BetaMessageNewParams {
	converted, err := ConvertAnthropicV1ToBetaRequestWithError(req)
	if err != nil {
		logrus.WithError(err).Warn("ConvertAnthropicV1ToBetaRequest: wire conversion failed")
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

// ConvertAnthropicBetaToV1Request is the provider-edge downgrade: the inverse
// projection for providers reached over the V1 wire. It is lossless for any
// request that originated as V1, so it refuses, rather than silently drops,
// what V1 cannot express: anthropic-beta header values and Beta-only
// top-level fields.
func ConvertAnthropicBetaToV1Request(req *anthropic.BetaMessageNewParams) (*anthropic.MessageNewParams, error) {
	if req == nil {
		return nil, nil
	}
	if len(req.Betas) > 0 {
		return nil, fmt.Errorf("downgrade Anthropic Beta request: anthropic-beta %v has no V1 form", req.Betas)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic Beta request: %w", err)
	}
	var v1 anthropic.MessageNewParams
	if err := json.Unmarshal(data, &v1); err != nil {
		return nil, fmt.Errorf("unmarshal Anthropic Beta request as v1: %w", err)
	}
	v1Data, err := json.Marshal(&v1)
	if err != nil {
		return nil, fmt.Errorf("marshal Anthropic v1 request: %w", err)
	}
	if dropped := droppedTopLevelFields(data, v1Data); len(dropped) > 0 {
		return nil, fmt.Errorf("downgrade Anthropic Beta request: Beta-only fields %v have no V1 form", dropped)
	}
	return &v1, nil
}

func droppedTopLevelFields(from, to []byte) []string {
	kept := map[string]bool{}
	gjson.ParseBytes(to).ForEach(func(key, _ gjson.Result) bool {
		kept[key.String()] = true
		return true
	})
	var dropped []string
	gjson.ParseBytes(from).ForEach(func(key, _ gjson.Result) bool {
		if !kept[key.String()] {
			dropped = append(dropped, key.String())
		}
		return true
	})
	return dropped
}
