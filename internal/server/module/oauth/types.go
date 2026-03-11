package oauth

// =============================================
// OAuth Provider Models
// =============================================

// OAuthProviderInfo represents OAuth provider information
type OAuthProviderInfo struct {
	Type        string   `json:"type" example:"anthropic"`
	DisplayName string   `json:"display_name" example:"Anthropic Claude"`
	AuthURL     string   `json:"auth_url,omitempty" example:"https://claude.ai/oauth/authorize"`
	Scopes      []string `json:"scopes,omitempty" example:"api"`
	Configured  bool     `json:"configured" example:"true"`
}

// OAuthProvidersResponse represents the response for listing OAuth providers
type OAuthProvidersResponse struct {
	Success bool                `json:"success" example:"true"`
	Data    []OAuthProviderInfo `json:"data"`
}

// OAuthProviderDataResponse represents a single provider data response
type OAuthProviderDataResponse struct {
	Success bool              `json:"success" example:"true"`
	Data    OAuthProviderInfo `json:"data"`
}

// =============================================
// OAuth Authorize Models
// =============================================

// OAuthAuthorizeRequest represents the request to initiate OAuth flow
type OAuthAuthorizeRequest struct {
	Provider     string `json:"provider" binding:"required" description:"OAuth provider type" example:"anthropic"`
	UserID       string `json:"user_id" description:"User ID for the OAuth flow" example:"user123"`
	Redirect     string `json:"redirect" description:"URL to redirect after OAuth completion" example:"http://localhost:3000/callback"`
	ResponseType string `json:"response_type" description:"Response type: 'redirect' or 'json'" example:"json"`
	Name         string `json:"name" description:"Custom name for the provider (optional, auto-generated if empty)" example:"my-claude-account"`
	ProxyURL     string `json:"proxy_url,omitempty" description:"HTTP/SOCKS proxy URL (e.g., http://127.0.0.1:7890 or socks5://127.0.0.1:1080)" example:"http://proxy.example.com:8080"`
}

// OAuthAuthorizeResponse represents the response for OAuth authorization initiation
type OAuthAuthorizeResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty" example:"Authorization initiated"`
	Data    struct {
		AuthURL   string `json:"auth_url,omitempty" example:"https://claude.ai/oauth/authorize?..."`
		State     string `json:"state,omitempty" example:"random_state_string"`
		SessionID string `json:"session_id,omitempty" example:"abc123def456"` // For status tracking
		// Device code flow fields
		DeviceCode              string `json:"device_code,omitempty" example:"MN-12345678-abcdef"`
		UserCode                string `json:"user_code,omitempty" example:"ABCD-EFGH"`
		VerificationURI         string `json:"verification_uri,omitempty" example:"https://chat.qwen.ai/activate"`
		VerificationURIComplete string `json:"verification_uri_complete,omitempty" example:"https://chat.qwen.ai/activate?user_code=ABCD-EFGH"`
		ExpiresIn               int64  `json:"expires_in,omitempty" example:"1800"`
		Interval                int64  `json:"interval,omitempty" example:"5"`
		Provider                string `json:"provider,omitempty" example:"qwen_code"`
	} `json:"data"`
}

// =============================================
// OAuth Token Models
// =============================================

// TokenInfo represents OAuth token information
type TokenInfo struct {
	Provider  string `json:"provider" example:"anthropic"`
	Valid     bool   `json:"valid" example:"true"`
	ExpiresAt string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
}

// OAuthTokenResponse represents the OAuth token response
type OAuthTokenResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		AccessToken  string `json:"access_token" example:"sk-ant-..."`
		RefreshToken string `json:"refresh_token,omitempty" example:"refresh_..."`
		TokenType    string `json:"token_type" example:"Bearer"`
		ExpiresAt    string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
		Provider     string `json:"provider" example:"anthropic"`
		Valid        bool   `json:"valid" example:"true"`
	} `json:"data"`
}

// OAuthTokensResponse represents the response for listing all user tokens
type OAuthTokensResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    []TokenInfo `json:"data"`
}

