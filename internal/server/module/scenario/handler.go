package scenario

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RemoteControlController defines the interface for controlling remote coder service
type RemoteControlController interface {
	StartRemoteCoder() error
	StopRemoteCoder()
	SyncRemoteCoderBots(ctx context.Context) error
}

// Handler handles scenario HTTP requests
type Handler struct {
	config    *config.Config
	rcControl RemoteControlController
}

// NewHandler creates a new scenario handler
func NewHandler(cfg *config.Config, rcControl RemoteControlController) *Handler {
	return &Handler{
		config:    cfg,
		rcControl: rcControl,
	}
}

// GetScenarios returns all scenario configurations
// GetScenarioDescriptors returns all registered scenario descriptors from the registry.
func (h *Handler) GetScenarioDescriptors(c *gin.Context) {
	descriptors := typ.RegisteredScenarioDescriptors()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": descriptors})
}

func (h *Handler) GetScenarios(c *gin.Context) {
	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	scenarios := h.config.GetScenarios()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    scenarios,
	})
}

// GetScenarioConfig returns configuration for a specific scenario
func (h *Handler) GetScenarioConfig(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	config := h.config.GetScenarioConfig(scenario)
	if config == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Scenario config not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// SetScenarioConfig creates or updates scenario configuration
func (h *Handler) SetScenarioConfig(c *gin.Context) {
	var config typ.ScenarioConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if config.Scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario field is required",
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := h.config.SetScenarioConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save scenario config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scenario config saved successfully",
		"data":    config,
	})
}

// GetScenarioFlag returns a specific flag value for a scenario
func (h *Handler) GetScenarioFlag(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	flag := c.Param("flag")
	if flag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Flag parameter is required",
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	value := h.config.GetScenarioFlag(scenario, flag)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"scenario": scenario,
			"flag":     flag,
			"value":    value,
		},
	})
}

// SetScenarioFlag sets a specific flag value for a scenario
func (h *Handler) SetScenarioFlag(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	flag := c.Param("flag")
	if flag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Flag parameter is required",
		})
		return
	}

	request := new(ScenarioFlagUpdateRequest)
	if err := c.ShouldBindJSON(&request); err != nil {
		logrus.Printf("[ERROR] SetScenarioFlag ShouldBindJSON failed: %v, scenario=%s, flag=%s", err, scenario, flag)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logrus.Printf("[DEBUG] SetScenarioFlag success: scenario=%s, flag=%s, value=%v", scenario, flag, request.Value)

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := h.config.SetScenarioFlag(scenario, flag, request.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save scenario flag: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scenario flag saved successfully",
		"data": gin.H{
			"scenario": scenario,
			"flag":     flag,
			"value":    request.Value,
		},
	})
}

// GetScenarioStringFlag returns a specific string flag value for a scenario
func (h *Handler) GetScenarioStringFlag(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	flag := c.Param("flag")
	if flag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Flag parameter is required",
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	value := h.config.GetScenarioStringFlag(scenario, flag)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"scenario": scenario,
			"flag":     flag,
			"value":    value,
		},
	})
}

// SetScenarioStringFlag sets a specific string flag value for a scenario
func (h *Handler) SetScenarioStringFlag(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	flag := c.Param("flag")
	if flag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Flag parameter is required",
		})
		return
	}

	request := new(ScenarioStringFlagUpdateRequest)
	if err := c.ShouldBindJSON(&request); err != nil {
		logrus.Printf("[ERROR] SetScenarioStringFlag ShouldBindJSON failed: %v, scenario=%s, flag=%s", err, scenario, flag)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logrus.Printf("[DEBUG] SetScenarioStringFlag: scenario=%s, flag=%s, value=%s", scenario, flag, request.Value)

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := h.config.SetScenarioStringFlag(scenario, flag, request.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save scenario flag: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scenario flag saved successfully",
		"data": gin.H{
			"scenario": scenario,
			"flag":     flag,
			"value":    request.Value,
		},
	})
}

// GetScenarioIntFlag returns a specific integer flag value for a scenario
func (h *Handler) GetScenarioIntFlag(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	flag := c.Param("flag")
	if flag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Flag parameter is required",
		})
		return
	}

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	value := h.config.GetScenarioIntFlag(scenario, flag)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"scenario": scenario,
			"flag":     flag,
			"value":    value,
		},
	})
}

