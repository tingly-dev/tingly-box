// Package weixin provides Weixin platform bot implementation for ImBot.
package weixin

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/contexttoken"
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

	// Use pairing adapter to get QR code
	pairingAdapter := h.bot.plugin.Pairing()
	if pairingAdapter == nil {
		return nil, fmt.Errorf("pairing adapter not available")
	}

	// Start QR login
	result, err := pairingAdapter.LoginWithQrStart(ctx, h.bot.accountID)
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

	// Use pairing adapter to start pairing
	pairingAdapter := h.bot.plugin.Pairing()
	if pairingAdapter == nil {
		return nil, fmt.Errorf("pairing adapter not available")
	}

	// Start QR login
	result, err := pairingAdapter.LoginWithQrStart(ctx, h.bot.accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to start pairing: %w", err)
	}

	// Set account as pending configuration
	h.bot.account.Configured = false
	if err := h.bot.plugin.Accounts().Save(h.bot.account); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
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

	// Use pairing adapter to check status
	pairingAdapter := h.bot.plugin.Pairing()
	if pairingAdapter == nil {
		return nil, fmt.Errorf("pairing adapter not available")
	}

	// Wait for QR scan
	result, err := pairingAdapter.LoginWithQrWait(ctx, h.bot.accountID, qrID)
	if err != nil {
		return nil, fmt.Errorf("failed to check pairing status: %w", err)
	}

	// Convert to PairingStatus
	status := &PairingStatus{}

	if result.Success {
		// Pairing successful, load updated account
		account, err := h.bot.plugin.Accounts().Get(h.bot.accountID)
		if err != nil {
			status.Status = "error"
			status.ErrorMsg = "failed to load account after pairing"
			return status, nil
		}

		status.Status = "success"
		status.BotID = account.BotID
		status.UserID = account.UserID
		status.PairedAt = account.LastLoginAt.Unix()

		// Update local account reference
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
	return h.bot.account.Configured && h.bot.account.BotToken != ""
}

// ReAuthenticate starts the re-authentication process for an expired session
func (h *InteractionHandler) ReAuthenticate(ctx context.Context) (*QRCodeResult, error) {
	// Reset account configuration
	if h.bot.account == nil {
		return nil, fmt.Errorf("account not loaded")
	}

	h.bot.account.Configured = false
	h.bot.account.BotToken = ""
	h.bot.account.BotID = ""
	h.bot.account.UserID = ""

	if err := h.bot.plugin.Accounts().Save(h.bot.account); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	h.bot.Logger().Info("Weixin account reset for re-authentication: %s", h.bot.account.ID)

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

	return &AccountInfo{
		AccountID:   h.bot.account.ID,
		Name:        h.bot.account.Name,
		BotID:       h.bot.account.BotID,
		UserID:      h.bot.account.UserID,
		BaseURL:     h.bot.account.BaseURL,
		Configured:  h.bot.account.Configured,
		Enabled:     h.bot.account.Enabled,
		CreatedAt:   h.bot.account.CreatedAt.Unix(),
		LastLoginAt: h.bot.account.LastLoginAt.Unix(),
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
	if h.bot.client == nil {
		return "", fmt.Errorf("not connected")
	}

	// Get context token from storage
	contextToken := contexttoken.GetContextToken(h.bot.accountID, userID)

	if err := h.bot.client.SendTextMessage(ctx, userID, contextToken, text); err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return fmt.Sprintf("weixin-%d", h.bot.Status().LastActivity), nil
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
