// Package server is the HTTP shell: it builds the gin engine, mounts every
// module on the swagger-tracked /api/v1 group and serves the embedded web UI.
package server

import (
	"context"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/web"
	"github.com/tingly-dev/tingly-box/swagger"
)

// Version is set at build time.
var Version = "dev"

// New builds the engine with all routes and starts the project runtime's
// background work, which stops when ctx ends.
func New(ctx context.Context, d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	manager := newManager(engine)
	mods, svc := build(d)
	module.Mount(&module.Routes{API: manager.NewGroup("api", "v1", ""), Engine: engine}, mods...)
	manager.RegisterOpenAPIEndpoint("/api/openapi.json", swagger.VersionV3)
	serveUI(engine)
	svc.Start(ctx)
	return engine
}

// OpenAPI renders the OpenAPI v3 document without starting a server.
func OpenAPI(d Deps) (string, error) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	manager := newManager(engine)
	mods, _ := build(d)
	module.Mount(&module.Routes{API: manager.NewGroup("api", "v1", ""), Engine: engine}, mods...)
	return manager.GenerateOpenAPI(swagger.VersionV3)
}

func newManager(engine *gin.Engine) *swagger.RouteManager {
	m := swagger.NewRouteManager(engine)
	m.SetSwaggerInfo(swagger.SwaggerInfo{
		Title:       "Tingly Project API",
		Description: "Projects (coordinator conversation and Claude Code threads), memory, library and chats.",
		Version:     "1.0.0",
		BasePath:    "/",
	})
	return m
}

// HealthResponse reports the server is up.
type HealthResponse struct {
	Status  string `json:"status" example:"ok"`
	Version string `json:"version"`
}

type healthModule struct{}

func (healthModule) RegisterRoutes(rt *module.Routes) {
	rt.API.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthResponse{Status: "ok", Version: Version})
	},
		swagger.WithTags("system"),
		swagger.WithDescription("Health check"),
		swagger.WithResponseModel(HealthResponse{}))
}

// serveUI serves the embedded single-page app, falling back to index.html
// for client-side routes.
func serveUI(engine *gin.Engine) {
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic(err)
	}
	index, indexErr := fs.ReadFile(dist, "index.html")
	files := http.FileServer(http.FS(dist))
	engine.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		if indexErr != nil {
			c.String(http.StatusOK, "tingly-project API is running. The web UI is not built: run `task frontend`, then rebuild.")
			return
		}
		name := strings.TrimPrefix(path, "/")
		if name != "" {
			if st, err := fs.Stat(dist, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
}
