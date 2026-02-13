package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

func TestPasswordService_HashPassword(t *testing.T) {
	config := DefaultPasswordConfig()
	svc := NewPasswordService(config)

	password := "TestPassword123!"

	hash, err := svc.HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)
	assert.Contains(t, hash, "$argon2id$")
}

func TestPasswordService_ValidatePassword(t *testing.T) {
	config := DefaultPasswordConfig()
	svc := NewPasswordService(config)

	password := "TestPassword123!"

	hash, err := svc.HashPassword(password)
	require.NoError(t, err)

	valid, err := svc.ValidatePassword(password, hash)
	require.NoError(t, err)
	assert.True(t, valid)

	// Test wrong password
	valid, err = svc.ValidatePassword("WrongPassword", hash)
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestPasswordService_ValidatePasswordStrength(t *testing.T) {
	config := DefaultPasswordConfig()
	svc := NewPasswordService(config)

	tests := []struct {
		name      string
		password  string
		wantErr   error
	}{
		{"valid password", "TestPass123", nil},
		{"too short", "Test1", ErrPasswordTooShort},
		{"no uppercase", "testpass123", ErrPasswordMissingUppercase},
		{"no lowercase", "TESTPASS123", ErrPasswordMissingLowercase},
		{"no digit", "TestPassword", ErrPasswordMissingDigit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ValidatePasswordStrength(tt.password)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPasswordService_GetPasswordStrength(t *testing.T) {
	config := DefaultPasswordConfig()
	svc := NewPasswordService(config)

	tests := []struct {
		name     string
		password string
		wantMin  PasswordStrength
	}{
		{"weak", "abc12345", PasswordWeak},
		{"fair", "Abc12345", PasswordFair},
		{"good", "Abcdefg123!", PasswordGood},
		{"strong", "Abcdefg123!@#", PasswordStrong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strength := svc.GetPasswordStrength(tt.password)
			assert.GreaterOrEqual(t, strength, tt.wantMin)
		})
	}
}

func TestPasswordService_GenerateRandomPassword(t *testing.T) {
	config := DefaultPasswordConfig()
	svc := NewPasswordService(config)

	password, err := svc.GenerateRandomPassword(16)
	require.NoError(t, err)
	assert.Len(t, password, 16)

	// Verify it meets strength requirements
	err = svc.ValidatePasswordStrength(password)
	assert.NoError(t, err)

	// Verify generated passwords are different
	password2, err := svc.GenerateRandomPassword(16)
	require.NoError(t, err)
	assert.NotEqual(t, password, password2)
}

func TestJWTService_GenerateToken(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	config := DefaultJWTConfig(secret)
	svc := NewJWTService(config)

	user := &db.User{
		ID:       1,
		UUID:     "test-uuid",
		Username: "testuser",
		Role:     db.RoleAdmin,
	}

	// Test access token generation
	accessToken, err := svc.GenerateAccessToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, accessToken)

	// Test refresh token generation
	refreshToken, err := svc.GenerateRefreshToken(user)
	require.NoError(t, err)
	assert.NotEmpty(t, refreshToken)

	// Tokens should be different
	assert.NotEqual(t, accessToken, refreshToken)
}

func TestJWTService_ValidateToken(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	config := DefaultJWTConfig(secret)
	svc := NewJWTService(config)

	user := &db.User{
		ID:       1,
		UUID:     "test-uuid",
		Username: "testuser",
		Role:     db.RoleAdmin,
	}

	token, err := svc.GenerateAccessToken(user)
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, user.Username, claims.Username)
	assert.Equal(t, string(user.Role), claims.Role)
	assert.Equal(t, string(TokenTypeAccess), claims.TokenType)
}

func TestJWTService_RefreshToken(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	config := DefaultJWTConfig(secret)
	svc := NewJWTService(config)

	user := &db.User{
		ID:       1,
		UUID:     "test-uuid",
		Username: "testuser",
		Role:     db.RoleAdmin,
	}

	// Generate refresh token
	refreshToken, err := svc.GenerateRefreshToken(user)
	require.NoError(t, err)

	// Validate refresh token
	claims, err := svc.ValidateRefreshToken(refreshToken)
	require.NoError(t, err)
	assert.Equal(t, string(TokenTypeRefresh), claims.TokenType)

	// Generate new access token from refresh token
	newAccessToken, err := svc.RefreshAccessToken(refreshToken, user)
	require.NoError(t, err)
	assert.NotEmpty(t, newAccessToken)

	// Validate new access token
	newClaims, err := svc.ValidateAccessToken(newAccessToken)
	require.NoError(t, err)
	assert.Equal(t, string(TokenTypeAccess), newClaims.TokenType)
}

func TestJWTService_TokenExpiry(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	config := DefaultJWTConfig(secret)
	config.AccessTokenExpiry = 1 * time.Millisecond // Very short expiry
	svc := NewJWTService(config)

	user := &db.User{
		ID:       1,
		UUID:     "test-uuid",
		Username: "testuser",
		Role:     db.RoleAdmin,
	}

	token, err := svc.GenerateAccessToken(user)
	require.NoError(t, err)

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	// Should fail validation
	_, err = svc.ValidateAccessToken(token)
	assert.Error(t, err)
}
