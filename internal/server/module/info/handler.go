// Package versioncheck exposes the /info/* HTTP endpoints (health, config,
// version, and latest-version check). Version lookup itself is delegated to
// Checker; this file only handles request/response wiring.
package info

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// Handler carries the minimal server state needed to serve /info/* endpoints.
type Handler struct {
	version       string
	configFile    string
	configDir     string
	launchSource  string
	instanceToken string
}

// NewHandler creates a Handler. launchSource is how this process was itself
// invoked (see server.WithLaunchSource) — known once at boot, never detected
// or persisted. instanceToken, when non-empty, is echoed on the health
// endpoint so a launcher can tell this instance from an older one still
// answering on the same port (see server.WithInstanceToken).
func NewHandler(version, configFile, configDir, launchSource, instanceToken string) *Handler {
	return &Handler{
		version:       version,
		configFile:    configFile,
		configDir:     configDir,
		launchSource:  launchSource,
		instanceToken: instanceToken,
	}
}

// --- handlers ---------------------------------------------------------------

// GetHealthInfo is a lightweight health check that can be called frequently.
// It also carries the instance handshake headers: the launcher that started
// this process requires its token back before it will announce the URL.
func (h *Handler) GetHealthInfo(c *gin.Context) {
	c.Header(constant.HealthServiceHeader, constant.HealthServiceName)
	if h.instanceToken != "" {
		c.Header(constant.HealthInstanceHeader, h.instanceToken)
	}
	c.JSON(http.StatusOK, HealthInfoResponse{
		Health:  true,
		Status:  "healthy",
		Service: constant.HealthServiceName,
	})
}

// GetInfoConfig returns the runtime configuration paths.
func (h *Handler) GetInfoConfig(c *gin.Context) {
	c.JSON(http.StatusOK, ConfigInfoResponse{
		Success: true,
		Data: ConfigInfo{
			ConfigPath: h.configFile,
			ConfigDir:  h.configDir,
		},
	})
}

// GetInfoVersion returns the current running version.
func (h *Handler) GetInfoVersion(c *gin.Context) {
	c.JSON(http.StatusOK, VersionInfoResponse{
		Success: true,
		Data:    VersionInfo{Version: h.version, LaunchSource: h.launchSource},
	})
}

// GetLatestVersion checks the npm registry for the latest published version
// and compares it with the running version.
func (h *Handler) GetLatestVersion(c *gin.Context) {
	checker := New()
	latestVersion, releaseURL, err := checker.CheckLatestVersion()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, LatestVersionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	current := h.version
	hasUpdate := CompareVersions(latestVersion, current) > 0

	c.JSON(http.StatusOK, LatestVersionResponse{
		Success: true,
		Data: LatestVersionInfo{
			CurrentVersion: current,
			LatestVersion:  latestVersion,
			HasUpdate:      hasUpdate,
			ReleaseURL:     releaseURL,
			ShouldNotify:   hasUpdate,
			LaunchSource:   h.launchSource,
		},
	})
}
