package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/constant"
)

// Context1MSuffix is the model-name suffix Claude clients use to advertise
// the 1M context window (Claude Code strips it back off and sends the
// context-1m beta header; Claude Desktop picks names verbatim from /v1/models).
const Context1MSuffix = "[1m]"

// TrimContext1M removes the [1m] context-window suffix from a model name.
func TrimContext1M(model string) string {
	return strings.TrimSuffix(model, Context1MSuffix)
}

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
