package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetHealthInfo handles health check requests
func (s *Server) GetHealthInfo(c *gin.Context) {
	c.JSON(http.StatusOK, HealthInfoResponse{
		Health:  true,
		Status:  "healthy",
		Service: "tingly-box",
	})
}

// HealthInfoResponse represents the health check response
type HealthInfoResponse struct {
	Status  string `json:"status" example:"healthy"`
	Service string `json:"service" example:"tingly-box"`
	Health  bool   `json:"health" example:"healthy"`
}

func (s *Server) GetInfoConfig(c *gin.Context) {
	// Return configuration information
	configInfo := ConfigInfo{
		ConfigPath: s.config.ConfigFile,
		ConfigDir:  s.config.ConfigDir,
	}

	c.JSON(http.StatusOK, ConfigInfoResponse{
		Success: true,
		Data:    configInfo,
	})
}

// ConfigInfo represents configuration information
type ConfigInfo struct {
	ConfigPath string `json:"config_path" example:"/Users/user/.tingly-box/config.json"`
	ConfigDir  string `json:"config_dir" example:"/Users/user/.tingly-box"`
}

// ConfigInfoResponse represents the response for config info endpoint
type ConfigInfoResponse struct {
	Success bool       `json:"success" example:"true"`
	Data    ConfigInfo `json:"data"`
}

func (s *Server) GetInfoVersion(c *gin.Context) {
	c.JSON(http.StatusOK, VersionInfoResponse{
		Success: true,
		Data: VersionInfo{
			Version: s.version,
		},
	})
}

type VersionInfoResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Provider models successfully"`
	Data    VersionInfo `json:"data"`
}

type VersionInfo struct {
	Version string `json:"version" example:"1.0.0"`
}
