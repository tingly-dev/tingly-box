package server

// DEPRECATED: This file has been refactored and moved to internal/server/module/oauth
// All OAuth handler functionality has been extracted to the oauth module.
// Type aliases are now provided in oauth_compat.go for backward compatibility.
//
// The new module structure:
// - internal/server/module/oauth/handler.go - Handler with all OAuth methods
// - internal/server/module/oauth/routes.go - Route registration
// - internal/server/module/oauth/types.go - Request/Response types
// - internal/server/module/oauth/session.go - Session management
//
// See webui.go for how the new module is integrated:
//   oauthHandler := oauthmodule.NewHandler(s.oauthManager, s.config, s.logger)
//   oauthmodule.RegisterRoutes(apiV1, s.authMW.UserAuthMiddleware(), oauthHandler)
//   oauthmodule.RegisterCallbackRoutes(manager, oauthHandler)
