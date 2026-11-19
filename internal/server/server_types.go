package server

// =============================================
// Token Management Models
// =============================================

// GenerateTokenRequest represents the request to generate a token
type GenerateTokenRequest struct {
	ClientID string `json:"client_id" binding:"required" description:"Client ID for token generation" example:"user123"`
}
