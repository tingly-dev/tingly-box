package codeximport

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type Handler struct {
	importer *Importer
	config   *config.Config
}

func NewHandler(importer *Importer, cfg *config.Config) *Handler {
	if importer == nil {
		importer = NewImporter()
	}
	return &Handler{importer: importer, config: cfg}
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

	if h.config != nil && !req.DryRun {
		autoUndo := req.AutoUndoOnStop != nil && *req.AutoUndoOnStop
		extensions := map[string]interface{}{
			importStateActiveKey:         result.UpdatedSessionFiles > 0 || result.UpdatedArchivedFiles > 0 || result.UpdatedThreadRows > 0,
			importStateSourceProviderKey: result.SourceProvider,
			importStateTargetProviderKey: result.TargetProvider,
			importStateCodexHomeKey:      result.CodexHome,
			importStateSqliteHomeKey:     result.SqliteHome,
			importStateStateDBPathKey:    result.StateDBPath,
			importStateAutoUndoOnStopKey: autoUndo,
		}
		if req.SourceProvider == "tingly-box" && req.TargetProvider == "openai" {
			extensions = map[string]interface{}{
				importStateActiveKey:         false,
				importStateSourceProviderKey: nil,
				importStateTargetProviderKey: nil,
				importStateCodexHomeKey:      nil,
				importStateSqliteHomeKey:     nil,
				importStateStateDBPathKey:    nil,
				importStateAutoUndoOnStopKey: false,
			}
		}
		if err := h.config.SetScenarioExtensions(typ.ScenarioCodex, extensions); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "failed to persist Codex import state: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, result)
}
