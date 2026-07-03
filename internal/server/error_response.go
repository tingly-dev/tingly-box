package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// ProbeSyntheticRuleUUID marks the throwaway rule built for an
// X-Tingly-Probe-Service request — it has no persisted identity. Owned here
// (moved from root's handlers.go constant) since protocol_dispatch.go's
// setProbeUpstreamHeaders is the sole consumer that has moved so far; root's
// handlers.go (not yet moved) keeps a companion alias.
const ProbeSyntheticRuleUUID = "probe-synthetic"

// SendErrorResponse registers the error into gin context for logging middleware and sends JSON response.
func SendErrorResponse(c *gin.Context, err error, desc string) {

	// upstreamForwardStatus returns the status code to send to the client when a
	// non-streaming forward fails. It propagates the upstream provider's HTTP status
	// when the error carries one (so a 401/429/4xx is not flattened into a 500) and
	// defaults to 500 Internal Server Error otherwise.
	statusCode := protocol.UpstreamStatus(err, http.StatusInternalServerError)

	asErr := fmt.Errorf("%s: %s", err.Error(), desc)
	c.Error(asErr).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(statusCode, ErrorResponse{
		Error: ErrorDetail{
			Message: asErr.Error(),
			Type:    "protocol_error",
			Code:    desc,
		},
	})
}
