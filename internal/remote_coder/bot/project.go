package bot

import (
	"time"
)

// Project represents a workspace/project identified by path
type Project struct {
	ID        string    `json:"id"`       // UUID
	Path      string    `json:"path"`     // Project path (e.g., /Users/yz/Project/myapp)
	Name      string    `json:"name"`     // Optional display name (derived from path)
	OwnerID   string    `json:"owner_id"` // User ID who created the project
	Platform  string    `json:"platform"` // Platform (telegram, discord, etc.)
	BotUUID   string    `json:"bot_uuid"` // Bot UUID managing this project
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GroupProjectBinding associates a group chat with a project
type GroupProjectBinding struct {
	ID        string    `json:"id"`         // UUID
	GroupID   string    `json:"group_id"`   // Platform-specific group ID
	Platform  string    `json:"platform"`   // Platform identifier
	ProjectID string    `json:"project_id"` // Associated project ID
	BotUUID   string    `json:"bot_uuid"`   // Bot managing this group
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectWithBinding combines project info with its group binding
type ProjectWithBinding struct {
	Project *Project             `json:"project"`
	Binding *GroupProjectBinding `json:"binding,omitempty"`
}
