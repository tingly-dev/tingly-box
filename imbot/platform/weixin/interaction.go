// Package weixin provides Weixin platform bot implementation for ImBot.
package weixin

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/types"
)

// InteractionHandler provides interaction handlers for Weixin
type InteractionHandler struct {
	bot *Bot
}

// NewInteractionHandler creates a new interaction handler
func NewInteractionHandler(bot *Bot) *InteractionHandler {
	return &InteractionHandler{bot: bot}
}

// QRCodeResult represents a QR code for pairing (imbot layer type)
type QRCodeResult struct {
	QrCodeID   string
	QrCodeURL  string
	QrCodeData string // Base64 encoded image data
	ExpiresIn  int    // Seconds until expiration
}

// PairingStatus represents the status of QR code pairing (imbot layer type)
type PairingStatus struct {
	Status   string // "pending", "success", "failed", "expired"
	BotID    string
	UserID   string
	PairedAt int64
	ErrorMsg string
}

// AccountInfo represents information about a Weixin account (imbot layer type)
type AccountInfo struct {
	AccountID   string
	Name        string
	BotID       string
	UserID      string
	BaseURL     string
	Configured  bool
	Enabled     bool
	CreatedAt   int64
	LastLoginAt int64
}

// GetQRCode returns a QR code image for account pairing
func (h *InteractionHandler) GetQRCode(ctx context.Context) (*QRCodeResult, error) {
	if h.bot.account == nil {
		return nil, fmt.Errorf("account not loaded")
	}
	result, err := h.bot.LoginWithQrStart(ctx, h.bot.accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR code: %w", err)
	}
	return &QRCodeResult{
		QrCodeID:   result.QrCodeID,
		QrCodeURL:  result.QrCodeURL,
		QrCodeData: result.QrCodeData,
		ExpiresIn:  result.ExpiresIn,
	}, nil
}

// GetQRCodeDisplayURL returns a URL to display the QR code
func (h *InteractionHandler) GetQRCodeDisplayURL(ctx context.Context) (string, error) {
	qrResult, err := h.GetQRCode(ctx)
	if err != nil {
		return "", err
	}
	return qrResult.QrCodeURL, nil
}

// StartPairing starts the QR code pairing process
func (h *InteractionHandler) StartPairing(ctx context.Context) (*QRCodeResult, error) {
	if h.bot.account == nil {
		return nil, fmt.Errorf("account not loaded")
	}
	result, err := h.bot.LoginWithQrStart(ctx, h.bot.accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to start pairing: %w", err)
	}
	h.bot.Logger().Info("Weixin pairing started for account: %s", h.bot.accountID)
	return &QRCodeResult{
		QrCodeID:   result.QrCodeID,
		QrCodeURL:  result.QrCodeURL,
		QrCodeData: result.QrCodeData,
		ExpiresIn:  result.ExpiresIn,
	}, nil
}

// CheckPairingStatus checks the status of the QR code pairing process
func (h *InteractionHandler) CheckPairingStatus(ctx context.Context, qrID string) (*PairingStatus, error) {
	if h.bot.account == nil {
		return nil, fmt.Errorf("account not loaded")
	}
	result, err := h.bot.LoginWithQrWait(ctx, h.bot.accountID, qrID)
	if err != nil {
		return nil, fmt.Errorf("failed to check pairing status: %w", err)
	}

	status := &PairingStatus{}
	if result.Success {
		account := h.bot.Account()
		if account == nil {
			status.Status = "error"
			status.ErrorMsg = "failed to get account after pairing"
			return status, nil
		}
		wcAccount := account.WeChatAccount()
		status.Status = "success"
		status.BotID = wcAccount.BotID
		status.UserID = wcAccount.UserID
		status.PairedAt = wcAccount.LastLoginAt.Unix()
		h.bot.account = account
	} else {
		status.Status = "failed"
		status.ErrorMsg = result.Error
	}
	return status, nil
}

// IsConfigured checks if the account is fully configured
func (h *InteractionHandler) IsConfigured() bool {
	return h.bot.account != nil && h.bot.account.IsConfigured()
}

// ReAuthenticate starts the re-authentication process for an expired session
func (h *InteractionHandler) ReAuthenticate(ctx context.Context) (*QRCodeResult, error) {
	if h.bot.account == nil {
		return nil, fmt.Errorf("account not loaded")
	}

	wcAccount := h.bot.account.WeChatAccount()
	wcAccount.Configured = false
	wcAccount.BotToken = ""
	wcAccount.BotID = ""
	wcAccount.UserID = ""

	if err := h.bot.SaveAccount(wcAccount); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	h.bot.Logger().Info("Weixin account reset for re-authentication: %s", h.bot.accountID)
	h.bot.EmitError(core.NewAuthFailedError(core.PlatformWeixin, "session expired, please re-authenticate", nil))

	return h.StartPairing(ctx)
}

// GetAccountInfo returns information about the current account
func (h *InteractionHandler) GetAccountInfo() *AccountInfo {
	if h.bot.account == nil {
		return &AccountInfo{
			AccountID:  h.bot.accountID,
			Configured: false,
		}
	}
	wcAccount := h.bot.account.WeChatAccount()
	return &AccountInfo{
		AccountID:   wcAccount.ID,
		Name:        wcAccount.Name,
		BotID:       wcAccount.BotID,
		UserID:      wcAccount.UserID,
		BaseURL:     wcAccount.BaseURL,
		Configured:  wcAccount.Configured,
		Enabled:     wcAccount.Enabled,
		CreatedAt:   wcAccount.CreatedAt.Unix(),
		LastLoginAt: wcAccount.LastLoginAt.Unix(),
	}
}

// PairAccount is a convenience method that starts pairing
func (h *InteractionHandler) PairAccount(ctx context.Context) (*PairingStatus, error) {
	_, err := h.StartPairing(ctx)
	if err != nil {
		return nil, err
	}
	return &PairingStatus{
		Status: "pending",
	}, nil
}

// CompletePairing waits for the user to scan the QR code and confirms pairing
func (h *InteractionHandler) CompletePairing(ctx context.Context, qrID string) (*PairingStatus, error) {
	if qrID == "" {
		return nil, fmt.Errorf("qrID is required")
	}
	return h.CheckPairingStatus(ctx, qrID)
}

// SendMessage sends a message to a specific user
func (h *InteractionHandler) SendMessage(ctx context.Context, userID string, text string) (string, error) {
	if h.bot.account == nil {
		return "", fmt.Errorf("not connected")
	}

	msg := &types.OutboundMessage{
		To:   userID,
		Text: text,
	}

	result, err := h.bot.Send(ctx, msg)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	if !result.OK {
		return "", fmt.Errorf("failed to send message: %s", result.Error)
	}

	return result.MessageID, nil
}

// GetContacts returns a list of contacts (not yet implemented)
func (h *InteractionHandler) GetContacts(ctx context.Context) ([]Contact, error) {
	return nil, core.NewBotError(core.ErrPlatformError, "contacts list not available", false)
}

// Contact represents a Weixin contact
type Contact struct {
	ID       string
	Name     string
	Avatar   string
	Type     string // "user", "group"
	Verified bool
}
