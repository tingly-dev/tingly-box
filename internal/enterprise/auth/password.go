package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/argon2"
)

var (
	// ErrPasswordTooShort is returned when password is too short
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	// ErrPasswordTooLong is returned when password is too long
	ErrPasswordTooLong = errors.New("password must be at most 128 characters")
	// ErrPasswordMissingUppercase is returned when password has no uppercase
	ErrPasswordMissingUppercase = errors.New("password must contain at least one uppercase letter")
	// ErrPasswordMissingLowercase is returned when password has no lowercase
	ErrPasswordMissingLowercase = errors.New("password must contain at least one lowercase letter")
	// ErrPasswordMissingDigit is returned when password has no digit
	ErrPasswordMissingDigit = errors.New("password must contain at least one digit")
)

// PasswordStrength represents the strength of a password
type PasswordStrength int

const (
	PasswordWeak   PasswordStrength = iota
	PasswordFair   PasswordStrength = iota
	PasswordGood   PasswordStrength = iota
	PasswordStrong PasswordStrength = iota
)

// PasswordConfig configures password hashing parameters
type PasswordConfig struct {
	Time    uint32 // Time cost
	Memory  uint32 // Memory cost in KB
	Threads uint8  // Number of threads
	KeyLen  uint32 // Key length (output hash length)
	SaltLen uint32 // Salt length
}

// DefaultPasswordConfig returns the default password configuration
func DefaultPasswordConfig() PasswordConfig {
	return PasswordConfig{
		Time:    3,
		Memory:  64 * 1024, // 64 MB
		Threads: 4,
		KeyLen:  32,
		SaltLen: 16,
	}
}

// PasswordService handles password hashing and validation
type PasswordService struct {
	config PasswordConfig
}

// NewPasswordService creates a new password service
func NewPasswordService(config PasswordConfig) *PasswordService {
	return &PasswordService{config: config}
}

// HashPassword hashes a password using Argon2id
func (s *PasswordService) HashPassword(password string) (string, error) {
	// Generate a random salt
	salt := make([]byte, s.config.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Hash the password with Argon2id
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		s.config.Time,
		s.config.Memory,
		s.config.Threads,
		s.config.KeyLen,
	)

	// Encode salt and hash to base64
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	// Format: $argon2id$v=19$t=<time>,m=<memory>,p=<threads>$<salt>$<hash>
	encoded := fmt.Sprintf(
		"$argon2id$v=19$t=%d,m=%d,p=%d$%s$%s",
		s.config.Time,
		s.config.Memory,
		s.config.Threads,
		saltB64,
		hashB64,
	)

	return encoded, nil
}

// ValidatePassword validates a password against a hash
func (s *PasswordService) ValidatePassword(password, encodedHash string) (bool, error) {
	// Parse the encoded hash
	params, salt, hash, err := parseArgon2Hash(encodedHash)
	if err != nil {
		return false, err
	}

	// Hash the provided password with the same parameters
	otherHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.time,
		params.memory,
		params.threads,
		params.keyLen,
	)

	// Compare hashes
	return compareHashes(hash, otherHash), nil
}

// ValidatePasswordStrength checks if a password meets strength requirements
func (s *PasswordService) ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 {
		return ErrPasswordTooLong
	}

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)

	if !hasUpper {
		return ErrPasswordMissingUppercase
	}
	if !hasLower {
		return ErrPasswordMissingLowercase
	}
	if !hasDigit {
		return ErrPasswordMissingDigit
	}

	return nil
}

// GetPasswordStrength returns the strength of a password
func (s *PasswordService) GetPasswordStrength(password string) PasswordStrength {
	score := 0

	// Length check
	if len(password) >= 8 {
		score++
	}
	if len(password) >= 12 {
		score++
	}
	if len(password) >= 16 {
		score++
	}

	// Character variety
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password)

	if hasUpper && hasLower {
		score++
	}
	if hasDigit {
		score++
	}
	if hasSpecial {
		score++
	}

	// Determine strength
	if score <= 2 {
		return PasswordWeak
	} else if score <= 4 {
		return PasswordFair
	} else if score <= 6 {
		return PasswordGood
	}
	return PasswordStrong
}

// GenerateRandomPassword generates a random password
func (s *PasswordService) GenerateRandomPassword(length int) (string, error) {
	if length < 8 {
		length = 8
	}
	if length > 128 {
		length = 128
	}

	// Character sets
	upper := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lower := "abcdefghijklmnopqrstuvwxyz"
	digits := "0123456789"
	special := "!@#$%^&*()_+-=[]{}|;:,.<>?"

	all := upper + lower + digits + special

	password := make([]byte, length)

	// Ensure at least one of each required type
	password[0] = upper[randomInt(len(upper))]
	password[1] = lower[randomInt(len(lower))]
	password[2] = digits[randomInt(len(digits))]
	password[3] = special[randomInt(len(special))]

	// Fill the rest randomly
	for i := 4; i < length; i++ {
		password[i] = all[randomInt(len(all))]
	}

	// Shuffle the password
	for i := len(password) - 1; i > 0; i-- {
		j := randomInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}

// argon2Params holds parsed Argon2 parameters
type argon2Params struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
}

// parseArgon2Hash parses an Argon2id hash string
func parseArgon2Hash(encodedHash string) (params argon2Params, salt, hash []byte, err error) {
	// Format: $argon2id$v=19$t=<time>,m=<memory>,p=<threads>$<salt>$<hash>
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return argon2Params{}, nil, nil, errors.New("invalid hash format")
	}

	if parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, fmt.Errorf("unsupported algorithm: %s", parts[1])
	}

	// Parse version
	if parts[2] != "v=19" {
		return argon2Params{}, nil, nil, fmt.Errorf("unsupported version: %s", parts[2])
	}

	// Parse parameters
	paramParts := strings.Split(parts[3], ",")
	if len(paramParts) != 3 {
		return argon2Params{}, nil, nil, errors.New("invalid parameters format")
	}

	params = argon2Params{}
	for _, param := range paramParts {
		kv := strings.Split(param, "=")
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			fmt.Sscanf(kv[1], "%d", &params.time)
		case "m":
			fmt.Sscanf(kv[1], "%d", &params.memory)
		case "p":
			fmt.Sscanf(kv[1], "%d", &params.threads)
		}
	}
	params.keyLen = 32 // Default key length

	// Decode salt
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	// Decode hash
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("failed to decode hash: %w", err)
	}

	return params, salt, hash, nil
}

// compareHashes compares two hash slices in constant time
func compareHashes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}

	return result == 0
}

// randomInt generates a cryptographically secure random integer
func randomInt(max int) int {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return int(b[0]) % max
}
