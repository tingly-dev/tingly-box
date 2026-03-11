package skill

import (
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// =============================================
// Skill Management API Models
// =============================================

// SkillLocationsResponse represents the response for listing skill locations
type SkillLocationsResponse struct {
	Success bool                `json:"success" example:"true"`
	Data    []typ.SkillLocation `json:"data"`
}

// AddSkillLocationRequest represents the request to add a skill location
type AddSkillLocationRequest struct {
	Name      string        `json:"name" binding:"required" description:"Display name for the location" example:"Claude Code Skills"`
	Path      string        `json:"path" binding:"required" description:"Full file system path to the skills directory" example:"/Users/user/.claude/skills"`
	IDESource typ.IDESource `json:"ide_source" binding:"required" description:"IDE/source type" example:"claude_code"`
}

// AddSkillLocationResponse represents the response for adding a skill location
type AddSkillLocationResponse struct {
	Success bool               `json:"success" example:"true"`
	Message string             `json:"message" example:"Skill location added successfully"`
	Data    *typ.SkillLocation `json:"data,omitempty"`
}

// SkillLocationResponse represents the response for getting a skill location
type SkillLocationResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    *typ.SkillLocation `json:"data,omitempty"`
}

// RemoveSkillLocationResponse represents the response for removing a skill location
type RemoveSkillLocationResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Skill location removed successfully"`
}

// RefreshSkillLocationResponse represents the response for refreshing a skill location
type RefreshSkillLocationResponse struct {
	Success bool            `json:"success" example:"true"`
	Message string          `json:"message" example:"Skill location refreshed successfully"`
	Data    *typ.ScanResult `json:"data,omitempty"`
}

// DiscoverIdesResponse represents the response for discovering IDEs
type DiscoverIdesResponse struct {
	Success bool                 `json:"success" example:"true"`
	Data    *typ.DiscoveryResult `json:"data,omitempty"`
}

// ImportSkillLocationsRequest represents the request to import skill locations
type ImportSkillLocationsRequest struct {
	Locations []typ.SkillLocation `json:"locations" binding:"required" description:"Array of skill locations to import"`
}

// ImportSkillLocationsResponse represents the response for importing skill locations
type ImportSkillLocationsResponse struct {
	Success bool                `json:"success" example:"true"`
	Message string              `json:"message" example:"Imported 5 skill locations"`
	Data    []typ.SkillLocation `json:"data,omitempty"`
}

// ScanIdesResponse represents the response for scanning all IDEs
type ScanIdesResponse struct {
	Success bool                 `json:"success" example:"true"`
	Data    *typ.DiscoveryResult `json:"data,omitempty"`
}

// SkillContentResponse represents the response for getting skill content
type SkillContentResponse struct {
	Success bool       `json:"success" example:"true"`
	Data    *typ.Skill `json:"data,omitempty"`
}
