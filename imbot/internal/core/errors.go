package core

import (
	"fmt"
)

// BotError represents a bot error with additional context
type BotError struct {
	Code        ErrorCode
	Message     string
	Platform    Platform
	Recoverable bool
	Cause       error
	Context     map[string]interface{}
}

// Error returns the error message
func (e *BotError) Error() string {
	if e.Platform != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Platform, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause
func (e *BotError) Unwrap() error {
	return e.Cause
}

// NewBotError creates a new bot error
func NewBotError(code ErrorCode, message string, recoverable bool) *BotError {
	return &BotError{
		Code:        code,
		Message:     message,
		Recoverable: recoverable,
	}
}

// NewPlatformError creates a new platform-specific error
func NewPlatformError(platform Platform, message string, cause error, recoverable bool) *BotError {
	return &BotError{
		Code:        ErrPlatformError,
		Message:     message,
		Platform:    platform,
		Recoverable: recoverable,
		Cause:       cause,
	}
}

// NewAuthFailedError creates a new authentication failed error
func NewAuthFailedError(platform Platform, message string, cause error) *BotError {
	return &BotError{
		Code:        ErrAuthFailed,
		Message:     message,
		Platform:    platform,
		Recoverable: false,
		Cause:       cause,
	}
}

// NewConnectionFailedError creates a new connection failed error
func NewConnectionFailedError(platform Platform, message string, recoverable bool) *BotError {
	return &BotError{
		Code:        ErrConnectionFailed,
		Message:     message,
		Platform:    platform,
		Recoverable: recoverable,
	}
}

// NewRateLimitedError creates a new rate limited error
func NewRateLimitedError(platform Platform, retryAfter int) *BotError {
	ctx := map[string]interface{}{"retryAfter": retryAfter}
	msg := fmt.Sprintf("Rate limit exceeded for platform: %s", platform)
	if retryAfter > 0 {
		msg += fmt.Sprintf(" (retry after %ds)", retryAfter)
	}
	return &BotError{
		Code:        ErrRateLimited,
		Message:     msg,
		Platform:    platform,
		Recoverable: true,
		Context:     ctx,
	}
}

// NewMessageTooLongError creates a new message too long error
func NewMessageTooLongError(platform Platform, length, limit int) *BotError {
	return &BotError{
		Code:        ErrMessageTooLong,
		Message:     fmt.Sprintf("Message too long for platform %s: %d characters (limit: %d)", platform, length, limit),
		Platform:    platform,
		Recoverable: true,
		Context:     map[string]interface{}{"length": length, "limit": limit},
	}
}

// NewInvalidTargetError creates a new invalid target error
func NewInvalidTargetError(platform Platform, target, reason string) *BotError {
	msg := fmt.Sprintf("Invalid target for platform %s: %s", platform, target)
	if reason != "" {
		msg += fmt.Sprintf(" (%s)", reason)
	}
	return &BotError{
		Code:        ErrInvalidTarget,
		Message:     msg,
		Platform:    platform,
		Recoverable: false,
		Context:     map[string]interface{}{"target": target, "reason": reason},
	}
}

// NewMediaNotSupportedError creates a new media not supported error
func NewMediaNotSupportedError(platform Platform, mediaType string) *BotError {
	return &BotError{
		Code:        ErrMediaNotSupported,
		Message:     fmt.Sprintf("Media type '%s' not supported by platform: %s", mediaType, platform),
		Platform:    platform,
		Recoverable: false,
		Context:     map[string]interface{}{"mediaType": mediaType},
	}
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(platform Platform, operation string, timeoutMs int) *BotError {
	return &BotError{
		Code:        ErrTimeout,
		Message:     fmt.Sprintf("Timeout during %s for platform %s (%dms)", operation, platform, timeoutMs),
		Platform:    platform,
		Recoverable: true,
		Context:     map[string]interface{}{"operation": operation, "timeoutMs": timeoutMs},
	}
}

// IsBotError checks if an error is a BotError
func IsBotError(err error) bool {
	_, ok := err.(*BotError)
	return ok
}

// IsRecoverable checks if an error is recoverable
func IsRecoverable(err error) bool {
	if botErr, ok := err.(*BotError); ok {
		return botErr.Recoverable
	}
	return false
}

// GetErrorCode returns the error code from an error
func GetErrorCode(err error) ErrorCode {
	if botErr, ok := err.(*BotError); ok {
		return botErr.Code
	}
	return ErrUnknown
}

// WrapError wraps an error as a BotError if it isn't already
func WrapError(err error, platform Platform, fallbackCode ErrorCode) *BotError {
	if botErr, ok := err.(*BotError); ok {
		return botErr
	}

	if err == nil {
		return nil
	}

	return &BotError{
		Code:        fallbackCode,
		Message:     err.Error(),
		Platform:    platform,
		Recoverable: false,
		Cause:       err,
	}
}

// WithContext adds context to a bot error
func (e *BotError) WithContext(key string, value interface{}) *BotError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithCause sets the cause of the error
func (e *BotError) WithCause(cause error) *BotError {
	e.Cause = cause
	return e
}
