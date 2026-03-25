package imbot

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// =============================================
// ImBot Settings API Types
// =============================================

// ListResponse represents the response for listing ImBot settings
type ListResponse struct {
	Success  bool          `json:"success"`
	Settings []db.Settings `json:"settings"`
}

// SettingsResponse represents the response for a single ImBot settings
type SettingsResponse struct {
	Success  bool        `json:"success"`
	Settings db.Settings `json:"settings"`
}

// CreateRequest represents the request to create ImBot settings
type CreateRequest struct {
	UUID               string            `json:"uuid,omitempty"`
	Name               string            `json:"name,omitempty"`
	Platform           string            `json:"platform"`
	AuthType           string            `json:"auth_type"`
	Auth               map[string]string `json:"auth"`
	ProxyURL           string            `json:"proxy_url,omitempty"`
	ChatID             string            `json:"chat_id,omitempty"`
	BashAllowlist      []string          `json:"bash_allowlist,omitempty"`
	DefaultCwd         string            `json:"default_cwd,omitempty"`   // Default working directory
	DefaultAgent       string            `json:"default_agent,omitempty"` // Default Agent UUID
	Enabled            bool              `json:"enabled"`
	Token              string            `json:"token,omitempty"`               // Legacy field
	SmartGuideProvider string            `json:"smartguide_provider,omitempty"` // Provider UUID
	SmartGuideModel    string            `json:"smartguide_model,omitempty"`    // Model identifier
}

// UpdateRequest represents the request to update ImBot settings
type UpdateRequest struct {
	Name               string            `json:"name,omitempty"`
	Platform           string            `json:"platform,omitempty"`
	AuthType           string            `json:"auth_type,omitempty"`
	Auth               map[string]string `json:"auth,omitempty"`
	ProxyURL           string            `json:"proxy_url,omitempty"`
	ChatID             string            `json:"chat_id,omitempty"`
	BashAllowlist      []string          `json:"bash_allowlist,omitempty"`
	DefaultCwd         *string           `json:"default_cwd,omitempty"`         // Pointer for partial update
	DefaultAgent       *string           `json:"default_agent,omitempty"`       // Pointer for partial update
	Enabled            *bool             `json:"enabled,omitempty"`             // Pointer to allow partial update
	Token              string            `json:"token,omitempty"`               // Legacy field
	SmartGuideProvider *string           `json:"smartguide_provider,omitempty"` // Provider UUID
	SmartGuideModel    *string           `json:"smartguide_model,omitempty"`    // Model identifier
}

// ToggleResponse represents the response for toggling ImBot settings
type ToggleResponse struct {
	Success bool `json:"success"`
	Enabled bool `json:"enabled"`
}

// PlatformsResponse represents the response for listing platforms
type PlatformsResponse struct {
	Success    bool             `json:"success"`
	Platforms  []PlatformConfig `json:"platforms"`
	Categories gin.H            `json:"categories"`
}

// PlatformConfigResponse represents the response for platform config
type PlatformConfigResponse struct {
	Success  bool           `json:"success"`
	Platform PlatformConfig `json:"platform"`
}

// PlatformConfig represents a platform configuration
type PlatformConfig struct {
	Platform    string            `json:"platform"`
	DisplayName string            `json:"display_name"`
	AuthType    string            `json:"auth_type"`
	Category    string            `json:"category"`
	Fields      []imbot.FieldSpec `json:"fields"`
}

// DeleteResponse represents the response for delete operations
type DeleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
