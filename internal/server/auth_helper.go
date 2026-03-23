package server

import (
	"github.com/gin-gonic/gin"
)

// Helper methods to get the appropriate auth middleware
// These methods return custom middleware if provided by TBE, otherwise default middleware

// getUserAuthMiddleware returns the user auth middleware to use
func (s *Server) getUserAuthMiddleware() gin.HandlerFunc {
	if s.customUserAuthMiddleware != nil {
		return s.customUserAuthMiddleware
	}
	return s.authMW.UserAuthMiddleware()
}

// getModelAuthMiddleware returns the model auth middleware to use
func (s *Server) getModelAuthMiddleware() gin.HandlerFunc {
	if s.customModelAuthMiddleware != nil {
		return s.customModelAuthMiddleware
	}
	return s.authMW.ModelAuthMiddleware()
}

// getVirtualModelAuthMiddleware returns the virtual model auth middleware to use
// Note: TBE typically doesn't override this, so always use default
func (s *Server) getVirtualModelAuthMiddleware() gin.HandlerFunc {
	return s.authMW.VirtualModelAuthMiddleware()
}
