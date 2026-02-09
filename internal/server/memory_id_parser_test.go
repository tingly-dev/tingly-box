package server

import (
	"testing"
)

func TestParseClaudeCodeUserID(t *testing.T) {
	tests := []struct {
		name          string
		userID        string
		wantUserID    string
		wantSessionID string
		wantErr       bool
	}{
		{
			name:          "Valid claude_code user_id",
			userID:        "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_81ca9881-6299-46c2-ae66-7bb28357034f",
			wantUserID:    "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907",
			wantSessionID: "81ca9881-6299-46c2-ae66-7bb28357034f",
			wantErr:       false,
		},
		{
			name:    "Invalid format - missing session",
			userID:  "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account",
			wantErr: true,
		},
		{
			name:    "Invalid format - wrong hash length",
			userID:  "user_abc123_account__session_81ca9881-6299-46c2-ae66-7bb28357034f",
			wantErr: true,
		},
		{
			name:    "Invalid format - not a UUID",
			userID:  "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_not-a-uuid",
			wantErr: true,
		},
		{
			name:    "Empty string",
			userID:  "",
			wantErr: true,
		},
		{
			name:    "Random string",
			userID:  "random_string_12345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseClaudeCodeUserID(tt.userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClaudeCodeUserID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if parsed.UserID != tt.wantUserID {
					t.Errorf("UserID = %v, want %v", parsed.UserID, tt.wantUserID)
				}
				if parsed.SessionID != tt.wantSessionID {
					t.Errorf("SessionID = %v, want %v", parsed.SessionID, tt.wantSessionID)
				}
			}
		})
	}
}

func TestTryParseClaudeCodeUserID(t *testing.T) {
	validID := "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_81ca9881-6299-46c2-ae66-7bb28357034f"
	invalidID := "invalid_format"

	// Valid ID should return parsed result
	if parsed := TryParseClaudeCodeUserID(validID); parsed == nil {
		t.Error("TryParseClaudeCodeUserID(validID) should not return nil")
	} else {
		if parsed.UserID != "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907" {
			t.Errorf("UserID = %v, want %v", parsed.UserID, "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907")
		}
		if parsed.SessionID != "81ca9881-6299-46c2-ae66-7bb28357034f" {
			t.Errorf("SessionID = %v, want %v", parsed.SessionID, "81ca9881-6299-46c2-ae66-7bb28357034f")
		}
	}

	// Invalid ID should return nil
	if parsed := TryParseClaudeCodeUserID(invalidID); parsed != nil {
		t.Error("TryParseClaudeCodeUserID(invalidID) should return nil")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id := GenerateSessionID()
	if id == "" {
		t.Error("GenerateSessionID() should not return empty string")
	}
	// UUID format: 8-4-4-4-12
	if len(id) != 36 {
		t.Errorf("GenerateSessionID() length = %v, want 36", len(id))
	}
}

func TestGenerateUserID(t *testing.T) {
	id := GenerateUserID()
	if id == "" {
		t.Error("GenerateUserID() should not return empty string")
	}
	// UUID format: 8-4-4-4-12
	if len(id) != 36 {
		t.Errorf("GenerateUserID() length = %v, want 36", len(id))
	}
}

func TestResolveSessionID(t *testing.T) {
	claudeCodeID := "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_81ca9881-6299-46c2-ae66-7bb28357034f"
	randomID := "some_random_user_id"

	// claude_code format should return extracted session ID
	if got := ResolveSessionID(claudeCodeID); got != "81ca9881-6299-46c2-ae66-7bb28357034f" {
		t.Errorf("ResolveSessionID(claudeCodeID) = %v, want 81ca9881-6299-46c2-ae66-7bb28357034f", got)
	}

	// Non-matching format should generate a new UUID (different from input)
	if got := ResolveSessionID(randomID); got == randomID {
		t.Error("ResolveSessionID(randomID) should generate a UUID, not return the input")
	}

	// Empty string should generate a UUID
	if got := ResolveSessionID(""); got == "" {
		t.Error("ResolveSessionID(\"\") should generate a UUID")
	}
}

func TestResolveUserID(t *testing.T) {
	claudeCodeID := "user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_81ca9881-6299-46c2-ae66-7bb28357034f"
	randomID := "some_other_user_id"

	// claude_code format should return extracted user ID hash
	if got := ResolveUserID(claudeCodeID); got != "5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907" {
		t.Errorf("ResolveUserID(claudeCodeID) = %v, want 5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907", got)
	}

	// Non-matching format should return original value
	if got := ResolveUserID(randomID); got != randomID {
		t.Errorf("ResolveUserID(randomID) = %v, want %v", got, randomID)
	}

	// Empty string should generate a UUID
	if got := ResolveUserID(""); got == "" {
		t.Error("ResolveUserID(\"\") should generate a UUID")
	}
}
