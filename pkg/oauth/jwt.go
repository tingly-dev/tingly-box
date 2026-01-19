package oauth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

// =============================================
// JWT ID Token Parsing
// =============================================

// jwtClaims represents the claims section of a JSON Web Token (JWT).
// It includes standard claims like issuer, subject, and expiration time, as well as
// custom claims specific to OpenAI's authentication.
type jwtClaims struct {
	AtHash               string         `json:"at_hash,omitempty"`
	AuthProvider         string         `json:"auth_provider,omitempty"`
	AuthTime             int            `json:"auth_time,omitempty"`
	Email                string         `json:"email"`
	EmailVerified        bool           `json:"email_verified,omitempty"`
	Name                 string         `json:"name,omitempty"`
	CodexAuthInfo        *codexAuthInfo `json:"https://api.openai.com/auth"` // OpenAI namespaced claim (required, no omitempty)
	jwt.RegisteredClaims                // Embedded standard claims
}

// Organizations defines the structure for organization details within the JWT claims.
// It holds information about the user's organization, such as ID, role, and title.
type organizations struct {
	ID        string `json:"id,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
	Role      string `json:"role,omitempty"`
	Title     string `json:"title,omitempty"`
}

// codexAuthInfo contains authentication-related details specific to Codex.
// This includes ChatGPT account information, subscription status, and user/organization IDs.
type codexAuthInfo struct {
	ChatgptAccountID               string          `json:"chatgpt_account_id,omitempty"`
	ChatgptPlanType                string          `json:"chatgpt_plan_type,omitempty"`
	ChatgptSubscriptionActiveStart any             `json:"chatgpt_subscription_active_start,omitempty"`
	ChatgptSubscriptionActiveUntil any             `json:"chatgpt_subscription_active_until,omitempty"`
	ChatgptSubscriptionLastChecked *time.Time      `json:"chatgpt_subscription_last_checked,omitempty"`
	ChatgptUserID                  string          `json:"chatgpt_user_id,omitempty"`
	Groups                         []any           `json:"groups,omitempty"`
	Organizations                  []organizations `json:"organizations,omitempty"`
	UserID                         string          `json:"user_id,omitempty"`
}

// GetAccountID extracts the user's account ID from the JWT claims.
// For OpenAI Codex, this retrieves the ChatGPT account ID from the namespaced claim.
func (c *jwtClaims) GetAccountID() string {
	// Try OpenAI's namespaced claim first
	if c.CodexAuthInfo != nil && c.CodexAuthInfo.ChatgptAccountID != "" {
		return c.CodexAuthInfo.ChatgptAccountID
	}
	// Fallback to subject field (from RegisteredClaims)
	return c.Subject
}

// GetUserEmail extracts the user's email address from the JWT claims.
func (c *jwtClaims) GetUserEmail() string {
	return c.Email
}

// parseIDToken parses a JWT ID token and returns the claims
// For security, tokens should be validated, but this implementation
// extracts claims without signature verification for simplicity.
// The token comes directly from the OAuth server, so basic extraction is acceptable.
func parseIDToken(idToken string) *jwtClaims {
	if idToken == "" {
		return nil
	}

	// Parse the JWT without verification (we trust the token from the OAuth server)
	token, _, err := jwt.NewParser().ParseUnverified(idToken, &jwtClaims{})
	if err != nil {
		logrus.Debugf("Failed to parse ID token: %v", err)
		return nil
	}

	claims, ok := token.Claims.(*jwtClaims)
	if !ok {
		logrus.Debugf("Invalid ID token claims type")
		return nil
	}

	return claims
}
