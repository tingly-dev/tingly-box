package configapply

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	aiagent "github.com/tingly-dev/tingly-box/ai/agent"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// collectWorkBuddyRuleModels returns the request_models of every active rule in
// the WorkBuddy scenario, deduplicated and in declaration order.
func collectWorkBuddyRuleModels(cfg *config.Config) []string {
	return collectScenarioRuleModels(cfg, typ.ScenarioWorkBuddy)
}

const noWorkBuddyRulesMessage = "Add a model rule for WorkBuddy first — Auto Config writes one WorkBuddy model per rule."

// workBuddyDisplayPaths names the installed editions' files, or every edition's
// when none is installed, so the UI always shows where Auto Config looks.
func workBuddyDisplayPaths(installed, all []config.WorkBuddyTarget) string {
	if len(installed) > 0 {
		return config.WorkBuddyDisplayPaths(installed)
	}
	return config.WorkBuddyDisplayPaths(all)
}

// ApplyWorkBuddyConfigFromState writes the active WorkBuddy-scenario rules into
// the models.json of every installed WorkBuddy edition. Like the other agents it
// does NOT touch routing rules, and unlike OpenCode it does not invent a default
// model: with no rules there is nothing to write, which is reported instead of
// creating a phantom entry.
func (h *Handler) ApplyWorkBuddyConfigFromState(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, ApplyWorkBuddyConfigResponse{
			WorkBuddyApplyResult: config.WorkBuddyApplyResult{ApplyResult: config.ApplyResult{Message: "Global config not available"}},
		})
		return
	}
	models := collectWorkBuddyRuleModels(cfg)
	if len(models) == 0 {
		c.JSON(http.StatusOK, ApplyWorkBuddyConfigResponse{
			WorkBuddyApplyResult: config.WorkBuddyApplyResult{ApplyResult: config.ApplyResult{Message: noWorkBuddyRulesMessage}},
		})
		return
	}

	baseURL := middleware.BaseURLFromRequest(c, h.resolvedPort()) + "/tingly/workbuddy"
	result, err := config.ApplyWorkBuddyConfig(baseURL, cfg.GetModelToken(), models)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApplyWorkBuddyConfigResponse{
			WorkBuddyApplyResult: config.WorkBuddyApplyResult{ApplyResult: config.ApplyResult{Message: "Internal error: " + err.Error()}},
		})
		return
	}
	c.JSON(http.StatusOK, ApplyWorkBuddyConfigResponse{WorkBuddyApplyResult: *result})
}

// GetWorkBuddyConfigPreview returns the models.json entries Auto Config would
// write now, with the API key masked.
func (h *Handler) GetWorkBuddyConfigPreview(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, WorkBuddyConfigPreviewResponse{Message: "Global config not available"})
		return
	}
	installed, all, err := config.WorkBuddyTargets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, WorkBuddyConfigPreviewResponse{Message: err.Error()})
		return
	}
	models := collectWorkBuddyRuleModels(cfg)
	baseURL := middleware.BaseURLFromRequest(c, h.resolvedPort()) + "/tingly/workbuddy"
	entries := config.BuildWorkBuddyModelEntries(baseURL, maskSecret(cfg.GetModelToken()), models)
	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, WorkBuddyConfigPreviewResponse{Message: "Failed to generate config JSON: " + err.Error()})
		return
	}
	resp := WorkBuddyConfigPreviewResponse{Success: true, ConfigJSON: string(out), Path: workBuddyDisplayPaths(installed, all), Models: models}
	switch {
	case len(models) == 0:
		resp.Message = noWorkBuddyRulesMessage
	case len(installed) == 0:
		resp.Message = config.WorkBuddyNotInstalledMessage(all)
	}
	c.JSON(http.StatusOK, resp)
}

// GetWorkBuddyConfigStatus compares the installed editions' models.json with
// the models the WorkBuddy rules produce now.
func (h *Handler) GetWorkBuddyConfigStatus(c *gin.Context) {
	installed, all, err := config.WorkBuddyTargets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ClientConfigStatusResponse{Error: err.Error()})
		return
	}
	state, diffs := config.CompareWorkBuddyTargets(installed, collectWorkBuddyRuleModels(h.config))
	c.JSON(http.StatusOK, ClientConfigStatusResponse{
		Success:     true,
		State:       state,
		Path:        workBuddyDisplayPaths(installed, all),
		Differences: toDifferences(diffs),
	})
}

// RestoreWorkBuddyConfig rolls each installed edition's models.json back to its
// most recent backup (the pre-restore state is itself backed up first).
func (h *Handler) RestoreWorkBuddyConfig(c *gin.Context) {
	const agentType = "workbuddy"
	installed, all, err := config.WorkBuddyTargets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, RestoreConfigResponse{AgentType: agentType, Message: "Restore failed: " + err.Error()})
		return
	}
	if len(installed) == 0 {
		msg := config.WorkBuddyNotInstalledMessage(all)
		c.JSON(http.StatusOK, RestoreConfigResponse{AgentType: agentType, Failures: []string{msg}, Message: "Restore failed for " + agentType + ": " + msg})
		return
	}

	resp := RestoreConfigResponse{AgentType: agentType, RestoredFiles: []string{}, PreRestoreBackups: []string{}}
	var restored []string
	for _, t := range installed {
		r, err := aiagent.RestoreLatestBackup(t.Path)
		if err != nil {
			msg := err.Error()
			if r != nil && r.Message != "" {
				msg = r.Message
			}
			resp.Failures = append(resp.Failures, t.DisplayPath+": "+msg)
			continue
		}
		restored = append(restored, t.DisplayPath)
		resp.RestoredFiles = append(resp.RestoredFiles, t.DisplayPath+" <- "+r.RestoredFrom)
		if r.PreRestoreBackup != "" {
			resp.PreRestoreBackups = append(resp.PreRestoreBackups, r.PreRestoreBackup)
		}
	}
	resp.Success = len(restored) > 0 && len(resp.Failures) == 0
	switch {
	case resp.Success:
		resp.Message = "Restored " + strings.Join(restored, ", ")
	case len(restored) > 0:
		resp.Message = "Partially restored " + agentType + ": restored " + strings.Join(restored, ", ") +
			"; failed " + strings.Join(resp.Failures, "; ")
	default:
		resp.Message = "Restore failed for " + agentType + ": " + strings.Join(resp.Failures, "; ")
	}
	c.JSON(http.StatusOK, resp)
}
