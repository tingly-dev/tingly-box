package oauth

import (
	"html/template"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers OAuth API routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, authMiddleware gin.HandlerFunc, handler *Handler) {
	// Register HTML templates
	// Note: Templates will be registered by RegisterCallbackRoutes which has access to RouteManager

	// Authenticated API routes
	router.Router.Use(authMiddleware)

	// OAuth Provider Management
	router.GET("/oauth/providers", handler.ListOAuthProviders,
		swagger.WithTags("oauth"),
		swagger.WithDescription("List all available OAuth providers"),
		swagger.WithResponseModel(OAuthProvidersResponse{}),
	)

	router.GET("/oauth/providers/:type", handler.GetOAuthProvider,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Get specific OAuth provider configuration"),
		swagger.WithResponseModel(OAuthProviderDataResponse{}),
	)

	router.PUT("/oauth/providers/:type", handler.UpdateOAuthProvider,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Update OAuth provider configuration"),
		swagger.WithRequestModel(OAuthUpdateProviderRequest{}),
		swagger.WithResponseModel(OAuthUpdateProviderResponse{}),
	)

	router.DELETE("/oauth/providers/:type", handler.DeleteOAuthProvider,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Delete OAuth provider configuration (clears credentials)"),
		swagger.WithResponseModel(OAuthUpdateProviderResponse{}),
	)

	// OAuth Authorization Flow
	router.POST("/oauth/authorize", handler.AuthorizeOAuth,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Initiate OAuth authorization flow"),
		swagger.WithRequestModel(OAuthAuthorizeRequest{}),
		swagger.WithResponseModel(OAuthAuthorizeResponse{}),
	)

	// OAuth Token Management
	router.GET("/oauth/token", handler.GetOAuthToken,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Get OAuth token for a user and provider"),
		swagger.WithResponseModel(OAuthTokenResponse{}),
	)

	router.POST("/oauth/refresh", handler.RefreshOAuthToken,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Refresh OAuth token using refresh token"),
		swagger.WithRequestModel(OAuthRefreshTokenRequest{}),
		swagger.WithResponseModel(OAuthRefreshTokenResponse{}),
	)

	router.DELETE("/oauth/token", handler.RevokeOAuthToken,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Revoke OAuth token for a user and provider"),
		swagger.WithResponseModel(OAuthMessageResponse{}),
	)

	router.GET("/oauth/tokens", handler.ListOAuthTokens,
		swagger.WithTags("oauth"),
		swagger.WithDescription("List all OAuth tokens for a user"),
		swagger.WithResponseModel(OAuthTokensResponse{}),
	)

	// OAuth Session Status
	router.GET("/oauth/status", handler.GetOAuthSessionStatus,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Get OAuth session status"),
		swagger.WithQueryRequired("session_id", "string", "OAuth session ID from authorize response"),
		swagger.WithResponseModel(OAuthSessionStatusResponse{}),
	)

	// OAuth Cancel Session
	router.POST("/oauth/cancel", handler.CancelOAuthSession,
		swagger.WithTags("oauth"),
		swagger.WithDescription("Cancel an in-progress OAuth session and cleanup resources"),
		swagger.WithRequestModel(OAuthCancelRequest{}),
		swagger.WithResponseModel(OAuthMessageResponse{}),
	)
}

// RegisterCallbackRoutes registers unauthenticated callback routes
// These must be registered outside the authenticated API group
func RegisterCallbackRoutes(manager *swagger.RouteManager, handler *Handler) {
	// Register HTML templates
	registerHTMLTemplates(manager)

	// Register callback routes directly on the engine (no auth required)
	manager.GetEngine().GET("/oauth/callback", handler.OAuthCallback)
	manager.GetEngine().GET("/callback", handler.OAuthCallback)
}

// registerHTMLTemplates registers HTML templates for OAuth callback pages
func registerHTMLTemplates(manager *swagger.RouteManager) {
	const oauthSuccessHTML = `
{{ define "oauth_success.html" }}
<!DOCTYPE html>
<html>
<head>
    <title>OAuth Success - Tingly Box</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
        .container { text-align: center; padding: 40px; background: white; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,0.1); }
        .icon { font-size: 64px; margin-bottom: 20px; }
        h1 { color: #10b981; margin: 0 0 10px; }
        p { color: #666; margin: 8px 0; }
        .token { background: #f3f4f6; padding: 12px; border-radius: 6px; font-family: monospace; margin: 20px auto; max-width: 400px; word-break: break-all; }
        .provider-name { background: #e0f2fe; color: #0369a1; padding: 8px 16px; border-radius: 6px; font-weight: 500; margin: 10px auto; max-width: 400px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">✅</div>
        <h1>OAuth Authorization Successful</h1>
        <p><strong>Provider:</strong> {{ .provider }}</p>
        <div class="provider-name">{{ .provider_name }}</div>
        <p><strong>Token Type:</strong> {{ .token_type }}</p>
        <div class="token">{{ .access_token }}</div>
        <p style="font-size: 14px; color: #999;">Provider has been created. You can close this window and return to the application.</p>
    </div>
</body>
</html>
{{ end }}
`

	const oauthErrorHTML = `
{{ define "oauth_error.html" }}
<!DOCTYPE html>
<html>
<head>
    <title>OAuth Error - Tingly Box</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5; }
        .container { text-align: center; padding: 40px; background: white; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,0.1); }
        .icon { font-size: 64px; margin-bottom: 20px; }
        h1 { color: #ef4444; margin: 0 0 10px; }
        p { color: #666; margin: 8px 0; }
        .error { background: #fef2f2; color: #dc2626; padding: 16px; border-radius: 6px; margin: 20px auto; max-width: 500px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">❌</div>
        <h1>OAuth Authorization Failed</h1>
        <div class="error">{{ .error }}</div>
        <p style="font-size: 14px; color: #999;">Please try again or contact support if the issue persists.</p>
    </div>
</body>
</html>
{{ end }}
`

	tmpl := template.Must(template.New("oauth").Parse(oauthSuccessHTML + oauthErrorHTML))
	manager.GetEngine().SetHTMLTemplate(tmpl)
}
