// Package imbotsettings provides handlers for ImBot settings management.
package imbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// WeChatQRLoginHandler handles Weixin QR code login flow
type WeChatQRLoginHandler struct {
	settingsStore *db.ImBotSettingsStore
	sessions      map[string]*qrSession
	mu            sync.RWMutex
	// Rate limiting: track recent QR requests per bot
	rateLimitMap map[string][]time.Time // botUUID -> request timestamps
	rateLimitMu  sync.RWMutex
}

// Rate limit constants
const (
	maxQRRequestsPerMinute = 5 // Max 5 QR requests per minute per bot
	rateLimitWindow        = 1 * time.Minute
)

type qrSession struct {
	botUUID   string
	qrID      string
	qrURL     string
	qrData    string
	botType   string
	startedAt time.Time
	// Bot metadata for deferred creation (when bot doesn't exist in DB yet)
	botName     string
	botPlatform string
}

type wechatQRClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewWeChatQRLoginHandler creates a new Weixin QR login handler
func NewWeChatQRLoginHandler(settingsStore *db.ImBotSettingsStore) *WeChatQRLoginHandler {
	return &WeChatQRLoginHandler{
		settingsStore: settingsStore,
		sessions:      make(map[string]*qrSession),
		rateLimitMap:  make(map[string][]time.Time),
	}
}

// QRStartRequest is the request to start QR login
type QRStartRequest struct {
	BotUUID     string `json:"bot_uuid" binding:"required"`
	BotType     string `json:"bot_type,omitempty"`     // Optional bot type (default: "3")
	BotName     string `json:"bot_name,omitempty"`     // Optional: bot display name (for deferred creation)
	BotPlatform string `json:"bot_platform,omitempty"` // Optional: platform (for deferred creation)
}

// QRStartData is the data for QR start response
type QRStartData struct {
	QrCodeID   string `json:"qrcode_id"`
	QrCodeData string `json:"qrcode_data"`
	ExpiresIn  int    `json:"expires_in"`
}

// QRStartResponse is the response for QR start
type QRStartResponse struct {
	Success bool        `json:"success"`
	Data    QRStartData `json:"data"`
	Error   string      `json:"error,omitempty"`
}

// QRStatusData is the data for QR status response
type QRStatusData struct {
	Status  string `json:"status"`             // wait, scaned, confirmed, expired
	BotUUID string `json:"bot_uuid,omitempty"` // Real bot UUID after confirmed (may differ from session UUID for new bots)
}

