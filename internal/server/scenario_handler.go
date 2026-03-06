package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

type ScenarioFlagUpdateRequest struct {
	Value bool `json:"value"`
}

// ScenarioUpdateRequest represents the request to update a scenario
type ScenarioUpdateRequest struct {
	Scenario typ.RuleScenario  `json:"scenario" binding:"required" example:"claude_code"`
	Flags    typ.ScenarioFlags `json:"flags" binding:"required"`
}

// ScenariosResponse represents the response for getting all scenarios
type ScenariosResponse struct {
	Success bool                 `json:"success" example:"true"`
	Data    []typ.ScenarioConfig `json:"data"`
}

// ScenarioResponse represents the response for a single scenario
type ScenarioResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    typ.ScenarioConfig `json:"data"`
}

// ScenarioFlagResponse represents the response for a scenario flag
type ScenarioFlagResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		Scenario typ.RuleScenario `json:"scenario" example:"claude_code"`
		Flag     string           `json:"flag" example:"unified"`
		Value    bool             `json:"value" example:"true"`
	} `json:"data"`
}

// ScenarioUpdateResponse represents the response for updating scenario
type ScenarioUpdateResponse struct {
	Success bool               `json:"success" example:"true"`
	Message string             `json:"message" example:"Scenario config saved successfully"`
	Data    typ.ScenarioConfig `json:"data"`
}

// GetScenarios returns all scenario configurations
func (s *Server) GetScenarios(c *gin.Context) {
	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	scenarios := cfg.GetScenarios()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    scenarios,
	})
}

// GetScenarioConfig returns configuration for a specific scenario
func (s *Server) GetScenarioConfig(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if scenario == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Scenario parameter is required",
		})
		return
	}

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	config := cfg.GetScenarioConfig(scenario)
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
func (s *Server) SetScenarioConfig(c *gin.Context) {
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

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	if err := cfg.SetScenarioConfig(config); err != nil {
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
func (s *Server) GetScenarioFlag(c *gin.Context) {
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

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	value := cfg.GetScenarioFlag(scenario, flag)

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
func (s *Server) SetScenarioFlag(c *gin.Context) {
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

	cfg := s.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	// Get the old value before setting
	oldValue := cfg.GetScenarioFlag(scenario, flag)

	if err := cfg.SetScenarioFlag(scenario, flag, request.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save scenario flag: " + err.Error(),
		})
		return
	}

	// Handle special flags that require runtime actions
	if scenario == typ.ScenarioGlobal && flag == "enable_remote_coder" && oldValue != request.Value {
		if request.Value {
			// Enable remote control: start the service and sync bots
			logrus.Info("Enabling remote control...")
			if err := s.StartRemoteCoder(); err != nil {
				logrus.WithError(err).Warn("Failed to start remote control")
			} else {
				// Sync bots after a short delay to allow the service to initialize
				go func() {
					ctx := context.Background()
					if err := s.SyncRemoteCoderBots(ctx); err != nil {
						logrus.WithError(err).Warn("Failed to sync bots after enabling remote control")
					}
				}()
			}
		} else {
			// Disable remote control: stop the service
			logrus.Info("Disabling remote control...")
			s.StopRemoteCoder()
		}
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
