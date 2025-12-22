package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS returns a CORS middleware handler
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// CORSWithConfig returns a CORS middleware handler with custom configuration
func CORSWithConfig(config CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set allowed origins
		if config.AllowOrigins != "" {
			c.Header("Access-Control-Allow-Origin", config.AllowOrigins)
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}

		// Set allowed methods
		if config.AllowMethods != "" {
			c.Header("Access-Control-Allow-Methods", config.AllowMethods)
		} else {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}

		// Set allowed headers
		if config.AllowHeaders != "" {
			c.Header("Access-Control-Allow-Headers", config.AllowHeaders)
		} else {
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		}

		// Set exposed headers if provided
		if config.ExposeHeaders != "" {
			c.Header("Access-Control-Expose-Headers", config.ExposeHeaders)
		}

		// Set max age if provided
		if config.MaxAge > 0 {
			c.Header("Access-Control-Max-Age", string(rune(config.MaxAge)))
		}

		// Handle preflight requests
		if config.HandlePreflight && c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// CORSConfig defines the configuration for CORS middleware
type CORSConfig struct {
	AllowOrigins    string
	AllowMethods    string
	AllowHeaders    string
	ExposeHeaders   string
	MaxAge          int
	HandlePreflight bool
}
