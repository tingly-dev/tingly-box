// Package module defines the contract every HTTP module under
// internal/server/module implements, mirroring tingly-box: the host builds one
// Routes value and every module registers on it the same way.
package module

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/swagger"
)

// Routes is everything a module may register routes on.
type Routes struct {
	// API is /api/v1. tingly-project is a local single-user app bound to
	// loopback by default, so the group carries no user auth.
	API *swagger.RouteGroup
	// Engine is for routes outside the swagger-managed group.
	Engine *gin.Engine
}

// Module is an HTTP feature that can register its routes.
type Module interface {
	RegisterRoutes(rt *Routes)
}

// Mount registers each module in order, skipping nil modules.
func Mount(rt *Routes, mods ...Module) {
	for _, m := range mods {
		if m == nil {
			continue
		}
		m.RegisterRoutes(rt)
	}
}
