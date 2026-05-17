package server

import (
	"github.com/tingly-dev/tingly-box/internal/probe"
)

// ProbeV2Response represents a Probe V2 response. The wrapper stays in the
// server package because it embeds *ErrorDetail; the request and data
// shapes live in internal/probe.
type ProbeV2Response struct {
	Success bool               `json:"success"`
	Error   *ErrorDetail       `json:"error,omitempty"`
	Data    *probe.ProbeV2Data `json:"data,omitempty"`
}
