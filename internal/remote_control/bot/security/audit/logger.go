// Package audit provides security event logging for bot operations.
// It wraps the audit logger and provides bot-specific integration.
package audit

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/remote_control/audit"
)

// Logger wraps the audit logger with bot-specific convenience methods
type Logger struct {
	logger *audit.Logger
}

// NewLogger creates a new bot audit logger
func NewLogger(config audit.Config) *Logger {
	return &Logger{
		logger: audit.NewLogger(config),
	}
}

// LogPairingAttempt logs a pairing attempt
func (l *Logger) LogPairingAttempt(botUUID, chatID, senderID, platform, code string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelInfo,
		Action:   "pairing.attempt",
		UserID:   senderID,
		Success:  false,
		Message:  "Pairing attempt",
		Details: map[string]interface{}{
			"bot_uuid":  botUUID,
			"chat_id":   chatID,
			"platform":  platform,
			"code":      code,
		},
	})
}

// LogPairingSuccess logs a successful pairing
func (l *Logger) LogPairingSuccess(botUUID, chatID, senderID, platform string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelInfo,
		Action:   "pairing.success",
		UserID:   senderID,
		Success:  true,
		Message:  "Pairing successful",
		Details: map[string]interface{}{
			"bot_uuid":  botUUID,
			"chat_id":   chatID,
			"platform":  platform,
		},
	})
}

// LogPairingFailure logs a failed pairing attempt
func (l *Logger) LogPairingFailure(botUUID, chatID, senderID, platform, reason string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelWarn,
		Action:   "pairing.failure",
		UserID:   senderID,
		Success:  false,
		Message:  "Pairing failed: " + reason,
		Details: map[string]interface{}{
			"bot_uuid":  botUUID,
			"chat_id":   chatID,
			"platform":  platform,
			"reason":    reason,
		},
	})
}

// LogPairingRejection logs when a pairing code is rejected
func (l *Logger) LogPairingRejection(botUUID, chatID, senderID, platform, reason string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelWarn,
		Action:   "pairing.rejection",
		UserID:   senderID,
		Success:  false,
		Message:  "Pairing rejected: " + reason,
		Details: map[string]interface{}{
			"bot_uuid":  botUUID,
			"chat_id":   chatID,
			"platform":  platform,
			"reason":    reason,
		},
	})
}

// LogPermissionRequest logs a permission request
func (l *Logger) LogPermissionRequest(botUUID, chatID, toolName, requestID string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelInfo,
		Action:   "permission.request",
		UserID:   chatID,
		Success:  false,
		Message:  "Permission requested",
		Details: map[string]interface{}{
			"bot_uuid":   botUUID,
			"chat_id":    chatID,
			"tool_name":  toolName,
			"request_id": requestID,
		},
	})
}

// LogPermissionApproved logs a permission approval
func (l *Logger) LogPermissionApproved(botUUID, chatID, toolName, requestID string, remember bool) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelInfo,
		Action:   "permission.approved",
		UserID:   chatID,
		Success:  true,
		Message:  "Permission approved",
		Details: map[string]interface{}{
			"bot_uuid":   botUUID,
			"chat_id":    chatID,
			"tool_name":  toolName,
			"request_id": requestID,
			"remember":   remember,
		},
	})
}

// LogPermissionDenied logs a permission denial
func (l *Logger) LogPermissionDenied(botUUID, chatID, toolName, requestID, reason string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelWarn,
		Action:   "permission.denied",
		UserID:   chatID,
		Success:  false,
		Message:  "Permission denied: " + reason,
		Details: map[string]interface{}{
			"bot_uuid":   botUUID,
			"chat_id":    chatID,
			"tool_name":  toolName,
			"request_id": requestID,
			"reason":     reason,
		},
	})
}

// LogUnpairedMessageRejected logs when a message from an unpaired chat is rejected
func (l *Logger) LogUnpairedMessageRejected(botUUID, chatID, senderID, platform string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelWarn,
		Action:   "security.unpaired_message_rejected",
		UserID:   senderID,
		Success:  false,
		Message:  "Message rejected from unpaired chat",
		Details: map[string]interface{}{
			"bot_uuid":  botUUID,
			"chat_id":   chatID,
			"platform":  platform,
		},
	})
}

// LogWhitelistAdd logs when a group is added to the whitelist
func (l *Logger) LogWhitelistAdd(botUUID, groupID, userID, platform string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelInfo,
		Action:   "security.whitelist_add",
		UserID:   userID,
		Success:  true,
		Message:  "Group added to whitelist",
		Details: map[string]interface{}{
			"bot_uuid": botUUID,
			"group_id": groupID,
			"platform": platform,
		},
	})
}

// LogWhitelistRemove logs when a group is removed from the whitelist
func (l *Logger) LogWhitelistRemove(botUUID, groupID, userID, platform string) {
	l.logger.Log(audit.Entry{
		Level:    audit.LevelInfo,
		Action:   "security.whitelist_remove",
		UserID:   userID,
		Success:  true,
		Message:  "Group removed from whitelist",
		Details: map[string]interface{}{
			"bot_uuid": botUUID,
			"group_id": groupID,
			"platform": platform,
		},
	})
}

// GetLogger returns the underlying audit logger
func (l *Logger) GetLogger() *audit.Logger {
	return l.logger
}

// ConsoleOnly returns an audit logger that only logs to console
func ConsoleOnly() *Logger {
	return NewLogger(audit.Config{
		Console: true,
	})
}

// Now returns the current time (convenience method)
func Now() time.Time {
	return time.Now().UTC()
}