// QRStatusResponse is the response for QR status
type QRStatusResponse struct {
	Success bool         `json:"success"`
	Data    QRStatusData `json:"data,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// QRStart initiates the QR code login flow
func (h *WeChatQRLoginHandler) QRStart(c *gin.Context) {
	var req QRStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	botUUID := c.Param("uuid")
	if botUUID != req.BotUUID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID mismatch"})
		return
	}

	// Default bot type to "3" (官方小程序机器人)
	botType := req.BotType
	if botType == "" {
		botType = "3"
	}

	// Validate bot existence if not a temporary UUID (for new bot creation)
	// Temp UUIDs start with "temp-" for deferred bot creation
	if !strings.HasPrefix(botUUID, "temp-") {
		existing, err := h.settingsStore.GetSettingsByUUID(botUUID)
		if err != nil {
			logrus.WithError(err).WithField("bot", botUUID).Error("Failed to check bot existence")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate bot"})
			return
		}
		if existing.UUID == "" {
			logrus.WithField("bot", botUUID).Warn("Bot not found for QR login")
			c.JSON(http.StatusNotFound, gin.H{"error": "Bot not found"})
			return
		}
	}

	// Check rate limit
	if !h.checkRateLimit(botUUID) {
		logrus.WithField("bot", botUUID).Warn("QR request rate limit exceeded")
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":       "Too many QR code requests. Please wait a moment before trying again.",
			"retry_after": int(rateLimitWindow.Seconds()),
		})
		return
	}

	// Create QR code client
	client := &wechatQRClient{
		baseURL: "https://ilinkai.weixin.qq.com",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Fetch QR code
	qrResp, err := client.GetBotQRCode(c.Request.Context(), botType)
	if err != nil {
		logrus.WithError(err).WithField("bot", botUUID).Error("Failed to get QR code")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get QR code"})
		return
	}

	// Store session
	h.mu.Lock()
	h.sessions[botUUID] = &qrSession{
		botUUID:     botUUID,
		qrID:        qrResp.Qrcode,
		qrData:      qrResp.QrcodeImgContent,
		botType:     botType,
		startedAt:   time.Now(),
		botName:     req.BotName,
		botPlatform: req.BotPlatform,
	}
	logrus.WithFields(logrus.Fields{
		"botUUID":        botUUID,
		"qrID":           qrResp.Qrcode,
		"total_sessions": len(h.sessions),
	}).Info("QR session stored")
	h.mu.Unlock()

	c.JSON(http.StatusOK, QRStartResponse{
		Success: true,
		Data: QRStartData{
			QrCodeID:   qrResp.Qrcode,
			QrCodeData: qrResp.QrcodeImgContent,
			ExpiresIn:  300, // 5 minutes
		},
	})
}

// QRStatus polls the QR code login status
func (h *WeChatQRLoginHandler) QRStatus(c *gin.Context) {
	botUUID := c.Param("uuid")
	qrID := c.Query("qrcode_id")

	if qrID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing qrcode_id parameter"})
		return
	}

	h.mu.RLock()
	session, exists := h.sessions[botUUID]
	totalSessions := len(h.sessions)
	h.mu.RUnlock()

	logrus.WithFields(logrus.Fields{
		"botUUID":        botUUID,
		"qrID":           qrID,
		"session_exists": exists,
		"total_sessions": totalSessions,
	}).Info("QR status check")

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active QR session found"})
		return
	}

	if session.qrID != qrID {
		logrus.WithFields(logrus.Fields{
			"botUUID":       botUUID,
			"expected_qrID": session.qrID,
			"provided_qrID": qrID,
		}).Warn("QR ID mismatch")
		c.JSON(http.StatusNotFound, gin.H{"error": "QR ID mismatch"})
		return
	}

	// Check if session expired
	if time.Since(session.startedAt) > 8*time.Minute {
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()
		c.JSON(http.StatusOK, QRStatusResponse{
			Success: true,
			Data:    QRStatusData{Status: "expired"},
		})
		return
	}

	// Create client for this request
	client := &wechatQRClient{
		baseURL: "https://ilinkai.weixin.qq.com",
		httpClient: &http.Client{
			Timeout: 35 * time.Second, // Longer timeout for long-poll
		},
	}

	// Poll QR status
	statusResp, err := client.GetQRStatus(c.Request.Context(), qrID)
	if err != nil {
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()
		c.JSON(http.StatusInternalServerError, QRStatusResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Handle status
	switch statusResp.Status {
	case "wait", "scaned":
		c.JSON(http.StatusOK, QRStatusResponse{
			Success: true,
			Data:    QRStatusData{Status: statusResp.Status},
		})

	case "confirmed":
		// Save credentials to database (create bot if it doesn't exist yet)
		realUUID, err := h.saveCredentials(session, statusResp)
		if err != nil {
			logrus.WithError(err).WithField("bot", botUUID).Error("Failed to save credentials")
			c.JSON(http.StatusInternalServerError, QRStatusResponse{
				Success: false,
				Error:   "Failed to save credentials",
			})
			return
		}

		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()

		c.JSON(http.StatusOK, QRStatusResponse{
			Success: true,
			Data:    QRStatusData{Status: "confirmed", BotUUID: realUUID},
		})

	case "expired":
		// QR expired, allow frontend to request new one
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()

		c.JSON(http.StatusOK, QRStatusResponse{
			Success: true,
			Data:    QRStatusData{Status: "expired"},
		})

	default:
		c.JSON(http.StatusOK, QRStatusResponse{
			Success: true,
			Data:    QRStatusData{Status: statusResp.Status},
		})
	}
}

// QRCancel cancels the pending QR login
func (h *WeChatQRLoginHandler) QRCancel(c *gin.Context) {
	botUUID := c.Param("uuid")

	h.mu.Lock()
	delete(h.sessions, botUUID)
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// saveCredentials saves the Weixin credentials to the database.
// If the bot doesn't exist yet (temp UUID), it creates a new record and returns the real UUID.
func (h *WeChatQRLoginHandler) saveCredentials(session *qrSession, status *qrStatusResponse) (string, error) {
	authConfig := map[string]string{
		"token":    status.BotToken,
		"bot_id":   status.IlinkBotID,
		"user_id":  status.IlinkUserID,
		"base_url": status.BaseURL,
	}

	// Check if bot already exists in DB
	existing, err := h.settingsStore.GetSettingsByUUID(session.botUUID)
	if err != nil {
		return "", fmt.Errorf("get bot setting: %w", err)
	}

	if existing.UUID != "" {
		// Bot exists — update credentials
		existing.Auth = authConfig
		existing.AuthType = "qr"
		if err := h.settingsStore.UpdateSettings(existing.UUID, existing); err != nil {
			return "", fmt.Errorf("update bot setting: %w", err)
		}
		logrus.WithFields(logrus.Fields{
			"bot":    existing.UUID,
			"bot_id": status.IlinkBotID,
		}).Info("Weixin credentials updated")
		return existing.UUID, nil
	}

	// Bot doesn't exist — create it with the credentials
	platform := session.botPlatform
	if platform == "" {
		platform = "weixin"
	}
	name := session.botName
	if name == "" {
		name = platform + " Bot"
	}
	created, err := h.settingsStore.CreateSettings(db.Settings{
		Name:     name,
		Platform: platform,
		AuthType: "qr",
		Auth:     authConfig,
		Enabled:  false,
	})
	if err != nil {
		return "", fmt.Errorf("create bot setting: %w", err)
	}
	logrus.WithFields(logrus.Fields{
		"bot":    created.UUID,
		"bot_id": status.IlinkBotID,
	}).Info("Weixin bot created with credentials")
	return created.UUID, nil
}

// qrBotQRCodeResponse represents the QR code response from Weixin API
type qrBotQRCodeResponse struct {
	Qrcode           string `json:"qrcode,omitempty"`
	QrcodeImgContent string `json:"qrcode_img_content,omitempty"`
}

// qrStatusResponse represents the QR status response from Weixin API
type qrStatusResponse struct {
	Status      string `json:"status,omitempty"` // wait, scaned, confirmed, expired
	BotToken    string `json:"bot_token,omitempty"`
	IlinkBotID  string `json:"ilink_bot_id,omitempty"`
	IlinkUserID string `json:"ilink_user_id,omitempty"`
	BaseURL     string `json:"baseurl,omitempty"`
}

// GetBotQRCode fetches a QR code for Weixin bot login
func (c *wechatQRClient) GetBotQRCode(ctx context.Context, botType string) (*qrBotQRCodeResponse, error) {
	if botType == "" {
		botType = "3" // Default bot type
	}

	// Build URL with query params (GET request)
	u, err := url.Parse(c.baseURL + "/ilink/bot/get_bot_qrcode")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	query := u.Query()
	query.Set("bot_type", botType)
	u.RawQuery = query.Encode()

	logrus.Debugf("GetBotQRCode URL: %s", u.String())

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers (no Authorization for QR code request)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Log response for debugging
	logrus.Debugf("GetBotQRCode response: %s", string(body))

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	// Unmarshal response
	var result qrBotQRCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// GetQRStatus polls the QR code status
func (c *wechatQRClient) GetQRStatus(ctx context.Context, qrcode string) (*qrStatusResponse, error) {
	// Build URL with query params
	u, err := url.Parse(c.baseURL + "/ilink/bot/get_qrcode_status")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	query := u.Query()
	query.Set("qrcode", qrcode)
	u.RawQuery = query.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	// Add required header for QR status polling
	req.Header.Set("iLink-App-ClientVersion", "1")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Timeout is normal, return "wait" status
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &qrStatusResponse{Status: "wait"}, nil
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	// Unmarshal response
	var result qrStatusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// checkRateLimit checks if the bot has exceeded the QR code request rate limit
// Returns true if the request is allowed, false if rate limited
func (h *WeChatQRLoginHandler) checkRateLimit(botUUID string) bool {
	now := time.Now()
	windowStart := now.Add(-rateLimitWindow)

	h.rateLimitMu.Lock()
	defer h.rateLimitMu.Unlock()

	// Get existing request timestamps for this bot
	timestamps := h.rateLimitMap[botUUID]

	// Filter out timestamps outside the current window
	var validTimestamps []time.Time
	for _, ts := range timestamps {
		if ts.After(windowStart) {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	// Check if we've exceeded the limit
	if len(validTimestamps) >= maxQRRequestsPerMinute {
		return false
	}

	// Add current request timestamp
	validTimestamps = append(validTimestamps, now)
	h.rateLimitMap[botUUID] = validTimestamps

	return true
}

// cleanupRateLimitMap removes old entries from the rate limit map
// This should be called periodically (e.g., via a background goroutine)
func (h *WeChatQRLoginHandler) cleanupRateLimitMap() {
	h.rateLimitMu.Lock()
	defer h.rateLimitMu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rateLimitWindow)

	for botUUID, timestamps := range h.rateLimitMap {
		var validTimestamps []time.Time
		for _, ts := range timestamps {
			if ts.After(windowStart) {
				validTimestamps = append(validTimestamps, ts)
			}
		}

		if len(validTimestamps) == 0 {
			// Remove empty entries to save memory
			delete(h.rateLimitMap, botUUID)
		} else {
			h.rateLimitMap[botUUID] = validTimestamps
		}
	}
}
