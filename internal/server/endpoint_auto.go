package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// alternateOpenAIProtocol returns the other OpenAI protocol type.
func alternateOpenAIProtocol(current protocol.APIType) protocol.APIType {
	if current == protocol.TypeOpenAIResponses {
		return protocol.TypeOpenAIChat
	}
	return protocol.TypeOpenAIResponses
}

// incomingToTarget maps IncomingAPIType to protocol.APIType.
func incomingToTarget(incoming IncomingAPIType) protocol.APIType {
	if incoming == IncomingAPIResponses {
		return protocol.TypeOpenAIResponses
	}
	return protocol.TypeOpenAIChat
}

// extractLastGinError returns the most recent error recorded on the gin
// context via c.Error(). Returns nil when no errors exist.
func extractLastGinError(c *gin.Context) error {
	errs := c.Errors
	if len(errs) == 0 {
		return nil
	}
	return errs[len(errs)-1].Err
}

// clearGinErrors removes all errors from the gin context so that a
// fallback retry starts with a clean slate.
func clearGinErrors(c *gin.Context) {
	c.Errors = c.Errors[:0]
}

// overrideToTarget converts an EndpointOverride to a protocol.APIType.
func overrideToTarget(ov EndpointOverride) protocol.APIType {
	if ov == OverrideResponses {
		return protocol.TypeOpenAIResponses
	}
	return protocol.TypeOpenAIChat
}

// resolveAutoTarget handles the auto-mode target resolution shared by both
// OpenAI Chat and Responses handlers. It checks override → cache → default.
// Returns the resolved target and whether auto-fallback should be enabled.
func (s *Server) resolveAutoTarget(
	flags typ.RuleFlags, provider *typ.Provider, model string, incoming IncomingAPIType,
) (target protocol.APIType, autoFallback bool) {
	if ov := ParseEndpointOverride(flags.OpenAIEndpointOverride); ov == OverrideChat || ov == OverrideResponses {
		return overrideToTarget(ov), false
	}
	if cached, ok := s.endpointCache.Get(provider.UUID, model); ok {
		return cached, false
	}
	return incomingToTarget(incoming), true
}

// autoDispatchFn is the callback for dispatchWithAutoFallback.
// It performs transform + dispatch for a given target protocol, using
// the provided gate. Returns true if dispatch executed (even on error),
// false if the transform itself failed.
type autoDispatchFn func(target protocol.APIType, gate *firstChunkGate) bool

// dispatchWithAutoFallback wraps a dispatch attempt with protocol
// auto-detection. It tries the preferred target first; on retryable
// failure it falls back to the alternate protocol. Successful protocol
// choices are cached per provider+model.
func (s *Server) dispatchWithAutoFallback(
	c *gin.Context,
	provider *typ.Provider,
	model string,
	preferredTarget protocol.APIType,
	dispatch autoDispatchFn,
) {
	realWriter := c.Writer
	gate := newFirstChunkGate(realWriter)
	c.Writer = gate
	defer func() {
		c.Writer = realWriter
		gate.CommitIfBuffered()
	}()

	// First attempt with preferred protocol
	dispatch(preferredTarget, gate)

	if gate.Committed() || (gate.Status() > 0 && gate.Status() < http.StatusBadRequest) {
		s.endpointCache.Set(provider.UUID, model, preferredTarget)
		return
	}

	// Check if fallback is worthwhile
	if !isRetryableStatus(gate.Status()) {
		return
	}
	lastErr := extractLastGinError(c)
	if client.IsNonRetryableForProtocolSwitch(lastErr) {
		return
	}

	// Fallback to alternate protocol
	altTarget := alternateOpenAIProtocol(preferredTarget)
	logrus.WithContext(c.Request.Context()).Infof(
		"[auto-endpoint] %s:%s status=%d → fallback from %s to %s",
		provider.UUID, model, gate.Status(), preferredTarget, altTarget,
	)
	gate.Discard()
	clearGinErrors(c)

	dispatch(altTarget, gate)

	if gate.Committed() || (gate.Status() > 0 && gate.Status() < http.StatusBadRequest) {
		s.endpointCache.Set(provider.UUID, model, altTarget)
	}
}
