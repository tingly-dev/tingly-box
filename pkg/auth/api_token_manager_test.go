package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPITokenManager(t *testing.T) {
	tests := []struct {
		name    string
		config  APITokenManagerConfig
		wantErr bool
	}{
		{
			name: "valid HS256 config",
			config: APITokenManagerConfig{
				SecretKey:     "test-secret-key",
				SigningMethod: "HS256",
				Issuer:        "tingly-box",
			},
			wantErr: false,
		},
		{
			name: "valid RS256 config",
			config: APITokenManagerConfig{
				SecretKey:     "test-secret-key",
				SigningMethod: "RS256",
				Issuer:        "tingly-box",
			},
			wantErr: false,
		},
		{
			name: "default signing method",
			config: APITokenManagerConfig{
				SecretKey: "test-secret-key",
			},
			wantErr: false,
		},
		{
			name: "empty secret key",
			config: APITokenManagerConfig{
				SecretKey: "",
			},
			wantErr: true,
		},
		{
			name: "unsupported signing method",
			config: APITokenManagerConfig{
				SecretKey:     "test-secret-key",
				SigningMethod: "HS512",
			},
			wantErr: true,
		},
		{
			name: "default issuer",
			config: APITokenManagerConfig{
				SecretKey: "test-secret-key",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewAPITokenManager(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, mgr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, mgr)
				if mgr != nil {
					assert.Equal(t, "tingly-box", mgr.issuer)
				}
			}
		})
	}
}

func TestAPITokenManager_GenerateToken(t *testing.T) {
	mgr, err := NewAPITokenManager(APITokenManagerConfig{
		SecretKey:     "test-secret-key",
		SigningMethod: "HS256",
		Issuer:        "tingly-box",
	})
	require.NoError(t, err)

	tests := []struct {
		name       string
		userUUID   string
		tokenID    string
		expiresAt  time.Time
		wantErr    bool
		errMessage string
	}{
		{
			name:      "valid token",
			userUUID:  "user-123",
			tokenID:   "token-abc",
			expiresAt: time.Now().Add(24 * time.Hour),
			wantErr:   false,
		},
		{
			name:      "empty user UUID",
			userUUID:  "",
			tokenID:   "token-abc",
			expiresAt: time.Now().Add(24 * time.Hour),
			wantErr:   true,
			errMessage: "user UUID cannot be empty",
		},
		{
			name:      "empty token ID",
			userUUID:  "user-123",
			tokenID:   "",
			expiresAt: time.Now().Add(24 * time.Hour),
			wantErr:   true,
			errMessage: "token ID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := mgr.GenerateToken(tt.userUUID, tt.tokenID, tt.expiresAt)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
			}
		})
	}
}

func TestAPITokenManager_ValidateToken(t *testing.T) {
	secret := "test-secret-key"
	mgr, err := NewAPITokenManager(APITokenManagerConfig{
		SecretKey:     secret,
		SigningMethod: "HS256",
		Issuer:        "tingly-box",
	})
	require.NoError(t, err)

	// Generate a valid token
	userUUID := "user-123"
	tokenID := "token-abc"
	expiresAt := time.Now().Add(24 * time.Hour)
	validToken, err := mgr.GenerateToken(userUUID, tokenID, expiresAt)
	require.NoError(t, err)

	// Generate an expired token
	expiredMgr, _ := NewAPITokenManager(APITokenManagerConfig{
		SecretKey:     secret,
		SigningMethod: "HS256",
		Issuer:        "tingly-box",
	})
	expiredToken, _ := expiredMgr.GenerateToken(userUUID, tokenID, time.Now().Add(-1*time.Hour))

	tests := []struct {
		name      string
		token     string
		wantErr   bool
		checkUser string
		checkID   string
	}{
		{
			name:    "valid token",
			token:   validToken,
			wantErr: false,
			checkUser: userUUID,
			checkID: tokenID,
		},
		{
			name:    "expired token",
			token:   expiredToken,
			wantErr: true,
		},
		{
			name:    "invalid token format",
			token:   "invalid.jwt.token",
			wantErr: true,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "token with wrong secret",
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX3V1aWQiOiJ1c2VyLTEyMyIsInRva2VuX2lkIjoidG9rZW4tYWJjIiwiaXNzIjoidGluZ2x5LWJveCIsImV4cCI6MTc0NjQwMDAwMCwiaWF0IjoxNzQ2MzEzNjAwLCJub2JlIjoxNzQ2MzEzNjAwfQ.wrong",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := mgr.ValidateToken(tt.token)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, claims)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
				assert.Equal(t, tt.checkUser, claims.UserUUID)
				assert.Equal(t, tt.checkID, claims.TokenID)
				assert.Equal(t, "tingly-box", claims.Issuer)
			}
		})
	}
}

func TestAPITokenManager_RS256Signing(t *testing.T) {
	// Note: RS256 requires an actual RSA key pair.
	// This test verifies that the manager correctly configures RS256 signing method.
	// For actual token generation with RS256, you need a real RSA private key.
	mgr, err := NewAPITokenManager(APITokenManagerConfig{
		SecretKey:     "test-secret-key",
		SigningMethod: "RS256",
		Issuer:        "test-issuer",
	})
	require.NoError(t, err)

	// Verify the signing method is correctly set
	method := mgr.getSigningMethod()
	assert.Equal(t, "RS256", method.Alg())
	assert.NotNil(t, method)
}

func TestAPITokenManager_GetIssuer(t *testing.T) {
	tests := []struct {
		name   string
		issuer string
	}{
		{
			name:   "default issuer",
			issuer: "tingly-box",
		},
		{
			name:   "custom issuer",
			issuer: "my-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewAPITokenManager(APITokenManagerConfig{
				SecretKey: "test-secret-key",
				Issuer:    tt.issuer,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.issuer, mgr.GetIssuer())
		})
	}
}
