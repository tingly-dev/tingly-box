package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/audit"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/launcher"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/session"
	"github.com/tingly-dev/tingly-box/cmd/remote-cc/internal/summarizer"
)

func init() {
	gin.SetMode(gin.TestMode)
	logrus.SetLevel(logrus.DebugLevel)
}

func setupTestHandler() *Handler {
	sessionMgr := session.NewManager(session.Config{Timeout: 30 * time.Minute}, nil)
	claudeLauncher := launcher.NewClaudeCodeLauncher()
	summaryEngine := summarizer.NewEngine()
	auditLogger := audit.NewLogger(audit.Config{
		Console:    false,
		MaxEntries: 100,
	})
	return NewHandler(sessionMgr, claudeLauncher, summaryEngine, auditLogger)
}

func TestHandler_Handshake_Success(t *testing.T) {
	handler := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/remote-coder/handshake", nil)
	c.Request = req

	handler.Handshake(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response HandshakeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.SessionID == "" {
		t.Error("Expected session_id to be non-empty")
	}

	if response.Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", response.Status)
	}

	if response.ExpiresAt == "" {
		t.Error("Expected expires_at to be non-empty")
	}
}

func TestHandler_Handshake_WithBody(t *testing.T) {
	handler := setupTestHandler()

	body := HandshakeRequest{}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/remote-coder/handshake", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Handshake(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHandler_Execute_SessionNotFound(t *testing.T) {
	handler := setupTestHandler()

	body := ExecuteRequest{
		SessionID: "non-existent-session",
		Request:   "test request",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/remote-coder/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Execute(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_Execute_InvalidRequest(t *testing.T) {
	handler := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/remote-coder/execute", nil)
	c.Request = req

	handler.Execute(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandler_Status_SessionNotFound(t *testing.T) {
	handler := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/remote-coder/status/non-existent", nil)
	c.Request = req
	c.Params = gin.Params{{Key: "session_id", Value: "non-existent"}}

	handler.Status(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_Status_Success(t *testing.T) {
	handler := setupTestHandler()

	// Create a session first
	session := handler.sessionMgr.Create()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/remote-coder/status/"+session.ID, nil)
	c.Request = req
	c.Params = gin.Params{{Key: "session_id", Value: session.ID}}

	handler.Status(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.SessionID != session.ID {
		t.Errorf("Expected session_id '%s', got '%s'", session.ID, response.SessionID)
	}
}

func TestHandler_Close_SessionNotFound(t *testing.T) {
	handler := setupTestHandler()

	body := CloseRequest{
		SessionID: "non-existent-session",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/remote-coder/close", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Close(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_Close_InvalidRequest(t *testing.T) {
	handler := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/remote-coder/close", nil)
	c.Request = req

	handler.Close(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandler_Close_Success(t *testing.T) {
	handler := setupTestHandler()

	// Create a session first
	session := handler.sessionMgr.Create()

	body := CloseRequest{
		SessionID: session.ID,
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/remote-coder/close", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Close(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response CloseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Status != "closed" {
		t.Errorf("Expected status 'closed', got '%s'", response.Status)
	}
}

func TestHandler_Execute_FullFlow(t *testing.T) {
	handler := setupTestHandler()

	// Create a session
	handshakeBody := HandshakeRequest{}
	handshakeBytes, _ := json.Marshal(handshakeBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/remote-coder/handshake", bytes.NewReader(handshakeBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Handshake(c)

	var handshakeResponse HandshakeResponse
	json.Unmarshal(w.Body.Bytes(), &handshakeResponse)

	// Execute a request
	executeBody := ExecuteRequest{
		SessionID: handshakeResponse.SessionID,
		Request:   "Say hello",
	}
	executeBytes, _ := json.Marshal(executeBody)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	req = httptest.NewRequest("POST", "/remote-coder/execute", bytes.NewReader(executeBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Execute(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var executeResponse ExecuteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &executeResponse); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if executeResponse.SessionID != handshakeResponse.SessionID {
		t.Errorf("Expected session_id '%s', got '%s'", handshakeResponse.SessionID, executeResponse.SessionID)
	}

	// Check status
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	req = httptest.NewRequest("GET", "/remote-coder/status/"+handshakeResponse.SessionID, nil)
	c.Request = req
	c.Params = gin.Params{{Key: "session_id", Value: handshakeResponse.SessionID}}

	handler.Status(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Close session
	closeBody := CloseRequest{
		SessionID: handshakeResponse.SessionID,
	}
	closeBytes, _ := json.Marshal(closeBody)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	req = httptest.NewRequest("POST", "/remote-coder/close", bytes.NewReader(closeBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler.Close(c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}
