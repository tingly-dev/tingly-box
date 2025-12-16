package server

import (
	"io/fs"
	"net/http"
	"strings"

	assets "tingly-box/internal"

	"github.com/gin-gonic/gin"
)

// EmbeddedAssets handles embedded web assets
type EmbeddedAssets struct{}

// NewEmbeddedAssets creates a new embedded assets handler
func NewEmbeddedAssets() (*EmbeddedAssets, error) {
	return &EmbeddedAssets{}, nil
}

// SetupStaticRoutes sets up static file serving with embedded assets
func (e *EmbeddedAssets) SetupStaticRoutes(router *gin.Engine) {

	// Serve static assets from embedded filesystem
	st, _ := fs.Sub(assets.WebDistAssets, "web/dist/assets")
	router.StaticFS("/assets", http.FS(st))

	router.StaticFile("/vite.svg", "web/dist/vite.svg")

	router.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes - let them return 404s
		path := c.Request.URL.Path
		// Check if this looks like an API route
		if path == "" || strings.HasPrefix(path, "/api/v") || strings.HasPrefix(path, "/v") || strings.HasPrefix(path, "/openai") || strings.HasPrefix(path, "/anthropic") {
			// This looks like an API route, return 404
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "API endpoint not found",
					"type":    "invalid_request_error",
					"code":    "not_found",
				},
			})
			return
		}

		// For all other routes, serve the SPA index.html
		data, err := assets.WebDistAssets.ReadFile("web/dist/index.html")
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}

// HTML renders HTML templates with embedded assets
func (e *EmbeddedAssets) HTML(c *gin.Context, name string, data any) {
	// For SPA, just serve the index.html file directly
	// Ignore the name parameter since we only have one index.html
	c.FileFromFS("web/dist/index.html", http.FS(assets.WebDistAssets))
}
