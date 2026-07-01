package sharing

import "time"

// --- types ------------------------------------------------------------------

// TokenCreateRequest is the request body for creating a new API token.
type TokenCreateRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
}

// TokenCreateResponse is returned after a token is created or regenerated.
type TokenCreateResponse struct {
	Token       string    `json:"token"`
	TokenID     string    `json:"token_id"`
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// APITokenInfo represents token metadata (without the raw token string).
type APITokenInfo struct {
	TokenID     string     `json:"token_id"`
	UserID      string     `json:"user_id"`
	DisplayName string     `json:"display_name"`
	Enabled     bool       `json:"enabled"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by,omitempty"`
}

// TokenListResponse is the response for listing tokens.
type TokenListResponse struct {
	Tokens []APITokenInfo `json:"tokens"`
	Total  int64          `json:"total"`
}
