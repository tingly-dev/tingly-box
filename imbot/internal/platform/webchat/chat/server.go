package chat

import (
	"context"
	"embed"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed index.html
var defaultHTML embed.FS

// ChatServer serves the frontend chat UI
type ChatServer struct {
	addr          string
	relayAddr     string
	engine        *gin.Engine
	httpServer    *http.Server
	customHTMLDir string
}

// Config configures a ChatServer
type Config struct {
	Addr          string
	RelayAddr     string
	CustomHTMLDir string
}

// NewChatServer creates a new chat server
func NewChatServer(cfg Config) *ChatServer {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger())

	s := &ChatServer{
		addr:          cfg.Addr,
		relayAddr:     cfg.RelayAddr,
		engine:        engine,
		customHTMLDir: cfg.CustomHTMLDir,
	}

	s.setupRoutes()

	return s
}

// setupRoutes sets up the HTTP routes
func (s *ChatServer) setupRoutes() {
	if s.customHTMLDir != "" {
		// Serve custom HTML files from directory
		fileServer := http.FileServer(http.Dir(s.customHTMLDir))
		s.engine.GET("/", func(c *gin.Context) {
			indexPath := s.customHTMLDir + "/index.html"
			if _, err := os.Stat(indexPath); err == nil {
				c.File(indexPath)
			} else {
				fileServer.ServeHTTP(c.Writer, c.Request)
			}
		})
		// SPA fallback
		s.engine.NoRoute(func(c *gin.Context) {
			if !strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.File(s.customHTMLDir + "/index.html")
				return
			}
			c.Status(404)
		})
	} else {
		// Serve embedded demo HTML
		s.engine.GET("/", s.handleIndex)
	}

	// Config endpoint for frontend
	s.engine.GET("/api/config", s.handleConfig)
}

// handleIndex serves the demo HTML page
func (s *ChatServer) handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")

	content, err := defaultHTML.ReadFile("index.html")
	if err != nil {
		c.String(500, "Failed to load page")
		return
	}

	// Inject relay address into HTML
	// If relayAddr starts with :, prepend localhost for valid URL
	relayAddr := s.relayAddr
	if strings.HasPrefix(relayAddr, ":") {
		relayAddr = "localhost" + relayAddr
	}

	html := string(content)
	html = strings.Replace(html, "{{.RelayAddr}}", relayAddr, 1)

	c.Data(200, "text/html; charset=utf-8", []byte(html))
}

// handleConfig returns the configuration for the frontend
func (s *ChatServer) handleConfig(c *gin.Context) {
	// Format relay address for frontend use
	relayAddr := s.relayAddr
	if strings.HasPrefix(relayAddr, ":") {
		relayAddr = "localhost" + relayAddr
	}

	c.JSON(200, gin.H{
		"relayAddr": relayAddr,
	})
}

// Start starts the chat server
func (s *ChatServer) Start(ctx context.Context) error {
	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: s.engine,
	}

	// Start server in background
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error
		}
	}()

	return nil
}

// Stop stops the chat server
func (s *ChatServer) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Addr returns the server address
func (s *ChatServer) Addr() string {
	return s.addr
}

// RelayAddr returns the relay server address
func (s *ChatServer) RelayAddr() string {
	return s.relayAddr
}
