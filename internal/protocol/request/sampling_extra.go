package request

import (
	"github.com/openai/openai-go/v3/packages/param"
)

// The anthropic-sdk-go v2 MessageNewParams / BetaMessageNewParams no longer
// carry temperature / top_p / top_k as typed fields. We pin API version
// 2023-06-01, whose wire format still accepts them, so they travel through
// the SDK's extra-fields mechanism instead. Decoded inbound requests surface
// them in ExtraFields() as float64.

// extrasOpt reads a float64 value out of an extras map as param.Opt[float64].
func extrasOpt(extras map[string]any, key string) param.Opt[float64] {
	if v, ok := extras[key]; ok {
		if f, ok := v.(float64); ok {
			return param.NewOpt(f)
		}
	}
	return param.Opt[float64]{}
}

// SamplingTemperature returns the request's temperature, if present.
func SamplingTemperature(req interface{ ExtraFields() map[string]any }) param.Opt[float64] {
	return extrasOpt(req.ExtraFields(), "temperature")
}

// SamplingTopP returns the request's top_p, if present.
func SamplingTopP(req interface{ ExtraFields() map[string]any }) param.Opt[float64] {
	return extrasOpt(req.ExtraFields(), "top_p")
}

// SetSamplingExtras writes temperature / top_p onto an anthropic request via
// the extras mechanism. Existing extras are preserved.
func SetSamplingExtras(req interface {
	ExtraFields() map[string]any
	SetExtraFields(map[string]any)
}, temp, topP param.Opt[float64]) {
	m := req.ExtraFields()
	if m == nil {
		m = map[string]any{}
	}
	if !param.IsOmitted(temp) {
		m["temperature"] = temp.Value
	}
	if !param.IsOmitted(topP) {
		m["top_p"] = topP.Value
	}
	req.SetExtraFields(m)
}
