package access

import (
	"context"
	"encoding/json"
	"time"
)

type BotCapability struct {
	BotUUID   string          `json:"bot_uuid"`
	Name      CapabilityName  `json:"capability"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Actor struct {
	ID              string    `json:"id"`
	BotUUID         string    `json:"bot_uuid"`
	Platform        string    `json:"platform"`
	ExternalActorID string    `json:"external_actor_id"`
	DisplayName     string    `json:"display_name,omitempty"`
	LastSeenAt      time.Time `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type DirectChat struct {
	ID             string     `json:"id"`
	BotUUID        string     `json:"bot_uuid"`
	Platform       string     `json:"platform"`
	ExternalChatID string     `json:"external_chat_id"`
	PeerActorID    string     `json:"peer_actor_id,omitempty"`
	Blocked        bool       `json:"blocked"`
	PairedAt       *time.Time `json:"paired_at,omitempty"`
	ProjectPath    string     `json:"project_path,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Group struct {
	ID              string    `json:"id"`
	BotUUID         string    `json:"bot_uuid"`
	Platform        string    `json:"platform"`
	ExternalGroupID string    `json:"external_group_id"`
	Name            string    `json:"name,omitempty"`
	Blocked         bool      `json:"blocked"`
	ProjectPath     string    `json:"project_path,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Permission struct {
	Capability CapabilityName `json:"capability"`
	Action     ActionName     `json:"action"`
	Effect     AccessEffect   `json:"effect"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type GroupActor struct {
	GroupID     string       `json:"group_id"`
	Actor       Actor        `json:"actor"`
	Label       string       `json:"label,omitempty"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Route struct {
	ID          string          `json:"id"`
	BotUUID     string          `json:"bot_uuid"`
	Name        string          `json:"name"`
	Source      string          `json:"source"`
	EventFilter json.RawMessage `json:"event_filter"`
	Target      TargetRef       `json:"target"`
	Enabled     bool            `json:"enabled"`
	Options     json.RawMessage `json:"options"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type TransportFactsSource interface {
	TransportFacts(botUUID string, capability CapabilityName, action ActionName) (TransportStatus, bool)
}

type ResolvedRoute struct {
	Route            Route  `json:"route"`
	ExternalTargetID string `json:"external_target_id"`
}

type RouteResolver interface {
	ResolveRoute(ctx context.Context, source, event string) (*ResolvedRoute, error)
}
