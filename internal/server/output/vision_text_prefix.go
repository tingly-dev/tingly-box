// Package output holds the business-layer implementations of
// protocol/outputinjector.OutputInjector. Each implementation here owns a
// specific business reason to mutate the model's output; the protocol layer
// handles the protocol-specific where-and-how.
package output

import (
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol/outputinjector"
)

// VisionTextPrefix prepends a vision-proxy description summary to the first
// text-bearing slot of the model's response (stream or non-stream). It is the
// first concrete OutputInjector and the only one needed for the vision
// proxy: the descriptions array is populated during request-side processing
// (see internal/server/processor.VisionProxyProcessor) and stashed on the
// gin.Context for the handler to pick up and attach here.
//
// Single-call-per-request semantics: PrefixText returns the formatted
// prefix once and then "" thereafter. Implementations of OutputInjector
// only fire on the first text-bearing event for that reason — once consumed,
// later events pass through unchanged.
type VisionTextPrefix struct {
	descs    []string
	injected bool // single-request, sequential consumption -> no mutex needed
}

// NewVisionTextPrefix returns an injector that prepends a summary of the
// given descriptions. Passing nil or an empty slice yields an injector that
// is always a no-op (PrefixText returns "" from the start), so callers can
// unconditionally Attach the result.
func NewVisionTextPrefix(descs []string) *VisionTextPrefix {
	return &VisionTextPrefix{descs: descs}
}

// PrefixText implements outputinjector.OutputInjector.
//
// Format: "[Vision: a; b; c]\n\n" for three descriptions; single description
// drops the separator. The `[Vision: ...]` framing signals to the user that
// the text is not a model utterance, and the trailing blank line separates
// it from the model's real first words.
func (v *VisionTextPrefix) PrefixText() string {
	if v == nil || v.injected || len(v.descs) == 0 {
		return ""
	}
	v.injected = true
	return "[Vision: " + strings.Join(v.descs, "; ") + "]\n\n"
}

// Compile-time check.
var _ outputinjector.OutputInjector = (*VisionTextPrefix)(nil)
