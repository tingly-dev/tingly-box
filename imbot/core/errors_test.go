package core

import (
	"errors"
	"testing"
)

func TestNewBotError(t *testing.T) {
	err := NewBotError(ErrAuthFailed, "authentication failed", false)

	if err.Code != ErrAuthFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrAuthFailed)
	}

	if err.Message != "authentication failed" {
		t.Errorf("Message = %v, want %v", err.Message, "authentication failed")
	}

	if err.Recoverable != false {
		t.Errorf("Recoverable = %v, want %v", err.Recoverable, false)
	}
}

func TestNewPlatformError(t *testing.T) {
	platform := PlatformTelegram
	cause := errors.New("connection error")
	err := NewPlatformError(platform, "failed to connect", cause, true)

	if err.Platform != platform {
		t.Errorf("Platform = %v, want %v", err.Platform, platform)
	}

	if err.Code != ErrPlatformError {
		t.Errorf("Code = %v, want %v", err.Code, ErrPlatformError)
	}

	if err.Recoverable != true {
		t.Errorf("Recoverable = %v, want %v", err.Recoverable, true)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}

	// Test Unwrap
	if errors.Unwrap(err) != cause {
		t.Error("Unwrap() should return the cause")
	}
}

func TestNewAuthFailedError(t *testing.T) {
	platform := PlatformTelegram
	cause := errors.New("invalid token")
	err := NewAuthFailedError(platform, "token expired", cause)

	if err.Code != ErrAuthFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrAuthFailed)
	}

	if err.Recoverable != false {
		t.Errorf("AuthFailedError should not be recoverable")
	}
}

func TestNewConnectionFailedError(t *testing.T) {
	platform := PlatformTelegram
	err := NewConnectionFailedError(platform, "network error", true)

	if err.Code != ErrConnectionFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrConnectionFailed)
	}

	if err.Recoverable != true {
		t.Errorf("ConnectionFailedError should be recoverable when specified")
	}
}

func TestNewRateLimitedError(t *testing.T) {
	platform := PlatformTelegram
	err := NewRateLimitedError(platform, 60)

	if err.Code != ErrRateLimited {
		t.Errorf("Code = %v, want %v", err.Code, ErrRateLimited)
	}

	if err.Recoverable != true {
		t.Errorf("RateLimitedError should be recoverable")
	}

	if err.Context == nil {
		t.Error("Context should not be nil")
	} else if retryAfter, ok := err.Context["retryAfter"].(int); !ok || retryAfter != 60 {
		t.Errorf("Context retryAfter = %v, want 60", err.Context["retryAfter"])
	}

	// Test error message
	if !containsSubstring(err.Error(), "retry after 60s") {
		t.Errorf("Error message should contain retry info: %v", err.Error())
	}
}

func TestNewMessageTooLongError(t *testing.T) {
	platform := PlatformTelegram
	length := 5000
	limit := 4096
	err := NewMessageTooLongError(platform, length, limit)

	if err.Code != ErrMessageTooLong {
		t.Errorf("Code = %v, want %v", err.Code, ErrMessageTooLong)
	}

	if err.Recoverable != true {
		t.Errorf("MessageTooLongError should be recoverable")
	}

	if err.Context == nil {
		t.Error("Context should not be nil")
	}

	// Test error message
	if !containsSubstring(err.Error(), "5000 characters") {
		t.Errorf("Error message should contain length: %v", err.Error())
	}
}

func TestNewInvalidTargetError(t *testing.T) {
	platform := PlatformTelegram
	target := "invalid-target"
	reason := "invalid chat ID format"
	err := NewInvalidTargetError(platform, target, reason)

	if err.Code != ErrInvalidTarget {
		t.Errorf("Code = %v, want %v", err.Code, ErrInvalidTarget)
	}

	if err.Recoverable != false {
		t.Errorf("InvalidTargetError should not be recoverable")
	}

	// Test without reason
	err2 := NewInvalidTargetError(platform, target, "")
	if err2.Recoverable != false {
		t.Errorf("InvalidTargetError should not be recoverable")
	}
}

func TestNewMediaNotSupportedError(t *testing.T) {
	platform := PlatformTelegram
	mediaType := "sticker"
	err := NewMediaNotSupportedError(platform, mediaType)

	if err.Code != ErrMediaNotSupported {
		t.Errorf("Code = %v, want %v", err.Code, ErrMediaNotSupported)
	}

	if err.Recoverable != false {
		t.Errorf("MediaNotSupportedError should not be recoverable")
	}

	// Test error message
	if !containsSubstring(err.Error(), mediaType) {
		t.Errorf("Error message should contain media type: %v", err.Error())
	}
}

