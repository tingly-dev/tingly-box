// Package probe is the HTTP service layer for the probe subsystem. It wraps
// internal/probe.V2Service in a gin handler and owns the response envelope
// shape for the V2 endpoint.
package probe

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/probe"
)

// Handler exposes the probe HTTP endpoints. It carries only the V2 service
// reference today; lightweight and adaptive handlers can be hung off the
// same struct when those strategies are decoupled from *Server.
type Handler struct {
	v2 *probe.V2Service
}

// NewHandler builds a Handler around the given V2 service.
func NewHandler(v2 *probe.V2Service) *Handler {
	return &Handler{v2: v2}
}

// errorDetail mirrors the JSON shape of the server's global ErrorDetail so
// the API contract is unchanged. Defined locally to keep this package free
// of any internal/server import.
type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// V2Response is the JSON envelope returned by POST /probe.
type V2Response struct {
	Success bool               `json:"success"`
	Error   *errorDetail       `json:"error,omitempty"`
	Data    *probe.ProbeV2Data `json:"data,omitempty"`
}

// HandleProbeV2 handles Probe V2 requests (unified endpoint for rules,
// saved providers, and unsaved provider configs).
func (h *Handler) HandleProbeV2(c *gin.Context) {
	var req probe.ProbeV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, V2Response{
			Success: false,
			Error: &errorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if err := probe.ValidateProbeV2Request(&req); err != nil {
		c.JSON(http.StatusBadRequest, V2Response{
			Success: false,
			Error: &errorDetail{
				Message: err.Error(),
				Type:    "validation_error",
			},
		})
		return
	}

	ctx := c.Request.Context()
	startTime := time.Now()

	var (
		data *probe.ProbeV2Data
		err  error
	)
	switch req.TestMode {
	case probe.ProbeV2ModeSimple:
		data, err = h.v2.Probe(ctx, &req)
	case probe.ProbeV2ModeStreaming, probe.ProbeV2ModeTool:
		data, err = h.v2.ProbeStream(ctx, &req)
	}

	if err != nil {
		c.JSON(http.StatusOK, V2Response{
			Success: false,
			Error: &errorDetail{
				Message: err.Error(),
				Type:    "probe_error",
			},
		})
		return
	}

	data.LatencyMs = time.Since(startTime).Milliseconds()
	c.JSON(http.StatusOK, V2Response{Success: true, Data: data})
}
