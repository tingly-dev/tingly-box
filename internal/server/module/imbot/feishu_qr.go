package imbot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// FeishuRegHandler drives the Feishu/Lark one-click app registration flow
// (OAuth 2.0 Device Authorization Grant, RFC 8628). The SDK's RegisterApp is a
// single blocking call that emits the QR link via a callback and then polls
// until the user authorizes, so it runs in a background goroutine while the
// HTTP layer exposes a start/status/cancel session model that mirrors the
// Weixin QR flow.
type FeishuRegHandler struct {
	settingsStore *db.ImBotSettingsStore
	sessions      map[string]*feishuRegSession
	mu            sync.RWMutex
	rateLimitMap  map[string][]time.Time
	rateLimitMu   sync.Mutex
}

type feishuRegSession struct {
	botUUID   string
	platform  string // feishu | lark
	botName   string
	startedAt time.Time
	cancel    context.CancelFunc

	// saveOnce guards the one-time credential persistence on confirmation.
	saveOnce  sync.Once
	savedUUID string
	saveErr   error

	mu       sync.Mutex
	status   string // pending, confirmed, expired, denied, error
	qrURL    string
	expireIn int
	result   *registration.RegisterAppResult
	errMsg   string
}

// NewFeishuRegHandler creates a new Feishu/Lark one-click registration handler.
func NewFeishuRegHandler(settingsStore *db.ImBotSettingsStore) *FeishuRegHandler {
	return &FeishuRegHandler{
		settingsStore: settingsStore,
		sessions:      make(map[string]*feishuRegSession),
		rateLimitMap:  make(map[string][]time.Time),
	}
}

// QRStart initiates the one-click registration flow and returns the QR link.
func (h *FeishuRegHandler) QRStart(c *gin.Context) {
	var req FeishuRegStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	botUUID := c.Param("uuid")
	if botUUID != req.BotUUID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID mismatch"})
		return
	}

	platform := req.BotPlatform
	if platform != "lark" {
		platform = "feishu"
	}

	// Validate bot existence unless this is a deferred (temp-) creation.
	if !strings.HasPrefix(botUUID, "temp-") {
		existing, err := h.settingsStore.GetSettingsByUUID(botUUID)
		if err != nil {
			logrus.WithError(err).WithField("bot", botUUID).Error("Failed to check bot existence")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate bot"})
			return
		}
		if existing.UUID == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bot not found"})
			return
		}
	}

	if !h.checkRateLimit(botUUID) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":       "Too many registration requests. Please wait a moment before trying again.",
			"retry_after": int(rateLimitWindow.Seconds()),
		})
		return
	}

	// RegisterApp polls until the user authorizes or the link expires; give it
	// a generous ceiling and rely on QRCancel / link expiry for early exit.
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	sess := &feishuRegSession{
		botUUID:   botUUID,
		platform:  platform,
		botName:   req.BotName,
		startedAt: time.Now(),
		cancel:    cancel,
		status:    "pending",
	}

	qrReady := make(chan struct{})
	var qrOnce sync.Once
	opts := &registration.Options{
		Source: "tingly-box",
		OnQRCode: func(info *registration.QRCodeInfo) {
			sess.mu.Lock()
			sess.qrURL = info.URL
			sess.expireIn = info.ExpireIn
			sess.mu.Unlock()
			qrOnce.Do(func() { close(qrReady) })
		},
		OnStatusChange: func(info *registration.StatusChangeInfo) {
			// The SDK auto-switches to the Lark domain when it detects a Lark
			// tenant; reflect that so the saved bot lands on the right platform.
			if info.Status == registration.StatusDomainSwitched {
				sess.mu.Lock()
				sess.platform = "lark"
				sess.mu.Unlock()
			}
		},
	}

	go func() {
		defer cancel()
		result, err := registration.RegisterApp(ctx, opts)
		sess.mu.Lock()
		defer sess.mu.Unlock()
		if err != nil {
			var accessDenied *registration.AccessDeniedError
			var expired *registration.ExpiredError
			switch {
			case errors.As(err, &accessDenied):
				sess.status = "denied"
			case errors.As(err, &expired):
				sess.status = "expired"
			default:
				sess.status = "error"
				sess.errMsg = err.Error()
			}
			logrus.WithError(err).WithField("bot", botUUID).Warn("Feishu RegisterApp failed")
			return
		}
		sess.result = result
		sess.status = "confirmed"
		logrus.WithField("bot", botUUID).Info("Feishu one-click registration succeeded")
	}()

	select {
	case <-qrReady:
		// QR link is ready to hand back to the caller.
	case <-ctx.Done():
		sess.mu.Lock()
		msg := sess.errMsg
		sess.mu.Unlock()
		if msg == "" {
			msg = "registration failed before a QR code was issued"
		}
		c.JSON(http.StatusInternalServerError, FeishuRegStartResponse{Success: false, Error: msg})
		return
	case <-time.After(30 * time.Second):
		cancel()
		c.JSON(http.StatusInternalServerError, FeishuRegStartResponse{Success: false, Error: "timed out waiting for QR code"})
		return
	}

	h.mu.Lock()
	if old := h.sessions[botUUID]; old != nil {
		old.cancel()
	}
	h.sessions[botUUID] = sess
	h.mu.Unlock()

	sess.mu.Lock()
	qrURL, expireIn := sess.qrURL, sess.expireIn
	sess.mu.Unlock()

	c.JSON(http.StatusOK, FeishuRegStartResponse{
		Success: true,
		Data: FeishuRegStartData{
			QRURL:     qrURL,
			ExpiresIn: expireIn,
		},
	})
}

