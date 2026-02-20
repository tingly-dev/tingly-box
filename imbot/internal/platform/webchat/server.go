package webchat

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// webchatIndexHTML is the embedded HTML for the web chat interface
const webchatIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WebChat Bot Demo</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; height: 100vh; display: flex; flex-direction: column; }
        .header { background: #2563eb; color: white; padding: 1rem 2rem; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .header h1 { font-size: 1.25rem; font-weight: 600; }
        .header .status { font-size: 0.875rem; opacity: 0.9; margin-top: 0.25rem; }
        .chat-container { flex: 1; max-width: 800px; width: 100%; margin: 0 auto; padding: 1rem; overflow-y: auto; }
        .messages { display: flex; flex-direction: column; gap: 0.75rem; }
        .message { max-width: 70%; padding: 0.75rem 1rem; border-radius: 1rem; word-wrap: break-word; }
        .message.user { align-self: flex-end; background: #2563eb; color: white; border-bottom-right-radius: 0.25rem; }
        .message.bot { align-self: flex-start; background: white; color: #1f2937; border-bottom-left-radius: 0.25rem; box-shadow: 0 1px 2px rgba(0,0,0,0.1); }
        .message .sender { font-size: 0.75rem; opacity: 0.7; margin-bottom: 0.25rem; }
        .message .time { font-size: 0.625rem; opacity: 0.5; margin-top: 0.25rem; }
        .input-container { background: white; padding: 1rem; border-top: 1px solid #e5e7eb; }
        .input-wrapper { max-width: 800px; margin: 0 auto; display: flex; gap: 0.5rem; }
        .input-wrapper input { flex: 1; padding: 0.75rem 1rem; border: 1px solid #e5e7eb; border-radius: 2rem; font-size: 1rem; outline: none; transition: border-color 0.2s; }
        .input-wrapper input:focus { border-color: #2563eb; }
        .input-wrapper button { padding: 0.75rem 1.5rem; background: #2563eb; color: white; border: none; border-radius: 2rem; font-size: 1rem; font-weight: 600; cursor: pointer; transition: background 0.2s; }
        .input-wrapper button:hover { background: #1d4ed8; }
        .input-wrapper button:disabled { background: #9ca3af; cursor: not-allowed; }
        .keyboard { display: flex; flex-wrap: wrap; gap: 0.5rem; margin-top: 0.5rem; }
        .keyboard-button { padding: 0.5rem 1rem; background: #f3f4f6; border: 1px solid #e5e7eb; border-radius: 0.5rem; cursor: pointer; font-size: 0.875rem; transition: all 0.2s; }
        .keyboard-button:hover { background: #e5e7eb; }
        .connecting { display: flex; align-items: center; justify-content: center; padding: 2rem; color: #6b7280; }
        .connecting span { animation: pulse 1.5s ease-in-out infinite; }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
        .hidden { display: none !important; }
    </style>
</head>
<body>
    <div class="header">
        <h1>🤖 WebChat Bot Demo</h1>
        <div class="status" id="status">Connecting...</div>
    </div>
    <div class="chat-container">
        <div class="connecting" id="connecting"><span>Connecting to WebSocket...</span></div>
        <div class="messages hidden" id="messages"></div>
    </div>
    <div class="input-container">
        <div class="input-wrapper">
            <input type="text" id="messageInput" placeholder="Type a message..." disabled>
            <button id="sendButton" disabled>Send</button>
        </div>
        <div class="keyboard" id="keyboard"></div>
    </div>
    <script>
        const wsUrl = 'ws://' + window.location.host + '/ws';
        let ws = null, senderId = 'user_' + Math.random().toString(36).substr(2, 9), senderName = 'User ' + senderId.substr(-4);
        const messagesDiv = document.getElementById('messages'), messageInput = document.getElementById('messageInput'), sendButton = document.getElementById('sendButton'), statusDiv = document.getElementById('status'), connectingDiv = document.getElementById('connecting'), keyboardDiv = document.getElementById('keyboard');
        function connect() {
            ws = new WebSocket(wsUrl);
            ws.onopen = () => { statusDiv.textContent = '✅ Connected'; statusDiv.style.color = '#10b981'; connectingDiv.classList.add('hidden'); messagesDiv.classList.remove('hidden'); messageInput.disabled = false; sendButton.disabled = false; messageInput.focus(); };
            ws.onclose = () => { statusDiv.textContent = '❌ Disconnected'; statusDiv.style.color = '#ef4444'; messageInput.disabled = true; sendButton.disabled = true; keyboardDiv.innerHTML = ''; setTimeout(connect, 3000); };
            ws.onerror = (error) => { console.error('WebSocket error:', error); statusDiv.textContent = '❌ Connection error'; };
            ws.onmessage = (event) => { const data = JSON.parse(event.data); addMessage(data, 'bot'); if (data.metadata && data.metadata.replyMarkup) showKeyboard(data.metadata.replyMarkup); };
        }
        function addMessage(data, type) {
            const messageDiv = document.createElement('div'); messageDiv.className = 'message ' + type;
            const senderDiv = document.createElement('div'); senderDiv.className = 'sender'; senderDiv.textContent = data.senderName || (type === 'user' ? senderName : 'Bot');
            const textDiv = document.createElement('div'); textDiv.textContent = data.text || '';
            const timeDiv = document.createElement('div'); timeDiv.className = 'time'; const timestamp = data.timestamp ? new Date(data.timestamp * 1000) : new Date(); timeDiv.textContent = timestamp.toLocaleTimeString();
            messageDiv.appendChild(senderDiv); messageDiv.appendChild(textDiv); messageDiv.appendChild(timeDiv); messagesDiv.appendChild(messageDiv); messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }
        function showKeyboard(keyboard) {
            keyboardDiv.innerHTML = ''; if (!keyboard.inline_keyboard) return;
            keyboard.inline_keyboard.forEach(row => { row.forEach(button => { const btn = document.createElement('button'); btn.className = 'keyboard-button'; btn.textContent = button.text; btn.onclick = () => { sendMessage(button.callback_data); }; keyboardDiv.appendChild(btn); }); });
        }
        function sendMessage(text) {
            const message = { id: 'msg_' + Date.now(), senderId: senderId, senderName: senderName, text: text, timestamp: Math.floor(Date.now() / 1000) };
            ws.send(JSON.stringify(message)); addMessage(message, 'user'); keyboardDiv.innerHTML = '';
        }
        sendButton.addEventListener('click', () => { const text = messageInput.value.trim(); if (text) { sendMessage(text); messageInput.value = ''; } });
        messageInput.addEventListener('keypress', (e) => { if (e.key === 'Enter') sendButton.click(); });
        connect();
    </script>
</body>
</html>
`

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Make this configurable via config
		return true // Allow all origins for MVP
	},
}

// GinServer wraps Gin HTTP server for WebChat
type GinServer struct {
	bot    *Bot
	addr   string
	engine *gin.Engine
	server *http.Server
	mu     sync.RWMutex
}

// NewGinServer creates a new Gin server
func NewGinServer(addr string, bot *Bot) *GinServer {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger())

	return &GinServer{
		bot:    bot,
		addr:   addr,
		engine: engine,
	}
}

// SetupRoutes sets up the Gin routes
func (s *GinServer) SetupRoutes() {
	// Serve the demo HTML page at root
	s.engine.GET("/", s.handleIndex)

	// WebSocket endpoint
	s.engine.GET("/ws", s.handleWebSocket)

	// API routes
	api := s.engine.Group("/api")
	{
		api.GET("/health", s.healthCheck)
		api.GET("/sessions", s.listSessions)
		api.GET("/sessions/:id", s.getSession)
	}

	// Static files for assets (if needed in future)
	// s.engine.Static("/static", "./static")
}

// handleIndex serves the demo HTML page
func (s *GinServer) handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, webchatIndexHTML)
}

// handleWebSocket handles WebSocket upgrade and session creation
func (s *GinServer) handleWebSocket(c *gin.Context) {
	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.bot.Logger().Error("Failed to upgrade WebSocket: %v", err)
		c.JSON(400, gin.H{"error": "Failed to upgrade WebSocket"})
		return
	}

	// Get client info
	clientIP := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	// Create session
	sessionID := generateSessionID()
	session := NewSession(sessionID, conn, s.bot)
	session.clientInfo = &WebSocketClientInfo{
		UserAgent:   userAgent,
		IPAddress:   clientIP,
		ConnectTime: time.Now().Unix(),
	}

	// Add session to bot
	s.bot.AddSession(session)

	s.bot.Logger().Info("New WebSocket session: %s from %s", sessionID, clientIP)

	// Start read and write loops
	go session.WriteLoop()
	go session.ReadLoop()
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