// =============================================
// OAuth Refresh Token Models
// =============================================

// OAuthRefreshTokenRequest represents the request to refresh an OAuth token
type OAuthRefreshTokenRequest struct {
	ProviderUUID string `json:"provider_uuid" binding:"required" description:"Provider UUID to refresh token for" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// OAuthRefreshTokenResponse represents the response for refreshing an OAuth token
type OAuthRefreshTokenResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty" example:"Token refreshed successfully"`
	Data    struct {
		ProviderUUID string `json:"provider_uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
		AccessToken  string `json:"access_token" example:"sk-ant-..."`
		RefreshToken string `json:"refresh_token,omitempty" example:"refresh_..."`
		TokenType    string `json:"token_type" example:"Bearer"`
		ExpiresAt    string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
		ProviderType string `json:"provider_type" example:"claude_code"`
	} `json:"data"`
}

// =============================================
// OAuth Config Models
// =============================================

// OAuthUpdateProviderRequest represents the request to update OAuth provider config
type OAuthUpdateProviderRequest struct {
	ClientID     string `json:"client_id" binding:"required" description:"OAuth client ID" example:"your_client_id"`
	ClientSecret string `json:"client_secret" description:"OAuth client secret" example:"your_client_secret"`
	RedirectURL  string `json:"redirect_url" description:"OAuth redirect URI" example:"http://localhost:12580/oauth/callback"`
}

// OAuthUpdateProviderResponse represents the response for updating provider config
type OAuthUpdateProviderResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Provider configuration updated"`
	Type    string `json:"type,omitempty" example:"anthropic"`
}

// =============================================
// OAuth Session Models
// =============================================

// OAuthSessionStatusResponse represents the session status check response
type OAuthSessionStatusResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		SessionID    string `json:"session_id" example:"abc123def456"`
		Status       string `json:"status" example:"success"`
		ProviderUUID string `json:"provider_uuid,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
		Error        string `json:"error,omitempty" example:"Authorization failed"`
	} `json:"data"`
}

// OAuthCancelRequest represents the request to cancel an OAuth session
type OAuthCancelRequest struct {
	SessionID string `json:"session_id" binding:"required" description:"Session ID to cancel" example:"abc123def456"`
}

// =============================================
// OAuth Common Response Models
// =============================================

// OAuthErrorResponse represents a standard error response
type OAuthErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Error   string `json:"error" example:"Error message"`
}

// OAuthMessageResponse represents a simple success message response
type OAuthMessageResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Operation successful"`
}

// =============================================
// OAuth Device Code Flow Models
// =============================================

// OAuthDeviceCodeResponse represents the response for device code flow initiation
type OAuthDeviceCodeResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty" example:"Device code flow initiated"`
	Data    struct {
		DeviceCode              string `json:"device_code" example:"MN-12345678-abcdef"`
		UserCode                string `json:"user_code" example:"ABCD-EFGH"`
		VerificationURI         string `json:"verification_uri" example:"https://chat.qwen.ai/activate"`
		VerificationURIComplete string `json:"verification_uri_complete,omitempty" example:"https://chat.qwen.ai/activate?user_code=ABCD-EFGH"`
		ExpiresIn               int64  `json:"expires_in" example:"1800"`
		Interval                int64  `json:"interval" example:"5"`
		Provider                string `json:"provider" example:"qwen_code"`
	} `json:"data"`
}

// OAuthCallbackDataResponse represents the OAuth callback response with token data
type OAuthCallbackDataResponse struct {
	Success      bool   `json:"success" example:"true"`
	AccessToken  string `json:"access_token,omitempty" example:"sk-ant-..."`
	RefreshToken string `json:"refresh_token,omitempty" example:"refresh_..."`
	TokenType    string `json:"token_type,omitempty" example:"Bearer"`
	ExpiresAt    string `json:"expires_at,omitempty" example:"2024-01-01T12:00:00Z"`
	Provider     string `json:"provider,omitempty" example:"anthropic"`
}
