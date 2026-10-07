// Package decision holds virtual models for the decisions endpoint
// (POST {base}/decisions, see .design/decision-protocol.md): given options the
// caller defines, answer with one of them.
//
// The upstream schema is not public, so the request body is treated as opaque
// JSON and two shapes are understood:
//
//	{"model": "...", "options": ["a","b"], ...}                    // single choice
//	{"model": "...", "questions": {"q1": {"options": [...]}}, ...} // Jev-style map
//
// Everything else in the body is ignored.
package decision

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// VirtualModel is the decisions sub-interface: the base identity plus one
// request/response method over opaque JSON.
type VirtualModel interface {
	vmodel.VirtualModel
	// HandleDecision answers one decisions request body with a JSON response
	// body. A malformed or unanswerable request returns *RequestError.
	HandleDecision(body []byte) ([]byte, error)
}

// Registry holds decision virtual models.
type Registry = vmodel.GenericRegistry[VirtualModel]

// NewRegistry creates an empty decision registry.
func NewRegistry() *Registry {
	return vmodel.NewGenericRegistry[VirtualModel]()
}

// RequestError marks a client-side problem (HTTP 400) as opposed to a model
// failure.
type RequestError struct{ Message string }

func (e *RequestError) Error() string { return e.Message }

func badRequest(format string, a ...any) error {
	return &RequestError{Message: fmt.Sprintf(format, a...)}
}
