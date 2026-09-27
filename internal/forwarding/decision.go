package forwarding

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/client"
)

// DecisionUpstreamError reports an upstream decision call that returned a
// non-2xx HTTP response. It carries the status so the handler can propagate
// the real status code, and the body so the caller sees the provider's own
// error JSON instead of a gateway-rewritten envelope.
type DecisionUpstreamError struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func (e *DecisionUpstreamError) Error() string {
	return fmt.Sprintf("decision upstream returned status %d", e.StatusCode)
}

// DecisionEndpointURL normalizes a decision endpoint base to the native
// decisions URL; re-exported from the client package where the decision
// client lives.
func DecisionEndpointURL(apiBase string) (string, error) {
	return client.DecisionEndpointURL(apiBase)
}

// ForwardDecision sends one native decision request through the dedicated
// decision client. body must already carry the routed (service) model name —
// decision has no transform chain, the handler rewrites the model in the
// opaque request map before calling this.
//
// There is deliberately no failover above this call: decision is a single-shot
// micro-model invocation, not a retryable stream.
func ForwardDecision(fc *ForwardContext, wrapper client.DecisionClientInterface, body []byte) (*client.DecisionHTTPResult, context.CancelFunc, error) {
	if wrapper == nil {
		return nil, func() {}, fmt.Errorf("failed to get decision client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(body)

	result, err := wrapper.Decisions(ctx, body)
	fc.Complete(ctx, result, err)
	if err != nil {
		return nil, cancel, err
	}

	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return result, cancel, &DecisionUpstreamError{
			StatusCode:  result.StatusCode,
			ContentType: result.ContentType,
			Body:        result.Body,
		}
	}
	return result, cancel, nil
}
