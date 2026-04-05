package codeximport

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	importer *Importer
}

func NewHandler(importer *Importer) *Handler {
	if importer == nil {
		importer = NewImporter()
	}
	return &Handler{importer: importer}
}

func (h *Handler) ImportOpenAISessions(c *gin.Context) {
	var req ImportOpenAISessionsRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	result, err := h.importer.ImportOpenAISessions(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}
