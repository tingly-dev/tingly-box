package webchat

import (
	"context"
	"embed"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

//go:embed index.html
var indexHTML embed.FS

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Make this configurable via config
		return true // Allow all origins for MVP
	},
}

// RouteSetupFunc is a callback for registering custom gin routes.
// It's called after default routes are set up.
type RouteSetupFunc func(*gin.Engine)

// GinServer wraps Gin HTTP server for WebChat
type GinServer struct {
	bot           *Bot
	addr          string
	engine        *gin.Engine
	server        *http.Server
	mu            sync.RWMutex
	customHTMLDir string         // Custom HTML directory path
	routeSetup    RouteSetupFunc // Custom route setup callback
}

// GinServerOption configures a GinServer
type GinServerOption func(*GinServer)

// WithHTMLPath sets a custom directory to serve HTML files from.
// If set, the root path "/" will serve files from this directory instead of the embedded index.html.
func WithHTMLPath(path string) GinServerOption {
	return func(s *GinServer) {
		s.customHTMLDir = path
	}
}

// WithRouteSetupFunc sets a callback for registering custom gin routes.
// The callback is invoked after default routes are set up.
func WithRouteSetupFunc(fn RouteSetupFunc) GinServerOption {
	return func(s *GinServer) {
		s.routeSetup = fn
	}
}

// NewGinServer creates a new Gin server
func NewGinServer(addr string, bot *Bot, opts ...GinServerOption) *GinServer {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger())

	s := &GinServer{
		bot:    bot,
		addr:   addr,
		engine: engine,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// SetupRoutes sets up the Gin routes
func (s *GinServer) SetupRoutes() {
	// Serve the HTML page at root
	if s.customHTMLDir != "" {
		// Serve custom HTML files from directory
		// Use FileServer with a wrapper to handle SPA fallback
		fileServer := http.FileServer(http.Dir(s.customHTMLDir))
		s.engine.GET("/", func(c *gin.Context) {
			// Try to serve index.html for root
			indexPath := s.customHTMLDir + "/index.html"
			if _, err := os.Stat(indexPath); err == nil {
				c.File(indexPath)
			} else {
				// Fallback to file server behavior
				fileServer.ServeHTTP(c.Writer, c.Request)
			}
		})
		// Serve static files from the custom directory
		s.engine.NoRoute(func(c *gin.Context) {
			// For SPA routing, serve index.html for non-API routes
			if !strings.HasPrefix(c.Request.URL.Path, "/api") && !strings.HasPrefix(c.Request.URL.Path, "/ws") {
				c.File(s.customHTMLDir + "/index.html")
				return
			}
			c.Status(404)
		})
	} else {
		// Serve embedded demo HTML page
		s.engine.GET("/", s.handleIndex)
	}

	// WebSocket endpoint
	s.engine.GET("/ws", s.handleWebSocket)

	// API routes
	api := s.engine.Group("/api")
	{
		api.GET("/health", s.healthCheck)
		api.GET("/sessions", s.listSessions)
		api.GET("/sessions/:id", s.getSession)
	}

	// Call custom route setup if provided
	if s.routeSetup != nil {
		s.routeSetup(s.engine)
	}
}

// handleIndex serves the demo HTML page
func (s *GinServer) handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")

	content, err := indexHTML.ReadFile("index.html")
	if err != nil {
		c.String(500, "Failed to load page")
		return
	}

	c.Data(200, "text/html; charset=utf-8", content)
}

