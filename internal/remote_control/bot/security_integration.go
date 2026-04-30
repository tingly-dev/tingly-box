package bot

import (
	"github.com/tingly-dev/tingly-box/internal/remote_control/audit"
	secaudit "github.com/tingly-dev/tingly-box/internal/remote_control/bot/security/audit"
	secpairing "github.com/tingly-dev/tingly-box/internal/remote_control/bot/security/pairing"
	secpermission "github.com/tingly-dev/tingly-box/internal/remote_control/bot/security/permission"
)

// InitSecuritySystem initializes the security system components.
// This is called during BotHandler construction.
func (h *BotHandler) InitSecuritySystem() error {
	// Initialize permission handler (parallel to existing IMPrompter)
	h.permissionHandler = secpermission.NewHandler(h.manager)

	// Initialize pairing integration wrapper
	if h.pairing != nil {
		h.pairingIntegration = secpairing.NewBotIntegration(h.pairing, h.manager)
	}

	// Initialize audit logger wrapper (parallel to existing audit logger)
	if h.audit != nil {
		h.botAuditLogger = secaudit.NewLogger(audit.Config{
			Console: true,
		})
	}

	return nil
}

// GetPermissionHandler returns the security permission handler
func (h *BotHandler) GetPermissionHandler() *secpermission.Handler {
	return h.permissionHandler
}

// GetPairingIntegration returns the pairing integration wrapper
func (h *BotHandler) GetPairingIntegration() *secpairing.BotIntegration {
	return h.pairingIntegration
}

// GetAuditLogger returns the bot audit logger wrapper
func (h *BotHandler) GetAuditLogger() *secaudit.Logger {
	return h.botAuditLogger
}
