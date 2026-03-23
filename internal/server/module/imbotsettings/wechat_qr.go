// Package imbotsettings provides handlers for ImBot settings management.
package imbotsettings

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// WeChatQRLoginHandler handles WeChat QR code login flow
type WeChatQRLoginHandler struct {
	settingsStore *db.ImBotSettingsStore
	sessions      map[string]*qrSession
	mu            sync.Mutex
}

type qrSession struct {
	botUUID   string
	qrID      string
	qrURL     string
	qrData    string
	startedAt time.Time
	client    *wechatQRClient
}

type wechatQRClient struct {
	baseURL string
}

// NewWeChatQRLoginHandler creates a new WeChat QR login handler
func NewWeChatQRLoginHandler(settingsStore *db.ImBotSettingsStore) *WeChatQRLoginHandler {
	return &WeChatQRLoginHandler{
		settingsStore: settingsStore,
		sessions:      make(map[string]*qrSession),
	}
}

// QRStartRequest is the request to start QR login
type QRStartRequest struct {
	BotUUID string `json:"bot_uuid" binding:"required"`
}

// QRStartResponse is the response for QR start
type QRStartResponse struct {
	QrCodeID   string `json:"qrcode_id"`
	QrCodeData string `json:"qrcode_data"`
	ExpiresIn  int    `json:"expires_in"`
}

// QRStatusResponse is the response for QR status
type QRStatusResponse struct {
	Status string `json:"status"` // wait, scaned, confirmed, expired
	Error  string `json:"error,omitempty"`
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

	// Create QR code client
	client := &wechatQRClient{
		baseURL: "https://ilinkai.weixin.qq.com",
	}

	// Fetch QR code
	qrResp, err := client.GetBotQRCode(c.Request.Context())
	if err != nil {
		logrus.WithError(err).WithField("bot", botUUID).Error("Failed to get QR code")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get QR code"})
		return
	}

	// Store session
	h.mu.Lock()
	h.sessions[botUUID] = &qrSession{
		botUUID:   botUUID,
		qrID:      qrResp.Qrcode,
		qrData:    qrResp.QrcodeImgContent,
		startedAt: time.Now(),
		client:    client,
	}
	h.mu.Unlock()

	c.JSON(http.StatusOK, QRStartResponse{
		QrCodeID:   qrResp.Qrcode,
		QrCodeData: qrResp.QrcodeImgContent,
		ExpiresIn:  300, // 5 minutes
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

	h.mu.Lock()
	session, exists := h.sessions[botUUID]
	h.mu.Unlock()

	if !exists || session.qrID != qrID {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active QR session found"})
		return
	}

	// Check if session expired
	if time.Since(session.startedAt) > 8*time.Minute {
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()
		c.JSON(http.StatusOK, QRStatusResponse{Status: "expired"})
		return
	}

	// Poll QR status
	statusResp, err := session.client.GetQRStatus(c.Request.Context(), qrID)
	if err != nil {
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()
		c.JSON(http.StatusInternalServerError, QRStatusResponse{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	// Handle status
	switch statusResp.Status {
	case "wait", "scaned":
		c.JSON(http.StatusOK, QRStatusResponse{Status: statusResp.Status})

	case "confirmed":
		// Save credentials to database
		if err := h.saveCredentials(c.Request.Context(), botUUID, statusResp); err != nil {
			logrus.WithError(err).WithField("bot", botUUID).Error("Failed to save credentials")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save credentials"})
			return
		}

		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()

		c.JSON(http.StatusOK, QRStatusResponse{Status: "confirmed"})

	case "expired":
		// QR expired, allow frontend to request new one
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()

		c.JSON(http.StatusOK, QRStatusResponse{Status: "expired"})

	default:
		c.JSON(http.StatusOK, QRStatusResponse{Status: statusResp.Status})
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

// saveCredentials saves the WeChat credentials to the database
func (h *WeChatQRLoginHandler) saveCredentials(ctx context.Context, botUUID string, status *qrStatusResponse) error {
	// Get existing bot settings
	setting, err := h.settingsStore.GetSettingsByUUID(botUUID)
	if err != nil {
		return fmt.Errorf("get bot setting: %w", err)
	}

	// Update auth config
	authConfig := map[string]string{
		"token":    status.BotToken,
		"bot_id":   status.IlinkBotID,
		"user_id":  status.IlinkUserID,
		"base_url": status.BaseURL,
	}

	setting.Auth = authConfig
	setting.AuthType = "qr"

	// Save to database
	if err := h.settingsStore.UpdateSettings(botUUID, setting); err != nil {
		return fmt.Errorf("update bot setting: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"bot":    botUUID,
		"bot_id": status.IlinkBotID,
	}).Info("WeChat credentials saved successfully")

	return nil
}

// wechatQRClient handles WeChat QR API calls
type qrBotQRCodeResponse struct {
	Qrcode           string `json:"qrcode"`
	QrcodeImgContent string `json:"qrcode_img_content"`
}

type qrStatusResponse struct {
	Status      string `json:"status"` // wait, scaned, confirmed, expired
	BotToken    string `json:"bot_token,omitempty"`
	IlinkBotID  string `json:"ilink_bot_id,omitempty"`
	IlinkUserID string `json:"ilink_user_id,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
}

func (c *wechatQRClient) GetBotQRCode(ctx context.Context) (*qrBotQRCodeResponse, error) {
	// TODO: Implement actual API call to https://ilinkai.weixin.qq.com/ilink/bot/get_bot_qrcode
	// For now, return a mock response
	return &qrBotQRCodeResponse{
		Qrcode:           "mock-qr-token-" + fmt.Sprint(time.Now().Unix()),
		QrcodeImgContent: "https://ilinkai.weixin.qq.com/mock-qr-" + fmt.Sprint(time.Now().Unix()),
	}, nil
}

func (c *wechatQRClient) GetQRStatus(ctx context.Context, qrID string) (*qrStatusResponse, error) {
	// TODO: Implement actual API call to https://ilinkai.weixin.qq.com/ilink/bot/get_qrcode_status
	// For now, return wait status
	return &qrStatusResponse{
		Status: "wait",
	}, nil
}
