package onboarding

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves the onboarding extraction endpoint.
type Handler struct {
	extractor Extractor
}

// NewHandler creates a new onboarding handler. The extractor is injected so
// tests and future variants (LLM-assisted, model-based) can swap in a
// different implementation without touching the route.
func NewHandler(extractor Extractor) *Handler {
	if extractor == nil {
		extractor = NewRuleExtractor()
	}
	return &Handler{extractor: extractor}
}

// Extract parses an arbitrary text blob (env file, curl, snippet from docs,
// etc.) and returns the URLs and possible API tokens it finds. The
// extraction is vendor-agnostic; the user picks which URL and which token to
// use in the dialog.
func (h *Handler) Extract(c *gin.Context) {
	var req ExtractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ExtractResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	data, err := h.extractor.Extract(c.Request.Context(), req.Input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ExtractResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "extraction_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, ExtractResponse{
		Success: true,
		Data:    &data,
	})
}
