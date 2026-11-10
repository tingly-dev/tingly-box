package protocolserver

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/recording"
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

// writeErrorResponse retains validation/dispatch failures in the access log as
// well as returning the existing wire response to the caller.
func writeErrorResponse(c *gin.Context, status int, response ErrorResponse) {
	c.Error(fmt.Errorf("%s: %s", response.Error.Type, response.Error.Message)).SetType(gin.ErrorTypePublic)
	c.JSON(status, response)
}

// ProbeSyntheticRuleUUID marks the throwaway rule built for an
// X-Tingly-Probe-Service request — it has no persisted identity. Owned here
// (moved from root's handlers.go constant) since protocol_dispatch.go's
// setProbeUpstreamHeaders is the sole consumer that has moved so far; root's
// handlers.go (not yet moved) keeps a companion alias.
const ProbeSyntheticRuleUUID = "probe-synthetic"

// failRequest reports a failed upstream call on a non-streaming (or
// pre-stream) path: zero-usage error tracking, a SendErrorResponse with the
// given description (propagating the upstream status), and recorder capture.
// It consolidates the track/send/record triplet repeated across the protocol
// dispatch paths.
func (ph *ProtocolHandler) failRequest(c *gin.Context, err error, desc string) {
	recorder := recording.FromGin(c)
	ph.trackUsageFromContext(c, 0, 0, err)
	SendErrorResponse(c, err, desc)
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// failForward is failRequest with the canonical forwarding-error body
// (stream.SendForwardingError) instead of a per-site description.
func (ph *ProtocolHandler) failForward(c *gin.Context, err error) {
	recorder := recording.FromGin(c)
	ph.trackUsageFromContext(c, 0, 0, err)
	stream.SendForwardingError(c, err)
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// contentPolicyBlockResponse is the client body for a content-policy block:
// the allowlisted fields plus the upstream's misalignment details, which
// Codex reads from an HTTP 400 body for misalignment_policy_violation.
type contentPolicyBlockResponse struct {
	Error contentPolicyBlockDetail `json:"error"`
}

type contentPolicyBlockDetail struct {
	protocol.ErrorDetail
	Misalignment json.RawMessage `json:"misalignment,omitempty"`
}

// failContentPolicyBlock answers a Responses request whose upstream blocked
// the content on policy grounds before anything reached the client (#2417).
// 400 is not retried by SDKs or Codex, and is outside
// retryableUpstreamStatuses, so a multi-service rule does not re-send the
// same content to another account. x-should-retry: false is the Stainless
// SDKs' explicit no-retry signal (shouldRetry in openai-go and
// anthropic-sdk-go). Codex maps error.code to its policy handling.
func (ph *ProtocolHandler) failContentPolicyBlock(c *gin.Context, err error, block protocol.ContentPolicyBlock) {
	recorder := recording.FromGin(c)
	ph.trackUsageFromContext(c, 0, 0, &protocol.ContentPolicyBlockError{Code: block.Code, Err: err})
	c.Error(protocol.LoggableUpstreamError(err)).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.Header("x-should-retry", "false")
	c.JSON(http.StatusBadRequest, contentPolicyBlockResponse{Error: contentPolicyBlockDetail{
		ErrorDetail: protocol.ErrorDetail{
			Message: block.Message,
			Type:    cmp.Or(block.Type, "invalid_request_error"),
			Code:    block.Code,
		},
		Misalignment: block.Misalignment,
	}})
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// SendErrorResponse registers the error into gin context for logging middleware and sends JSON response.
func SendErrorResponse(c *gin.Context, err error, desc string) {

	// Classifies err once for both the status (propagates the upstream
	// provider's HTTP status when the error carries one, so a 401/429/4xx is
	// not flattened into a 500) and the message.
	failure := protocol.ClassifyUpstreamFailure(err, http.StatusInternalServerError)

	// c.Error keeps the full raw error for the server-side access log
	// (internal/middleware/memory_log.go); the JSON body below shows the
	// client a categorized, non-leaky message instead.
	asErr := fmt.Errorf("%w: %s", protocol.LoggableUpstreamError(err), desc)
	c.Error(asErr).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(failure.Status, ErrorResponse{
		Error: ErrorDetail{
			Message: fmt.Sprintf("%s: %s", failure.Message, desc),
			Type:    "protocol_error",
			Code:    desc,
		},
	})
}
