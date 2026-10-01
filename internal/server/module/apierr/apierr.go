// Package apierr provides the shared JSON error-response writers for module
// HTTP handlers, so packages that must avoid importing internal/server can
// depend on error-response formatting without reintroducing that cycle.
//
// Three wire shapes are in use and the web UI depends on each, so the shapes
// are kept as separate, explicitly named writers rather than collapsed into
// one:
//
//	Send     {"error": {"message": ..., "type": ...}}   OpenAI-style, for gateway-flavoured endpoints
//	Message  {"error": "..."}                           bare message
//	Failure  {"success": false, "error": "..."}         envelope used by management APIs
package apierr

import "github.com/gin-gonic/gin"

// Writer is the common signature of Message and Failure. Helpers that must
// report an error in a caller-chosen shape (e.g. bind.JSON) take one.
type Writer func(c *gin.Context, status int, msg string)

// Send writes a {"error": {"message", "type"}} JSON response.
func Send(c *gin.Context, status int, err error, errType string) {
	c.JSON(status, gin.H{"error": gin.H{"message": err.Error(), "type": errType}})
}

// Message writes a {"error": "<msg>"} JSON response.
func Message(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// Failure writes a {"success": false, "error": "<msg>"} JSON response.
func Failure(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"success": false, "error": msg})
}
