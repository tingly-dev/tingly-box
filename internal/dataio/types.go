package dataio

import (
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// DataLine is the base type for all data lines (export/import)
type DataLine struct {
	Type string `json:"type"`
}

// Metadata represents the metadata line (used for both export and import)
type Metadata struct {
	Type       string `json:"type"`
	Version    string `json:"version"`
	ExportedAt string `json:"exported_at"`
}

// RuleData represents the rule data (used for both export and import)
type RuleData struct {
	Type          string                 `json:"type"`
	UUID          string                 `json:"uuid"`
	Scenario      string                 `json:"scenario"`
	RequestModel  string                 `json:"request_model"`
	ResponseModel string                 `json:"response_model"`
	Description   string                 `json:"description"`
	Services      []*loadbalance.Service `json:"services"`
	LBTactic      typ.Tactic             `json:"lb_tactic"`
	Active        bool                   `json:"active"`
	SmartEnabled  bool                   `json:"smart_enabled"`
	SmartRouting  []interface{}          `json:"smart_routing"`
}

// ProviderData represents the provider data (used for both export and import)
type ProviderData struct {
	Type        string           `json:"type"`
	UUID        string           `json:"uuid"`
	Name        string           `json:"name"`
	APIBase     string           `json:"api_base"`
	APIStyle    string           `json:"api_style"`
	AuthType    string           `json:"auth_type"`
	Token       string           `json:"token"`
	OAuthDetail *typ.OAuthDetail `json:"oauth_detail"`
	Enabled     bool             `json:"enabled"`
	ProxyURL    string           `json:"proxy_url"`
	Timeout     int64            `json:"timeout"`
	Tags        []string         `json:"tags"`
	Models      []string         `json:"models"`
}
