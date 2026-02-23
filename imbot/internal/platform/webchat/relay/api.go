package relay

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/protocol"
)

// handleWebSocket handles WebSocket upgrade and session creation
func (s *RelayServer) handleWebSocket(c *gin.Context) {
	log.Printf("[Relay] WebSocket connection attempt from %s", c.ClientIP())

	// Check for session resume from query parameter
	requestedSessionID := c.Query("session_id")

	// Get client info
	clientIP := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	var sessionID, senderID, senderName string

	// If session_id provided, try to resume from database
	if requestedSessionID != "" && s.store != nil {
		if sessionInfo, err := s.store.GetSession(requestedSessionID); err == nil && sessionInfo != nil {
			sessionID = sessionInfo.ID
			senderID = sessionInfo.SenderID
			senderName = sessionInfo.SenderName
			log.Printf("[Relay] Resuming session %s for user %s", sessionID, senderID)
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

	log.Printf("[Relay] Creating session %s for user %s", sessionID, senderID)

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Relay] Failed to upgrade WebSocket: %v", err)
		c.JSON(400, gin.H{"error": "Failed to upgrade WebSocket"})
		return
	}

	log.Printf("[Relay] WebSocket upgraded for session %s", sessionID)

	// Create session
	session := NewSession(sessionID, conn, s)
	session.SetSenderInfo(senderID, senderName)
	session.SetClientInfo(&ClientInfo{
		UserAgent:   userAgent,
		IPAddress:   clientIP,
		ConnectTime: time.Now().Unix(),
	})

	// Save/update session in database
	if s.store != nil {
		if err := s.store.CreateOrUpdateSession(sessionID, senderID, senderName, session.clientInfo); err != nil {
			// Log error but don't fail
		}
	}

	// Add session to relay
	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	_ = requestedSessionID != "" && sessionID == requestedSessionID // Track resume for logging

	// Notify bots about session join
	s.BroadcastSessionEvent(sessionID, "join")

	// Start read and write loops
	go session.WriteLoop()
	go session.ReadLoop()

	// Send history after connection established
	go func() {
		time.Sleep(100 * time.Millisecond)
		session.SendHistory(50) // Default history limit
	}()
}

// registerBot handles bot registration
func (s *RelayServer) registerBot(c *gin.Context) {
	var req struct {
		BotID    string `json:"botId" binding:"required"`
		BotToken string `json:"botToken,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// For now, just acknowledge registration
	// In a real implementation, you might validate tokens
	c.JSON(200, gin.H{
		"status":    "registered",
		"botId":     req.BotID,
		"timestamp": time.Now().Unix(),
	})
}

// botSend handles bot sending message to a session
func (s *RelayServer) botSend(c *gin.Context) {
	_ = c.Param("botid") // Bot ID for future authentication

	var req struct {
		SessionID string                `json:"sessionId" binding:"required"`
		Message   *protocol.MessageData `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Send to session
	if err := s.SendToSession(req.SessionID, req.Message); err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}

	// Persist message
	if s.store != nil {
		go s.store.SaveMessage(req.SessionID, req.Message)
	}

	// Update cache
	if s.cache != nil {
		s.cache.Add(req.SessionID, req.Message)
	}

	c.JSON(200, gin.H{
		"status":    "sent",
		"messageId": req.Message.ID,
		"timestamp": time.Now().Unix(),
	})
}

// listSessions returns all active sessions
func (s *RelayServer) listSessions(c *gin.Context) {
	sessions := s.GetAllSessions()

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
func (s *RelayServer) getSession(c *gin.Context) {
	id := c.Param("id")
	session := s.GetSession(id)

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

// BroadcastSessionEvent broadcasts session events to all bots
func (s *RelayServer) BroadcastSessionEvent(sessionID, eventType string) {
	s.mu.RLock()
	bots := make([]string, 0, len(s.bots))
	for botID := range s.bots {
		bots = append(bots, botID)
	}
	s.mu.RUnlock()

	for _, botID := range bots {
		s.mu.RLock()
		handler := s.bots[botID]
		s.mu.RUnlock()

		if handler != nil {
			if eventType == "join" {
				go handler.SessionJoined(sessionID)
			} else if eventType == "leave" {
				go handler.SessionLeft(sessionID)
			}
		}
	}
}

// GenerateSessionID generates a unique session ID using UUID
func GenerateSessionID() string {
	return uuid.New().String()
}
