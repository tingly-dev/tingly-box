package github

import tbgithub "github.com/tingly-dev/tingly-box/internal/github"

// ConnectRequest starts the App Manifest flow.
type ConnectRequest struct {
	// OwnerType is where the App is created: "user" (default) or "org".
	OwnerType string `json:"owner_type"`
	// Org is the organization login when OwnerType is "org".
	Org string `json:"org"`
	// BaseURL is the address the browser reaches this server at
	// (window.location.origin); GitHub redirects back to it.
	BaseURL string `json:"base_url" binding:"required"`
}

// ConnectResponse is what the browser posts to GitHub: a form POST to
// ActionURL with the Manifest in a field named "manifest".
type ConnectResponse = tbgithub.ConnectPlan

// StatusResponse is the connection shown on the settings page.
type StatusResponse = tbgithub.ConnectionStatus

// PATRequest stores a fine-grained personal access token as the fallback.
type PATRequest struct {
	Token string `json:"token" binding:"required"`
}

// OKResponse acknowledges a change.
type OKResponse struct {
	OK bool `json:"ok"`
}
