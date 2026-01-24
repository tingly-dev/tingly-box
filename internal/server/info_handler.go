package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetHealthInfo handles health check requests
// This is a lightweight health check that can be called frequently
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

// GetLatestVersion checks GitHub releases for the latest version
func (s *Server) GetLatestVersion(c *gin.Context) {
	checker := newVersionChecker()
	latestVersion, releaseURL, err := checker.CheckLatestVersion()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, LatestVersionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	currentVersion := s.version
	hasUpdate := compareVersions(latestVersion, currentVersion) > 0

	c.JSON(http.StatusOK, LatestVersionResponse{
		Success: true,
		Data: LatestVersionInfo{
			CurrentVersion: currentVersion,
			LatestVersion:  latestVersion,
			HasUpdate:      hasUpdate,
			ReleaseURL:     releaseURL,
			ShouldNotify:   hasUpdate,
		},
	})
}

// LatestVersionResponse represents the response for version check endpoint
type LatestVersionResponse struct {
	Success bool              `json:"success"`
	Error   string            `json:"error,omitempty"`
	Data    LatestVersionInfo `json:"data,omitempty"`
}

// LatestVersionInfo contains version comparison information
type LatestVersionInfo struct {
	CurrentVersion string `json:"current_version" example:"0.260124.1430"`
	LatestVersion  string `json:"latest_version" example:"0.260130.1200"`
	HasUpdate      bool   `json:"has_update" example:"true"`
	ReleaseURL     string `json:"release_url" example:"https://github.com/tingly-dev/tingly-box/releases/tag/v0.260130.1200"`
	ShouldNotify   bool   `json:"should_notify" example:"true"`
}
