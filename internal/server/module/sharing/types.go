package sharing

import "time"

// --- types ------------------------------------------------------------------

// TokenCreateRequest is the request body for creating a new API token.
type TokenCreateRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
	TeamID      string `json:"team_id,omitempty"`
	// RPMLimit is the key's requests-per-minute budget on /tingly/team.
	// Omitted → db.DefaultSharingKeyRPMLimit (0); 0 → unlimited.
	RPMLimit *int `json:"rpm_limit,omitempty" binding:"omitempty,min=0,max=1000000" description:"Requests per minute allowed for this key; 0 or omitted = unlimited; at most 1000000" example:"60"`
}

// TokenRateLimitRequest sets an existing key's requests-per-minute budget.
// The max mirrors db.MaxSharingKeyRPMLimit (binding tags must be literals).
type TokenRateLimitRequest struct {
	RPMLimit *int `json:"rpm_limit" binding:"required,min=0,max=1000000" description:"Requests per minute allowed for this key; 0 = unlimited; at most 1000000" example:"60"`
}

// TokenListQuery describes the optional filters accepted by GET /tokens.
type TokenListQuery struct {
	UserID  string `form:"user_id" json:"user_id,omitempty"`
	TeamID  string `form:"team_id" json:"team_id,omitempty"`
	Enabled *bool  `form:"enabled" json:"enabled,omitempty"`
	Limit   int    `form:"limit" json:"limit,omitempty"`
	Offset  int    `form:"offset" json:"offset,omitempty"`
}

// TokenCreateResponse is returned after a token is created or regenerated.
type TokenCreateResponse struct {
	Token       string    `json:"token"`
	TokenID     string    `json:"token_id"`
	UserID      string    `json:"user_id"`
	TeamID      string    `json:"team_id"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	RPMLimit    int       `json:"rpm_limit" description:"Requests per minute allowed for this key; 0 = unlimited"`
}

// APITokenInfo represents token metadata (without the raw token string).
type APITokenInfo struct {
	TokenID     string     `json:"token_id"`
	UserID      string     `json:"user_id"`
	TeamID      string     `json:"team_id"`
	DisplayName string     `json:"display_name"`
	Enabled     bool       `json:"enabled"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by,omitempty"`
	RPMLimit    int        `json:"rpm_limit" description:"Requests per minute allowed for this key; 0 = unlimited"`
}

// TokenMoveRequest moves an existing sharing key to another team. The raw key
// remains unchanged, but its authorization scope changes immediately.
type TokenMoveRequest struct {
	TeamID string `json:"team_id" binding:"required"`
}

// TokenListResponse is the response for listing tokens.
type TokenListResponse struct {
	Tokens []APITokenInfo `json:"tokens"`
	Total  int64          `json:"total"`
}
