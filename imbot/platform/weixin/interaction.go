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

// GetQRCode returns a QR code image for account pairing
func (h *InteractionHandler) GetQRCode(ctx context.Context) (*QRCodeResult, error) {
	if h.bot.account == nil {
		return nil, fmt.Errorf("account not loaded")
	}

	// Use WechatBot.LoginWithQrStart to get QR code
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

// QRCodeResult represents a QR code for pairing
type QRCodeResult struct {
	QrCodeID   string
	QrCodeURL  string
	QrCodeData string // Base64 encoded image data
	ExpiresIn  int    // Seconds until expiration
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

	// Use WechatBot.LoginWithQrStart to start pairing
	result, err := h.bot.LoginWithQrStart(ctx, h.bot.accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to start pairing: %w", err)
	}

	// Note: We don't save to file system here.
	// The account state will be updated by our database layer after successful pairing.

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

	// Use WechatBot.LoginWithQrWait to check status
	// Note: This will update the bot's internal account on success
	result, err := h.bot.LoginWithQrWait(ctx, h.bot.accountID, qrID)
	if err != nil {
		return nil, fmt.Errorf("failed to check pairing status: %w", err)
	}

	// Convert to PairingStatus
	status := &PairingStatus{}

	if result.Success {
		// Pairing successful - SDK has updated the account and saved it to the store
		// Get the updated account from the bot
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

		// Update our bot's account reference
		h.bot.account = account
	} else {
		status.Status = "failed"
		status.ErrorMsg = result.Error
	}

	return status, nil
}

// PairingStatus represents the status of QR code pairing
type PairingStatus struct {
	Status   string // "pending", "success", "failed", "expired"
	BotID    string
	UserID   string
	PairedAt int64
	ErrorMsg string
}

// IsConfigured checks if the account is fully configured
func (h *InteractionHandler) IsConfigured() bool {
	if h.bot.account == nil {
		return false
	}
	return h.bot.account.IsConfigured()
}

// ReAuthenticate starts the re-authentication process for an expired session
func (h *InteractionHandler) ReAuthenticate(ctx context.Context) (*QRCodeResult, error) {
	// Reset account configuration
	if h.bot.account == nil {
		return nil, fmt.Errorf("account not loaded")
	}

	wcAccount := h.bot.account.WeChatAccount()
	wcAccount.Configured = false
	wcAccount.BotToken = ""
	wcAccount.BotID = ""
	wcAccount.UserID = ""

	// Save the updated account to store
	if err := h.bot.SaveAccount(wcAccount); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	h.bot.Logger().Info("Weixin account reset for re-authentication: %s", h.bot.accountID)

	// Emit session expired event
	h.bot.EmitError(core.NewAuthFailedError(core.PlatformWeixin, "session expired, please re-authenticate", nil))

	// Start new pairing
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

// AccountInfo represents information about a Weixin account
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

// PairAccount is a convenience method that starts pairing and waits for completion
func (h *InteractionHandler) PairAccount(ctx context.Context) (*PairingStatus, error) {
	// Start pairing
	_, err := h.StartPairing(ctx)
	if err != nil {
		return nil, err
	}

	// In a real implementation, this would poll for status
	// For now, just return the initial status
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

	// Build outbound message
	msg := &types.OutboundMessage{
		To:   userID,
		Text: text,
	}

	// Send via WechatBot
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
	// Weixin API doesn't provide a contacts endpoint
	// This would need to be implemented differently
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