// QRStatus reports the current state of the registration session.
func (h *FeishuRegHandler) QRStatus(c *gin.Context) {
	botUUID := c.Param("uuid")

	h.mu.RLock()
	sess, exists := h.sessions[botUUID]
	h.mu.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active registration session found"})
		return
	}

	sess.mu.Lock()
	status := sess.status
	result := sess.result
	errMsg := sess.errMsg
	platform := sess.platform
	sess.mu.Unlock()

	switch status {
	case "pending":
		c.JSON(http.StatusOK, FeishuRegStatusResponse{
			Success: true,
			Data:    FeishuRegStatusData{Status: "pending"},
		})

	case "confirmed":
		sess.saveOnce.Do(func() {
			sess.savedUUID, sess.saveErr = h.saveCredentials(sess, result, platform)
		})
		if sess.saveErr != nil {
			logrus.WithError(sess.saveErr).WithField("bot", botUUID).Error("Failed to save Feishu credentials")
			c.JSON(http.StatusInternalServerError, FeishuRegStatusResponse{
				Success: false,
				Error:   "Failed to save credentials",
			})
			return
		}
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()

		tenantBrand := ""
		if result != nil && result.UserInfo != nil {
			tenantBrand = result.UserInfo.TenantBrand
		}
		c.JSON(http.StatusOK, FeishuRegStatusResponse{
			Success: true,
			Data: FeishuRegStatusData{
				Status:      "confirmed",
				BotUUID:     sess.savedUUID,
				TenantBrand: tenantBrand,
			},
		})

	case "denied", "expired":
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()
		c.JSON(http.StatusOK, FeishuRegStatusResponse{
			Success: true,
			Data:    FeishuRegStatusData{Status: status},
		})

	default: // "error"
		h.mu.Lock()
		delete(h.sessions, botUUID)
		h.mu.Unlock()
		c.JSON(http.StatusOK, FeishuRegStatusResponse{
			Success: false,
			Data:    FeishuRegStatusData{Status: "error"},
			Error:   errMsg,
		})
	}
}

// QRCancel cancels a pending registration session.
func (h *FeishuRegHandler) QRCancel(c *gin.Context) {
	botUUID := c.Param("uuid")

	h.mu.Lock()
	if sess, ok := h.sessions[botUUID]; ok {
		sess.cancel()
		delete(h.sessions, botUUID)
	}
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// saveCredentials persists the App ID / App Secret returned by RegisterApp.
// If the bot already exists it updates the record in place; otherwise (deferred
// temp- creation) it creates a new, disabled bot and returns the real UUID.
func (h *FeishuRegHandler) saveCredentials(sess *feishuRegSession, result *registration.RegisterAppResult, platform string) (string, error) {
	if result == nil {
		return "", fmt.Errorf("empty registration result")
	}
	if platform != "lark" {
		platform = "feishu"
	}
	// The SDK reports the tenant brand it actually authorized against; trust it
	// over the platform the request came in on.
	if result.UserInfo != nil && (result.UserInfo.TenantBrand == "feishu" || result.UserInfo.TenantBrand == "lark") {
		platform = result.UserInfo.TenantBrand
	}

	authConfig := map[string]string{
		"clientId":     result.ClientID,
		"clientSecret": result.ClientSecret,
	}

	existing, err := h.settingsStore.GetSettingsByUUID(sess.botUUID)
	if err != nil {
		return "", fmt.Errorf("get bot setting: %w", err)
	}
	if existing.UUID != "" {
		existing.Auth = authConfig
		existing.AuthType = "oauth"
		if err := h.settingsStore.UpdateSettings(existing.UUID, existing); err != nil {
			return "", fmt.Errorf("update bot setting: %w", err)
		}
		logrus.WithField("bot", existing.UUID).Info("Feishu credentials updated via one-click registration")
		return existing.UUID, nil
	}

	name := sess.botName
	if name == "" {
		name = platform + " Bot"
	}
	created, err := h.settingsStore.CreateSettings(db.Settings{
		Name:     name,
		Platform: platform,
		AuthType: "oauth",
		Auth:     authConfig,
		Enabled:  false,
	})
	if err != nil {
		return "", fmt.Errorf("create bot setting: %w", err)
	}
	logrus.WithField("bot", created.UUID).Info("Feishu bot created via one-click registration")
	return created.UUID, nil
}

// checkRateLimit caps one-click registration starts per bot, reusing the QR
// rate-limit window/quota constants defined in wechat_qr.go.
func (h *FeishuRegHandler) checkRateLimit(botUUID string) bool {
	now := time.Now()
	windowStart := now.Add(-rateLimitWindow)

	h.rateLimitMu.Lock()
	defer h.rateLimitMu.Unlock()

	var valid []time.Time
	for _, ts := range h.rateLimitMap[botUUID] {
		if ts.After(windowStart) {
			valid = append(valid, ts)
		}
	}
	if len(valid) >= maxQRRequestsPerMinute {
		h.rateLimitMap[botUUID] = valid
		return false
	}
	h.rateLimitMap[botUUID] = append(valid, now)
	return true
}
