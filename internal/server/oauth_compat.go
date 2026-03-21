package server

import (
	"github.com/tingly-dev/tingly-box/internal/server/module/oauth"
)

// OAuth type aliases for backward compatibility
// These allow existing code to continue using the old type names
// while the actual implementation is now in the oauth module
type OAuthProviderInfo = oauth.OAuthProviderInfo
type OAuthProvidersResponse = oauth.OAuthProvidersResponse
type OAuthProviderDataResponse = oauth.OAuthProviderDataResponse
type OAuthAuthorizeRequest = oauth.OAuthAuthorizeRequest
type OAuthAuthorizeResponse = oauth.OAuthAuthorizeResponse
type TokenInfo = oauth.TokenInfo
type OAuthTokenResponse = oauth.OAuthTokenResponse
type OAuthTokensResponse = oauth.OAuthTokensResponse
type OAuthRefreshTokenRequest = oauth.OAuthRefreshTokenRequest
type OAuthRefreshTokenResponse = oauth.OAuthRefreshTokenResponse
type OAuthUpdateProviderRequest = oauth.OAuthUpdateProviderRequest
type OAuthUpdateProviderResponse = oauth.OAuthUpdateProviderResponse
type OAuthSessionStatusResponse = oauth.OAuthSessionStatusResponse
type OAuthCancelRequest = oauth.OAuthCancelRequest
type OAuthErrorResponse = oauth.OAuthErrorResponse
type OAuthMessageResponse = oauth.OAuthMessageResponse
type OAuthDeviceCodeResponse = oauth.OAuthDeviceCodeResponse
type OAuthCallbackDataResponse = oauth.OAuthCallbackDataResponse
