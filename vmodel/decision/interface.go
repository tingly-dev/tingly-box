// Package decision holds virtual models for the decisions endpoint
// (POST {base}/decisions, see .design/decision-protocol.md): typed questions
// about a piece of state in, calibrated answers out.
//
// The wire shape follows Jev, the only published decisions API (OpenAI's is
// unpublished): a request carries "model", "state" and "questions" keyed by
// caller-chosen ids; each question has a "type" and "criteria":
//
//	choice  criteria: {"option": "description", ...}   -> choice + probabilities + confidence
//	score   criteria: ["lowest level", ..., "highest"]  -> score + legend + probabilities + confidence
//	noul    (instructions only)                         -> noul (probability the statement is true)
//
// The response is {"model", "answers": {id: {"type", ...}}, "usage"}.
package decision

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// VirtualModel is the decisions sub-interface: the base identity plus one
// request/response method over JSON.
type VirtualModel interface {
	vmodel.VirtualModel
	// HandleDecision answers one decisions request body (already known to be
	// valid JSON) with a JSON response body. An unanswerable request returns
	// *RequestError.
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