// SetScenarioIntFlag sets a specific integer flag value for a scenario
func (h *Handler) SetScenarioIntFlag(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	flag := c.Param("flag")
	if flag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Flag parameter is required",
		})
		return
	}

	request := new(ScenarioIntFlagUpdateRequest)
	if err := c.ShouldBindJSON(&request); err != nil {
		logrus.Printf("[ERROR] SetScenarioIntFlag ShouldBindJSON failed: %v, scenario=%s, flag=%s", err, scenario, flag)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logrus.Printf("[DEBUG] SetScenarioIntFlag: scenario=%s, flag=%s, value=%d", scenario, flag, request.Value)

	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := h.config.SetScenarioIntFlag(scenario, flag, request.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save scenario flag: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scenario flag saved successfully",
		"data": gin.H{
			"scenario": scenario,
			"flag":     flag,
			"value":    request.Value,
		},
	})
}

// GetProfiles returns all profiles for a base scenario
func (h *Handler) GetProfiles(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "scenario parameter is required"})
		return
	}
	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "config not available"})
		return
	}

	profiles := h.config.GetProfiles(scenario)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profiles})
}

// CreateProfile creates a new profile for a base scenario
func (h *Handler) CreateProfile(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "scenario parameter is required"})
		return
	}
	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "config not available"})
		return
	}

	var req ProfileCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name is required"})
		return
	}

	meta, err := h.config.CreateProfile(scenario, req.Name, req.Unified)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Auto-generate the Claude Code settings file for the new profile so it is
	// immediately usable without manual configuration.
	profiledScenario := string(typ.ProfiledScenarioName(scenario, meta.ID))
	baseURL := middleware.BaseURLFromRequest(c, h.config.GetServerPort())
	apiKey := h.config.GetModelToken()
	env := agent.GenerateCCEnv(h.config, baseURL, apiKey, profiledScenario, meta.Unified, true)
	if _, settingsErr := agent.BuildCCProfileSettings(meta.ID, profiledScenario, meta.Name, env); settingsErr != nil {
		logrus.WithError(settingsErr).Warn("failed to create Claude Code settings for new profile")
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": meta})
}

// UpdateProfile updates a profile's name and/or mode
func (h *Handler) UpdateProfile(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "scenario parameter is required"})
		return
	}
	profileID := c.Param("id")
	if profileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "profile id is required"})
		return
	}
	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "config not available"})
		return
	}

	var req ProfileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// If name is not provided, preserve existing profile name
	existing, ok := h.config.GetProfile(scenario, profileID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "profile not found"})
		return
	}
	oldName := existing.Name
	if req.Name == "" {
		req.Name = existing.Name
	}

	if err := h.config.UpdateProfile(scenario, profileID, req.Name, req.Unified); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, _ := h.config.GetProfile(scenario, profileID)
	if req.Name != oldName {
		// Profile directories are derived from config, so materialize the new
		// canonical path first and only then remove the old generated artifacts.
		profiledScenario := string(typ.ProfiledScenarioName(scenario, profileID))
		baseURL := middleware.BaseURLFromRequest(c, h.config.GetServerPort())
		apiKey := h.config.GetModelToken()
		env := agent.GenerateCCEnv(h.config, baseURL, apiKey, profiledScenario, updated.Unified, true)
		if _, settingsErr := agent.BuildCCProfileSettings(profileID, profiledScenario, updated.Name, env); settingsErr != nil {
			logrus.WithError(settingsErr).Warn("failed to rebuild Claude Code settings after profile rename")
		} else if cleanupErr := agent.RemoveRenamedCCProfileArtifacts(profileID, oldName, updated.Name); cleanupErr != nil {
			logrus.WithError(cleanupErr).Warn("failed to clean old Claude Code profile artifacts after rename")
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "profile updated", "data": updated})
}

// DeleteProfile deletes a profile by ID
func (h *Handler) DeleteProfile(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "scenario parameter is required"})
		return
	}
	profileID := c.Param("id")
	if profileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "profile id is required"})
		return
	}
	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "config not available"})
		return
	}

	// Capture the name before deletion so the derived runtime artifacts can be
	// removed after their source-of-truth config is deleted.
	profileName := ""
	if existing, ok := h.config.GetProfile(scenario, profileID); ok {
		profileName = existing.Name
	}

	if err := h.config.DeleteProfile(scenario, profileID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if cleanupErr := agent.RemoveCCProfileArtifacts(profileID, profileName); cleanupErr != nil {
		logrus.WithError(cleanupErr).Warn("failed to clean Claude Code profile artifacts after delete")
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "profile deleted"})
}
