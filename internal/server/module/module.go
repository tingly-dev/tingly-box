// Package module defines the contract every HTTP module under
// internal/server/module implements, so the host server mounts all of them
// the same way instead of through per-module RegisterRoutes signatures.
package module

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/swagger"
)

// Routes is everything a module may register routes on. The host builds it
// once and hands the same value to every module, so a module picks the group
// that matches the auth and API version it needs rather than receiving
// auth middleware as a parameter.
type Routes struct {
	// Public is /api/v1 without authentication (health, auth validation).
	Public *swagger.RouteGroup
	// V1 is /api/v1 behind user authentication.
	V1 *swagger.RouteGroup
	// V2 is /api/v2 behind user authentication.
	V2 *swagger.RouteGroup
	// UserAuth is the user-authentication middleware already applied to V1
	// and V2, for the few routes that also attach it per-route
	// (swagger.WithMiddleware) so the generated spec records it.
	UserAuth gin.HandlerFunc
	// Engine is for routes that live outside the swagger-managed /api groups
	// (status line, notify hooks) and for callback routes that need their own
	// path layout.
	Engine *gin.Engine
	// Manager creates additional swagger-tracked groups (e.g. unauthenticated
	// OAuth callbacks).
	Manager *swagger.RouteManager
}

// Module is an HTTP feature that can register its routes.
type Module interface {
	RegisterRoutes(rt *Routes)
}

// Mount registers each module in order. Nil modules are skipped so callers can
// pass optional modules (built only when their dependencies exist) directly.
func Mount(rt *Routes, mods ...Module) {
	for _, m := range mods {
		if m == nil {
			continue
		}
		m.RegisterRoutes(rt)
	}
}
