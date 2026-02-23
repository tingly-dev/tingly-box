package relay

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/protocol"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for MVP
	},
}

// BotHandler defines the interface for handling messages from relay to bot
type BotHandler interface {
	HandleMessage(sessionID string, data *protocol.MessageData) error
	SessionJoined(sessionID string)
	SessionLeft(sessionID string)
}

// RelayServer manages WebSocket connections and message routing
type RelayServer struct {
	addr       string
	engine     *gin.Engine
	httpServer *http.Server
	sessions   map[string]*Session
	bots       map[string]BotHandler
	mu         sync.RWMutex
	store      MessageStore
	cache      *MessageCache
	ctx        context.Context
	cancel     context.CancelFunc
}

// Config configures a RelayServer
type Config struct {
	Addr      string
	DBPath    string
	CacheSize int
}

// NewRelayServer creates a new relay server
func NewRelayServer(cfg Config) *RelayServer {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger())

	s := &RelayServer{
		addr:     cfg.Addr,
		engine:   engine,
		sessions: make(map[string]*Session),
		bots:     make(map[string]BotHandler),
	}

	// Initialize storage if DBPath provided
	if cfg.DBPath != "" {
		store, err := NewSQLiteStore(cfg.DBPath)
		if err != nil {
			// Log but don't fail - server can work without persistence
		} else {
			s.store = store
		}
	}

	// Initialize cache
	if cfg.CacheSize > 0 {
		s.cache = NewMessageCache(cfg.CacheSize)
	}

	s.setupRoutes()

	return s
}

// setupRoutes sets up the HTTP routes
func (s *RelayServer) setupRoutes() {
	// WebSocket endpoint for chat clients
	s.engine.GET("/ws", s.handleWebSocket)

	// Bot registration and communication
	api := s.engine.Group("/api")
	{
		bot := api.Group("/bot")
		{
			bot.POST("/register", s.registerBot)
			bot.POST("/:botid/send", s.botSend)
		}

		// Session management
		sessions := api.Group("/sessions")
		{
			sessions.GET("", s.listSessions)
			sessions.GET("/:id", s.getSession)
		}

		// Health check
		api.GET("/health", s.healthCheck)
	}
}

// Start starts the relay server
func (s *RelayServer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

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

// Stop stops the relay server
func (s *RelayServer) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}

	// Close all sessions
	s.closeAllSessions()

	// Close storage
	if s.store != nil {
		s.store.Close()
	}

	// Shutdown HTTP server
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}

	return nil
}

// RegisterBot registers a bot handler
func (s *RelayServer) RegisterBot(botID string, handler BotHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bots[botID] = handler
	log.Printf("[Relay] Bot registered: %s (total bots: %d)", botID, len(s.bots))
}

// UnregisterBot unregisters a bot handler
func (s *RelayServer) UnregisterBot(botID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bots, botID)
}

// BroadcastMessage sends a message to all registered bots
func (s *RelayServer) BroadcastMessage(sessionID string, data *protocol.MessageData) {
	s.mu.RLock()
	bots := make([]string, 0, len(s.bots))
	for botID := range s.bots {
		bots = append(bots, botID)
	}
	s.mu.RUnlock()

	if len(bots) == 0 {
		log.Printf("[Relay] No bots registered, message not delivered")
		return
	}

	for _, botID := range bots {
		s.mu.RLock()
		handler := s.bots[botID]
		s.mu.RUnlock()

		if handler != nil {
			log.Printf("[Relay] Broadcasting to bot %s: %s", botID, data.Text)
			go func(bid string, h BotHandler) {
				if err := h.HandleMessage(sessionID, data); err != nil {
					log.Printf("[Relay] Bot %s HandleMessage error: %v", bid, err)
				}
			}(botID, handler)
		}
	}
}

// SendToSession sends a message to a specific session
func (s *RelayServer) SendToSession(sessionID string, data *protocol.MessageData) error {
	s.mu.RLock()
	session, exists := s.sessions[sessionID]
	s.mu.RUnlock()

	if !exists {
		log.Printf("[Relay] Session not found: %s", sessionID)
		return ErrSessionNotFound
	}

	log.Printf("[Relay] Sending to session %s: %s", sessionID, data.Text)
	return session.Send(data)
}

// GetSession returns a session by ID
func (s *RelayServer) GetSession(sessionID string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

// GetAllSessions returns all active sessions
func (s *RelayServer) GetAllSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// SessionCount returns the number of active sessions
func (s *RelayServer) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// closeAllSessions closes all active sessions
func (s *RelayServer) closeAllSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.sessions {
		if err := session.Close(); err != nil {
			// Log error
		}
		delete(s.sessions, id)
	}
}

// healthCheck returns server health status
func (s *RelayServer) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":    "ok",
		"type":      "relay",
		"timestamp": time.Now().Unix(),
		"sessions":  s.SessionCount(),
		"bots":      len(s.bots),
	})
}
