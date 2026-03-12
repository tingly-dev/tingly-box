package scenario

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/server/config"
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
