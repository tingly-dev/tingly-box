package server

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// upstreamForwardStatus returns the status code to send to the client when a
// non-streaming forward fails. It propagates the upstream provider's HTTP status
// when the error carries one (so a 401/429/4xx is not flattened into a 500) and
// defaults to 500 Internal Server Error otherwise.
func upstreamForwardStatus(err error) int {
	return protocol.UpstreamStatus(err, http.StatusInternalServerError)
}
