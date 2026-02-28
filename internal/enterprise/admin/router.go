package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"github.com/tingly-dev/tingly-box/internal/enterprise/rbac"
	"github.com/tingly-dev/tingly-box/internal/enterprise/token"
	"github.com/tingly-dev/tingly-box/internal/enterprise/user"
)

// Router configures and returns the enterprise admin router
type Router struct {
	handler         *Handler
	authMiddleware  gin.HandlerFunc
	enterpriseGroup *gin.RouterGroup
}

// NewRouter creates a new enterprise admin router
func NewRouter(
	parentRouter *gin.RouterGroup,
	authSvc *auth.AuthService,
	userSvc user.Service,
	tokenSvc token.Service,
	auditRepo db.AuditLogRepository,
	authMiddleware gin.HandlerFunc,
) *Router {
	handler := NewHandler(authSvc, userSvc, tokenSvc, auditRepo)

	// Create /enterprise prefix group
	enterpriseGroup := parentRouter.Group("/enterprise")

	return &Router{
		handler:         handler,
		authMiddleware:  authMiddleware,
		enterpriseGroup: enterpriseGroup,
	}
}

// RegisterRoutes registers all enterprise admin routes
func (r *Router) RegisterRoutes() {
	// API version 1 group
	v1 := r.enterpriseGroup.Group("/api/v1")
	r.handler.RegisterRoutes(v1, r.authMiddleware)

	// Admin API group (requires admin role)
	admin := r.enterpriseGroup.Group("/admin")
	admin.Use(r.authMiddleware, rbac.RequireRole(db.RoleAdmin))
	{
		// Admin-specific routes can be added here
		// Most routes are already in /api/v1
	}
}
