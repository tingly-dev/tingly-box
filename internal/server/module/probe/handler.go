package probe

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler exposes the probe HTTP endpoints. It carries the V2 and
// lightweight services; adaptive can be hung off the same struct when that
// strategy is decoupled from *Server.
type Handler struct {
	v2          *probe.V2Service
	lightweight *probe.LightweightService
}

// NewHandler builds a Handler around the given probe services.
func NewHandler(v2 *probe.V2Service, lightweight *probe.LightweightService) *Handler {
	return &Handler{v2: v2, lightweight: lightweight}
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

// LightweightResponse is the JSON envelope returned by POST /probe/lightweight.
type LightweightResponse struct {
	Success bool                                `json:"success"`
	Error   *errorDetail                        `json:"error,omitempty"`
	Data    *probe.LightweightProbeResponseData `json:"data,omitempty"`
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

// HandleLightweightProbe handles the optional "Test Connection" probe used
// when adding API keys. Always returns 200 with success=true once a request
// passes validation — per-endpoint results in Data are informational only
// and never block onboarding.
func (h *Handler) HandleLightweightProbe(c *gin.Context) {
	var req probe.LightweightProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LightweightResponse{
			Success: false,
			Error: &errorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if req.APIBase == "" || req.APIStyle == "" || req.Token == "" {
		c.JSON(http.StatusBadRequest, LightweightResponse{
			Success: false,
			Error: &errorDetail{
				Message: "api_base, api_style, and token are required",
				Type:    "validation_error",
			},
		})
		return
	}

	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: protocol.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}
	if req.AuthType != "" {
		provider.AuthType = typ.AuthType(req.AuthType)
	}

	data := h.lightweight.Probe(c.Request.Context(), provider)
	c.JSON(http.StatusOK, LightweightResponse{Success: true, Data: data})
}