func TestNewTimeoutError(t *testing.T) {
	platform := PlatformTelegram
	operation := "send message"
	timeoutMs := 5000
	err := NewTimeoutError(platform, operation, timeoutMs)

	if err.Code != ErrTimeout {
		t.Errorf("Code = %v, want %v", err.Code, ErrTimeout)
	}

	if err.Recoverable != true {
		t.Errorf("TimeoutError should be recoverable")
	}

	// Test error message
	if !containsSubstring(err.Error(), operation) || !containsSubstring(err.Error(), "5000ms") {
		t.Errorf("Error message should contain operation and timeout: %v", err.Error())
	}
}

func TestBotError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *BotError
		wantMsg string
	}{
		{
			name: "Error with platform",
			err: &BotError{
				Platform: PlatformTelegram,
				Code:     ErrAuthFailed,
				Message:  "auth error",
			},
			wantMsg: "[telegram]",
		},
		{
			name: "Error without platform",
			err: &BotError{
				Code:    ErrUnknown,
				Message: "unknown error",
			},
			wantMsg: "UNKNOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !containsSubstring(msg, tt.wantMsg) {
				t.Errorf("Error() = %v, want to contain %v", msg, tt.wantMsg)
			}
		})
	}
}

func TestIsBotError(t *testing.T) {
	botErr := NewBotError(ErrAuthFailed, "test", false)
	regularErr := errors.New("regular error")

	if !IsBotError(botErr) {
		t.Error("IsBotError(BotError) should return true")
	}

	if IsBotError(regularErr) {
		t.Error("IsBotError(regular error) should return false")
	}

	if IsBotError(nil) {
		t.Error("IsBotError(nil) should return false")
	}
}

func TestIsRecoverable(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantRecover bool
	}{
		{
			name:        "Recoverable error",
			err:         NewBotError(ErrConnectionFailed, "test", true),
			wantRecover: true,
		},
		{
			name:        "Non-recoverable error",
			err:         NewBotError(ErrAuthFailed, "test", false),
			wantRecover: false,
		},
		{
			name:        "Regular error",
			err:         errors.New("regular error"),
			wantRecover: false,
		},
		{
			name:        "Nil error",
			err:         nil,
			wantRecover: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRecoverable(tt.err); got != tt.wantRecover {
				t.Errorf("IsRecoverable() = %v, want %v", got, tt.wantRecover)
			}
		})
	}
}

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode ErrorCode
	}{
		{
			name:     "BotError",
			err:      NewBotError(ErrAuthFailed, "test", false),
			wantCode: ErrAuthFailed,
		},
		{
			name:     "Regular error",
			err:      errors.New("regular error"),
			wantCode: ErrUnknown,
		},
		{
			name:     "Nil error",
			err:      nil,
			wantCode: ErrUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetErrorCode(tt.err); got != tt.wantCode {
				t.Errorf("GetErrorCode() = %v, want %v", got, tt.wantCode)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	platform := PlatformTelegram
	regularErr := errors.New("regular error")
	botErr := NewBotError(ErrConnectionFailed, "connection failed", false)

	tests := []struct {
		name        string
		err         error
		wantWrapped bool
	}{
		{
			name:        "Wrap regular error",
			err:         regularErr,
			wantWrapped: true,
		},
		{
			name:        "Already BotError",
			err:         botErr,
			wantWrapped: false,
		},
		{
			name:        "Nil error",
			err:         nil,
			wantWrapped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapError(tt.err, platform, ErrPlatformError)

			if tt.err == nil {
				if wrapped != nil {
					t.Error("WrapError(nil) should return nil")
				}
				return
			}

			if !IsBotError(wrapped) {
				t.Error("Wrapped error should be a BotError")
			}

			if tt.wantWrapped && wrapped == botErr {
				t.Error("Should have wrapped the error, not returned the same BotError")
			}

			if !tt.wantWrapped && wrapped != botErr {
				t.Error("Should not have wrapped an existing BotError")
			}
		})
	}
}

func TestBotError_WithContext(t *testing.T) {
	err := NewBotError(ErrConnectionFailed, "test", true)

	result := err.WithContext("attempt", 3).WithContext("reason", "timeout")

	if result != err {
		t.Error("WithContext should return the same error for chaining")
	}

	if err.Context["attempt"] != 3 {
		t.Errorf("Context[attempt] = %v, want 3", err.Context["attempt"])
	}

	if err.Context["reason"] != "timeout" {
		t.Errorf("Context[reason] = %v, want timeout", err.Context["reason"])
	}
}

func TestBotError_WithCause(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewBotError(ErrPlatformError, "test", true)

	result := err.WithCause(cause)

	if result != err {
		t.Error("WithCause should return the same error")
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}

	if errors.Unwrap(err) != cause {
		t.Error("Unwrap() should return the cause")
	}
}
