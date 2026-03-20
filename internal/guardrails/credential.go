package guardrails

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProtectedCredential stores a real secret and the pseudonym token exposed to the model.
// The real secret stays local; policy config and history should only refer to IDs or alias tokens.
type ProtectedCredential struct {
	ID          string    `json:"id" yaml:"id"`
	Name        string    `json:"name" yaml:"name"`
	Type        string    `json:"type" yaml:"type"`
	Secret      string    `json:"secret" yaml:"secret"`
	AliasToken  string    `json:"alias_token" yaml:"alias_token"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty" yaml:"tags,omitempty"`
	Enabled     bool      `json:"enabled" yaml:"enabled"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" yaml:"updated_at"`
}

const (
	ProtectedCredentialTypeAPIKey     = "api_key"
	ProtectedCredentialTypeToken      = "token"
	ProtectedCredentialTypePrivateKey = "private_key"
	AliasTokenPrefix                  = "TINGLY_SECRET_"
)

var protectedCredentialTypes = map[string]struct{}{
	ProtectedCredentialTypeAPIKey:     {},
	ProtectedCredentialTypeToken:      {},
	ProtectedCredentialTypePrivateKey: {},
}

func IsSupportedProtectedCredentialType(value string) bool {
	_, ok := protectedCredentialTypes[strings.TrimSpace(value)]
	return ok
}

func NewProtectedCredential(name, credentialType, secret, description string, tags []string, enabled bool) (ProtectedCredential, error) {
	if strings.TrimSpace(name) == "" {
		return ProtectedCredential{}, fmt.Errorf("credential name is required")
	}
	if !IsSupportedProtectedCredentialType(credentialType) {
		return ProtectedCredential{}, fmt.Errorf("unsupported credential type %q", credentialType)
	}
	if strings.TrimSpace(secret) == "" {
		return ProtectedCredential{}, fmt.Errorf("credential secret is required")
	}

	aliasToken, err := GenerateAliasToken()
	if err != nil {
		return ProtectedCredential{}, err
	}

	now := time.Now().UTC()
	return ProtectedCredential{
		ID:          uuid.NewString(),
		Name:        strings.TrimSpace(name),
		Type:        strings.TrimSpace(credentialType),
		Secret:      secret,
		AliasToken:  aliasToken,
		Description: strings.TrimSpace(description),
		Tags:        normalizeStringSlice(tags),
		Enabled:     enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func GenerateAliasToken() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate alias token: %w", err)
	}
	return AliasTokenPrefix + strings.ToUpper(hex.EncodeToString(buf)), nil
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func MaskedSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}