// handleWebSocket handles WebSocket upgrade and session creation
// Supports session resume via ?session_id= query parameter
func (s *GinServer) handleWebSocket(c *gin.Context) {
	// Check for session resume from query parameter
	requestedSessionID := c.Query("session_id")

	// Get client info
	clientIP := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	var sessionID, senderID, senderName string

	// If session_id provided, try to resume from database
	if requestedSessionID != "" && s.bot.Store() != nil {
		if sessionInfo, err := s.bot.Store().GetSession(requestedSessionID); err == nil && sessionInfo != nil {
			sessionID = sessionInfo.ID
			senderID = sessionInfo.SenderID
			senderName = sessionInfo.SenderName
			s.bot.Logger().Info("Resuming session: %s for user %s", sessionID, senderID)
		}
	}

	// Generate new session if resume failed or not requested
	if sessionID == "" {
		sessionID = generateSessionID()
	}
	if senderID == "" {
		senderID = generateUserID()
	}
	if senderName == "" {
		senderName = "User"
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.bot.Logger().Error("Failed to upgrade WebSocket: %v", err)
		c.JSON(400, gin.H{"error": "Failed to upgrade WebSocket"})
		return
	}

	// Create session
	clientInfo := &WebSocketClientInfo{
		UserAgent:   userAgent,
		IPAddress:   clientIP,
		ConnectTime: time.Now().Unix(),
	}

	session := NewSession(sessionID, conn, s.bot)
	session.SetSenderInfo(senderID, senderName)
	session.clientInfo = clientInfo

	// Save/update session in database
	if s.bot.Store() != nil {
		if err := s.bot.Store().CreateOrUpdateSession(sessionID, senderID, senderName, clientInfo); err != nil {
			s.bot.Logger().Error("Failed to save session: %v", err)
		}
	}

	// Add session to bot
	s.bot.AddSession(session)

	isResume := requestedSessionID != "" && sessionID == requestedSessionID
	s.bot.Logger().Info("WebSocket session: %s from %s (resume: %v)", sessionID, clientIP, isResume)

	// Start read and write loops
	go session.WriteLoop()
	go session.ReadLoop()

	// Send history after connection established (in a goroutine to not block)
	go func() {
		time.Sleep(100 * time.Millisecond) // Brief delay to ensure connection is ready
		historyLimit := s.bot.Config().GetOptionInt("historyLimit", 50)
		if historyLimit > 0 {
			if err := session.SendHistory(historyLimit); err != nil {
				s.bot.Logger().Error("Failed to send history: %v", err)
			}
		}
	}()
}

// healthCheck returns server health status
func (s *GinServer) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":    "ok",
		"platform":  "webchat",
		"timestamp": time.Now().Unix(),
	})
}

// listSessions returns all active sessions
func (s *GinServer) listSessions(c *gin.Context) {
	sessions := s.bot.GetAllSessions()

	result := make([]gin.H, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, gin.H{
			"id":         session.ID,
			"senderId":   session.SenderID(),
			"senderName": session.SenderName(),
			"createdAt":  session.CreatedAt(),
			"lastActive": session.LastActive(),
			"clientInfo": session.clientInfo,
		})
	}

	c.JSON(200, gin.H{
		"sessions": result,
		"count":    len(result),
	})
}

// getSession returns a specific session
func (s *GinServer) getSession(c *gin.Context) {
	id := c.Param("id")
	session := s.bot.GetSession(id)

	if session == nil {
		c.JSON(404, gin.H{"error": "Session not found"})
		return
	}

	c.JSON(200, gin.H{
		"id":         session.ID,
		"senderId":   session.SenderID(),
		"senderName": session.SenderName(),
		"createdAt":  session.CreatedAt(),
		"lastActive": session.LastActive(),
		"clientInfo": session.clientInfo,
	})
}

// Start starts the HTTP server
func (s *GinServer) Start(ctx context.Context) error {
	s.server = &http.Server{
		Addr:    s.addr,
		Handler: s.engine,
	}

	// Start server in background
	go func() {
		s.bot.Logger().Info("WebChat server listening on %s", s.addr)
		s.bot.Logger().Info("Web UI: http://localhost%s | WebSocket: ws://localhost%s/ws", s.addr, s.addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.bot.Logger().Error("Server error: %v", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the server
func (s *GinServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	s.bot.Logger().Info("Shutting down WebChat server")

	// Close all sessions
	s.bot.CloseAllSessions()

	// Shutdown server
	return s.server.Shutdown(ctx)
}

// Addr returns the server address
func (s *GinServer) Addr() string {
	return s.addr
}

// Engine returns the Gin engine (for testing)
func (s *GinServer) Engine() *gin.Engine {
	return s.engine
}
