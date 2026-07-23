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
	RequirePairing     *bool             `json:"require_pairing,omitempty"`     // TOFU pairing gate; nil → platform default
	// RemoteAgent is the remote_agent mount switch: whether this bot is used to
	// control Claude Code / SmartGuide from chat. nil → default (mounted). When
	// set true it also enables the bot (a mount with no live bot is useless).
	RemoteAgent *bool `json:"remote_agent,omitempty"`
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
	RequirePairing     *bool             `json:"require_pairing,omitempty"`     // TOFU pairing gate; nil → unchanged
	// RemoteAgent toggles the remote_agent mount (control Claude Code / SmartGuide
	// from chat). nil → unchanged. Setting it true also enables the bot (cascade);
	// setting it false leaves Enabled as-is but the bot stops if it was the only
	// active mount.
	RemoteAgent *bool `json:"remote_agent,omitempty"`
}

// PairingCodeResponse represents the response for pairing-code reveal/rotate.
type PairingCodeResponse struct {
	Success   bool   `json:"success"`
	Active    bool   `json:"active"`               // false = no live code (bot stopped, expired, or non-TOFU)
	Code      string `json:"code,omitempty"`       // cleartext pairing code, present iff Active
	ExpiresAt string `json:"expires_at,omitempty"` // RFC3339 expiry, present iff Active
	Message   string `json:"message,omitempty"`
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

// =============================================
// Feishu/Lark One-Click Registration API Types
// =============================================

// FeishuRegStartRequest is the request to start one-click app registration.
type FeishuRegStartRequest struct {
	BotUUID     string `json:"bot_uuid" binding:"required"`
	BotName     string `json:"bot_name,omitempty"`     // Optional: bot display name (for deferred creation)
	BotPlatform string `json:"bot_platform,omitempty"` // Optional: "feishu" or "lark" (for deferred creation)
}

// FeishuRegStartData is the data for the registration start response.
type FeishuRegStartData struct {
	QRURL     string `json:"qr_url"`     // Verification link; render as a QR code or open directly
	ExpiresIn int    `json:"expires_in"` // Link lifetime in seconds
}

// FeishuRegStartResponse is the response for starting one-click registration.
type FeishuRegStartResponse struct {
	Success bool               `json:"success"`
	Data    FeishuRegStartData `json:"data"`
	Error   string             `json:"error,omitempty"`
}

// FeishuRegStatusData is the data for the registration status response.
type FeishuRegStatusData struct {
	Status      string `json:"status"`                 // pending, confirmed, expired, denied, error
	BotUUID     string `json:"bot_uuid,omitempty"`     // Real bot UUID after confirmed (may differ for deferred creation)
	TenantBrand string `json:"tenant_brand,omitempty"` // "feishu" or "lark", reported by the SDK on confirmation
}

// FeishuRegStatusResponse is the response for polling registration status.
type FeishuRegStatusResponse struct {
	Success bool                `json:"success"`
	Data    FeishuRegStatusData `json:"data,omitempty"`
	Error   string              `json:"error,omitempty"`
}
