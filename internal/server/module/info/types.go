package info

// --- response types ---------------------------------------------------------

// HealthInfoResponse is the JSON envelope for GET /info/health.
type HealthInfoResponse struct {
	Status  string `json:"status" example:"healthy"`
	Service string `json:"service" example:"tingly-box"`
	Health  bool   `json:"health" example:"true"`
}

// ConfigInfo holds configuration path information.
type ConfigInfo struct {
	ConfigPath string `json:"config_path" example:"/Users/user/.tingly-box/config.json"`
	ConfigDir  string `json:"config_dir" example:"/Users/user/.tingly-box"`
}

// ConfigInfoResponse is the JSON envelope for GET /info/config.
type ConfigInfoResponse struct {
	Success bool       `json:"success" example:"true"`
	Data    ConfigInfo `json:"data"`
}

// VersionInfo holds the running version string.
type VersionInfo struct {
	Version string `json:"version" example:"1.0.0"`
	// LaunchSource is how this process was itself invoked ("binary", "npx",
	// "npm", "npx-bundle", "npm-bundle"; "" if unknown). The frontend uses it
	// to default multi-method pickers (update instructions, quick-start
	// commands) to whichever method matches how the user is already running
	// Tingly Box. See .design/shortcut.md.
	LaunchSource string `json:"launch_source" example:"npx"`
}

// VersionInfoResponse is the JSON envelope for GET /info/version.
type VersionInfoResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message,omitempty"`
	Data    VersionInfo `json:"data"`
}

// LatestVersionInfo contains version comparison results.
type LatestVersionInfo struct {
	CurrentVersion string `json:"current_version" example:"0.260124.1430"`
	LatestVersion  string `json:"latest_version" example:"0.260130.1200"`
	HasUpdate      bool   `json:"has_update" example:"true"`
	ReleaseURL     string `json:"release_url" example:"https://github.com/tingly-dev/tingly-box/releases"`
	ShouldNotify   bool   `json:"should_notify" example:"true"`
	// LaunchSource is how this process was itself invoked; see VersionInfo.
	// The update panel uses it to default its method picker (npx/npm/docker)
	// to whichever channel matches this install.
	LaunchSource string `json:"launch_source" example:"npx"`
}

// LatestVersionResponse is the JSON envelope for GET /info/version/check.
type LatestVersionResponse struct {
	Success bool              `json:"success"`
	Error   string            `json:"error,omitempty"`
	Data    LatestVersionInfo `json:"data,omitempty"`
}
