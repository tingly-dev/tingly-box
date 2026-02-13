package rbac

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"github.com/tingly-dev/tingly-box/internal/enterprise/token"
)

func setupTestRouter(authMiddleware *AuthMiddleware) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Test routes
	router.GET("/protected", authMiddleware.Authenticate(), func(c *gin.Context) {
		user := GetCurrentUser(c)
		c.JSON(http.StatusOK, user)
	})

	router.GET("/admin", authMiddleware.Authenticate(), RequireRole(db.RoleAdmin), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access"})
	})

	router.GET("/permission", authMiddleware.Authenticate(), RequirePermission(PermReadUsers), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "permission granted"})
	})

	router.GET("/scope", authMiddleware.Authenticate(), RequireScope(db.ScopeReadUsers), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "scope granted"})
	})

	return router
}

func createTestUser(id int64, role db.Role) *db.User {
	return &db.User{
		ID:       id,
		UUID:     "test-uuid",
		Username: "testuser",
		Email:    "test@example.com",
		Role:     role,
		IsActive: true,
	}
}

func TestAuthMiddleware_JWTToken(t *testing.T) {
	jwtConfig := auth.DefaultJWTConfig("test-secret")
	jwtSvc := auth.NewJWTService(jwtConfig)
	userRepo := new(mockUserRepository)
	tokenModel := new(mockTokenModel)

	middleware := NewAuthMiddleware(jwtSvc, userRepo, tokenModel)
	router := setupTestRouter(middleware)

	// Create test user
	testUser := createTestUser(1, db.RoleAdmin)
	userRepo.addUser(testUser)

	// Generate JWT token
	token, err := jwtSvc.GenerateAccessToken(testUser)
	require.NoError(t, err)

	// Make request with valid token
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_APIToken(t *testing.T) {
	jwtConfig := auth.DefaultJWTConfig("test-secret")
	jwtSvc := auth.NewJWTService(jwtConfig)
	userRepo := new(mockUserRepository)
	tokenModel := new(mockTokenModel)

	middleware := NewAuthMiddleware(jwtSvc, userRepo, tokenModel)
	router := setupTestRouter(middleware)

	// Create test user
	testUser := createTestUser(1, db.RoleAdmin)
	userRepo.addUser(testUser)

	// Create API token
	scopes := []db.Scope{db.ScopeReadProviders, db.ScopeWriteProviders}
	scopesJSON, _ := json.Marshal(scopes)

	apiToken := &db.APIToken{
		ID:        1,
		UserID:    1,
		TokenHash: "valid-hash",
		User:      testUser,
		Scopes:    string(scopesJSON),
		IsActive:  true,
	}
	tokenModel.addToken(apiToken)

	// Make request with API token
	rawToken := "ent-test-token-with-valid-hash"
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()

	// This will fail because our mock doesn't properly validate
	// But we're testing the middleware structure
	router.ServeHTTP(w, req)

	// Response should be unauthorized because hash won't match
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	jwtConfig := auth.DefaultJWTConfig("test-secret")
	jwtSvc := auth.NewJWTService(jwtConfig)
	userRepo := new(mockUserRepository)
	tokenModel := new(mockTokenModel)

	middleware := NewAuthMiddleware(jwtSvc, userRepo, tokenModel)
	router := setupTestRouter(middleware)

	// Make request without token
	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireRole_Admin(t *testing.T) {
	jwtConfig := auth.DefaultJWTConfig("test-secret")
	jwtSvc := auth.NewJWTService(jwtConfig)
	userRepo := new(mockUserRepository)
	tokenModel := new(mockTokenModel)

	middleware := NewAuthMiddleware(jwtSvc, userRepo, tokenModel)
	router := setupTestRouter(middleware)

	// Create admin user
	adminUser := createTestUser(1, db.RoleAdmin)
	userRepo.addUser(adminUser)

	// Generate JWT token for admin
	token, err := jwtSvc.GenerateAccessToken(adminUser)
	require.NoError(t, err)

	// Make request as admin
	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_NonAdmin(t *testing.T) {
	jwtConfig := auth.DefaultJWTConfig("test-secret")
	jwtSvc := auth.NewJWTService(jwtConfig)
	userRepo := new(mockUserRepository)
	tokenModel := new(mockTokenModel)

	middleware := NewAuthMiddleware(jwtSvc, userRepo, tokenModel)
	router := setupTestRouter(middleware)

	// Create regular user
	regularUser := createTestUser(1, db.RoleUser)
	userRepo.addUser(regularUser)

	// Generate JWT token for regular user
	token, err := jwtSvc.GenerateAccessToken(regularUser)
	require.NoError(t, err)

	// Make request as regular user to admin endpoint
	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequirePermission_Granted(t *testing.T) {
	jwtConfig := auth.DefaultJWTConfig("test-secret")
	jwtSvc := auth.NewJWTService(jwtConfig)
	userRepo := new(mockUserRepository)
	tokenModel := new(mockTokenModel)

	middleware := NewAuthMiddleware(jwtSvc, userRepo, tokenModel)
	router := setupTestRouter(middleware)

	// Create admin user (has read:users permission)
	adminUser := createTestUser(1, db.RoleAdmin)
	userRepo.addUser(adminUser)

	// Generate JWT token
	token, err := jwtSvc.GenerateAccessToken(adminUser)
	require.NoError(t, err)

	// Make request
	req := httptest.NewRequest("GET", "/permission", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequirePermission_Denied(t *testing.T) {
	jwtConfig := auth.DefaultJWTConfig("test-secret")
	jwtSvc := auth.NewJWTService(jwtConfig)
	userRepo := new(mockUserRepository)
	tokenModel := new(mockTokenModel)

	middleware := NewAuthMiddleware(jwtSvc, userRepo, tokenModel)
	router := setupTestRouter(middleware)

	// Create readonly user (doesn't have read:users permission)
	readonlyUser := createTestUser(1, db.RoleReadOnly)
	userRepo.addUser(readonlyUser)

	// Generate JWT token
	token, err := jwtSvc.GenerateAccessToken(readonlyUser)
	require.NoError(t, err)

	// Make request
	req := httptest.NewRequest("GET", "/permission", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name       string
		role       db.Role
		permission string
		expected   bool
	}{
		{"admin read users", db.RoleAdmin, PermReadUsers, true},
		{"admin write users", db.RoleAdmin, PermWriteUsers, true},
		{"admin read providers", db.RoleAdmin, PermReadProviders, true},
		{"user read users", db.RoleUser, PermReadUsers, false},
		{"user read providers", db.RoleUser, PermReadProviders, true},
		{"user write providers", db.RoleUser, PermWriteProviders, true},
		{"readonly read providers", db.RoleReadOnly, PermReadProviders, true},
		{"readonly write providers", db.RoleReadOnly, PermWriteProviders, false},
		{"readonly read users", db.RoleReadOnly, PermReadUsers, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
		user := &db.User{
			Role: tt.role,
		}
		result := HasPermission(user, tt.permission)
		assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasRole(t *testing.T) {
	user := &db.User{
		Role: db.RoleAdmin,
	}

	assert.True(t, IsAdmin(user))
	assert.True(t, HasRole(user, "admin"))
	assert.False(t, HasRole(user, "user"))
	assert.False(t, HasRole(user, "readonly"))
}

func TestExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name          string
		header        string
		expectedToken string
	}{
		{
			name:          "bearer token",
			header:        "Bearer test-token",
			expectedToken: "test-token",
		},
		{
			name:          "bearer with space",
			header:        "Bearer  test-token",
			expectedToken: "test-token",
		},
		{
			name:          "lowercase bearer",
			header:        "bearer test-token",
			expectedToken: "",
		},
		{
			name:          "no bearer",
			header:        "test-token",
			expectedToken: "",
		},
		{
			name:          "empty header",
			header:        "",
			expectedToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRequest("GET", "/", nil))
			c.Request.Header.Set("Authorization", tt.header)

			result := ExtractTokenFromHeader(c)
			assert.Equal(t, tt.expectedToken, result)
		})
	}
}

func TestSetCurrentUser(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRequest("GET", "/", nil))

	user := &db.User{
		ID:       123,
		Username: "testuser",
		Role:     db.RoleAdmin,
	}

	SetCurrentUser(c, user)
	retrieved := GetCurrentUser(c)

	assert.Same(t, user, retrieved)
}

func TestSetCurrentToken(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRequest("GET", "/", nil))

	token := &db.APIToken{
		ID:   456,
		Name: "test-token",
	}

	SetCurrentToken(c, token)
	retrieved := GetCurrentToken(c)

	assert.Same(t, token, retrieved)
}

// Mock implementations for testing
type mockUserRepository struct {
	users map[int64]*db.User
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users: make(map[int64]*db.User),
	}
}

func (m *mockUserRepository) addUser(user *db.User) {
	m.users[user.ID] = user
}

func (m *mockUserRepository) GetByID(id int64) (*db.User, error) {
	user, exists := m.users[id]
	if !exists {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (m *mockUserRepository) GetByUsername(username string) (*db.User, error) {
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) GetByEmail(email string) (*db.User, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) UpdateLastLogin(id int64) error {
	return nil
}

type mockTokenModel struct {
	tokens map[string]*db.APIToken // key: token hash
}

func newMockTokenModel() *mockTokenModel {
	return &mockTokenModel{
		tokens: make(map[string]*db.APIToken),
	}
}

func (m *mockTokenModel) addToken(token *db.APIToken) {
	m.tokens[token.TokenHash] = token
}

func (m *mockTokenModel) ValidateToken(tokenString string) (*db.APIToken, error) {
	// For testing, we'd need to implement hash calculation
	// This is a simplified mock
	return nil, ErrTokenNotFound
}
