// Package versioncheck exposes the /info/* HTTP endpoints (health, config,
// version, latest-version check and self-update). Version lookup itself is
// delegated to Checker and installing to updater; this file only handles
// request/response wiring.
package info

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// selfUpdateOutputLimit caps how much npm output a failed update returns.
const selfUpdateOutputLimit = 4000

// Handler carries the minimal server state needed to serve /info/* endpoints.
type Handler struct {
	version      string
	configFile   string
	configDir    string
	launchSource string

	latest  func() (version, releaseURL string, err error)
	updater *updater
}

// NewHandler creates a Handler. launchSource is how this process was itself
// invoked (see server.WithLaunchSource) — known once at boot, never detected
// or persisted.
func NewHandler(version, configFile, configDir, launchSource string) *Handler {
	return &Handler{
		version:      version,
		configFile:   configFile,
		configDir:    configDir,
		launchSource: launchSource,
		latest:       func() (string, string, error) { return New().CheckLatestVersion() },
		updater:      newUpdater(launchSource),
	}
}

// --- handlers ---------------------------------------------------------------

// GetHealthInfo is a lightweight health check that can be called frequently.
func (h *Handler) GetHealthInfo(c *gin.Context) {
	c.JSON(http.StatusOK, HealthInfoResponse{
		Health:  true,
		Status:  "healthy",
		Service: "tingly-box",
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
	latestVersion, releaseURL, err := h.latest()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, LatestVersionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	current := h.version
	hasUpdate := CompareVersions(latestVersion, current) > 0

	info := LatestVersionInfo{
		CurrentVersion: current,
		LatestVersion:  latestVersion,
		HasUpdate:      hasUpdate,
		ReleaseURL:     releaseURL,
		ShouldNotify:   hasUpdate,
		LaunchSource:   h.launchSource,
	}
	// Only probe the install (which shells out to npm) when there is
	// something to apply.
	if hasUpdate {
		sup, _ := h.updater.support(c.Request.Context())
		info.SelfUpdate = &sup
	}

	c.JSON(http.StatusOK, LatestVersionResponse{Success: true, Data: info})
}

// ApplyUpdate installs the latest published version over this npm global
// install and restarts into it (or reports that the service manager must).
func (h *Handler) ApplyUpdate(c *gin.Context) {
	latestVersion, _, err := h.latest()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, SelfUpdateResponse{Error: err.Error()})
		return
	}
	if CompareVersions(latestVersion, h.version) <= 0 {
		c.JSON(http.StatusConflict, SelfUpdateResponse{Error: "already on the latest version"})
		return
	}

	result, output, err := h.updater.update(c.Request.Context(), latestVersion)
	if err != nil {
		status := http.StatusInternalServerError
		var refused *refusedError
		if errors.As(err, &refused) {
			status = http.StatusConflict
		}
		c.JSON(status, SelfUpdateResponse{Error: err.Error(), Output: tail(output, selfUpdateOutputLimit)})
		return
	}

	c.JSON(http.StatusOK, SelfUpdateResponse{Success: true, Output: tail(output, selfUpdateOutputLimit), Data: result})
}

// tail returns at most the last n bytes of s.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
