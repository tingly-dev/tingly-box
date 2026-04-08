package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/constant"
)

// generateSecret generates a random secret for JWT
func generateSecret() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func GenerateUserToken() (string, error) {
	return GenerateSecureToken("tb-user-")
}

func GenerateModelToken() (string, error) {
	return GenerateSecureToken("tb-model-")
}

// GenerateSecureToken generates a cryptographically random token for user authentication
// The token is a 256-bit (32 byte) random value, hex-encoded, with a "tingly-box-" prefix
func GenerateSecureToken(prefix string) (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return prefix + hex.EncodeToString(bytes), nil
}

// IsDefaultToken checks if the given token is the default token
func IsDefaultToken(token string) bool {
	return token == constant.DefaultUserToken
}

// GenerateUUID generates a new UUID string
func GenerateUUID() string {
	id, err := uuid.NewUUID()
	if err != nil {
		// Fallback to timestamp-based UUID if generation fails
		return fmt.Sprintf("uuid-%d", time.Now().UnixNano())
	}
	return id.String()
}
