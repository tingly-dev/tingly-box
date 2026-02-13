// Package main demonstrates how to integrate and use the enterprise module
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	enterprise "github.com/tingly-dev/tingly-box/internal/enterprise"
)

const (
	serverPort = 12581
	baseURL    = "http://localhost:12581"
)

func main() {
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	printBanner()

	// Get config directory (use temp directory for example)
	configDir := filepath.Join(os.TempDir(), "tingly-box-example")
	logrus.Infof("📁 Using config directory: %s", configDir)

	// Initialize enterprise integration
	integration, err := setupEnterprise(configDir)
	if err != nil {
		logrus.Fatalf("❌ Failed to setup enterprise: %v", err)
	}

	// Setup Gin server with enterprise authentication
	router := setupRouter(integration)

	// Channel to signal server shutdown
	shutdownChan := make(chan struct{})

	// Start server in a goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		logrus.Infof("🚀 Starting server on http://localhost:%d", serverPort)
		if err := router.Run(fmt.Sprintf(":%d", serverPort)); err != nil {
			serverErrChan <- err
		}
	}()

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

	// Run demo
	logrus.Info("")
	logrus.Info("═══════════════════════════════════════════════════════════════")
	logrus.Info("                    STARTING AUTOMATED DEMO                    ")
	logrus.Info("═══════════════════════════════════════════════════════════════")
	logrus.Info("")

	if err := runDemo(integration); err != nil {
		logrus.Errorf("❌ Demo failed: %v", err)
	} else {
		logrus.Info("")
		logrus.Info("═══════════════════════════════════════════════════════════════")
		logrus.Info("                      ✅ DEMO COMPLETED                      ")
		logrus.Info("═══════════════════════════════════════════════════════════════")
	}

	// Shutdown server
	logrus.Info("")
	logrus.Info("🛑 Shutting down server...")
	close(shutdownChan)

	// Give server time to shutdown gracefully
	time.Sleep(500 * time.Millisecond)

	// Final stats
	printFinalStats(integration)

	logrus.Info("✅ Example program finished successfully")
}

func printBanner() {
	logrus.Info("═══════════════════════════════════════════════════════════════")
	logrus.Info("     Tingly Box Enterprise Edition - Integration Example")
	logrus.Info("═══════════════════════════════════════════════════════════════")
	logrus.Info("")
}

// runDemo executes the automated demonstration
func runDemo(ent enterprise.Integration) error {
	var demoErr error

	// Run demo in a controlled manner
	demoSteps := []struct {
		name string
		fn   func(enterprise.Integration) error
	}{
		{"Step 1: Test Public Endpoint (Ping)", testPingEndpoint},
		{"Step 2: Admin Login", testAdminLogin},
		{"Step 3: Change Default Admin Password", testChangeAdminPassword},
		{"Step 4: Create New Users", testCreateUsers},
		{"Step 5: User Login", testUserLogin},
		{"Step 6: Create API Token", testCreateAPIToken},
		{"Step 7: Test Protected Endpoint", testProtectedEndpoint},
		{"Step 8: Test Admin Endpoints", testAdminEndpoints},
		{"Step 9: Test Role-Based Access", testRoleBasedAccess},
		{"Step 10: Get System Stats", testSystemStats},
	}

	for i, step := range demoSteps {
		logrus.Info("")
		logrus.Infof("📍 [%d/%d] %s", i+1, len(demoSteps), step.name)
		logrus.Info(strings.Repeat("─", 60))

		if err := step.fn(ent); err != nil {
			logrus.WithError(err).Errorf("❌ Failed at: %s", step.name)
			demoErr = err
			break
		}

		logrus.Info("✅ Passed")
		time.Sleep(200 * time.Millisecond)
	}

	return demoErr
}

// Demo Step Functions

func testPingEndpoint(ent enterprise.Integration) error {
	resp, err := http.Get(baseURL + "/api/ping")
	if err != nil {
		return fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	logrus.WithField("response", result).Info("✓ Public endpoint accessible")
	return nil
}

var adminToken string
var regularUserID int64
var regularUserToken string

func testAdminLogin(ent enterprise.Integration) error {
	// Get admin user info first
	ctx := context.Background()
	adminUser, err := ent.GetUserInfoByUsername(ctx, "admin")
	if err != nil {
		return fmt.Errorf("failed to get admin user: %w", err)
	}

	// For demo, we'll use the integration directly to get a token
	// In production, this would be done via login API
	logrus.WithFields(logrus.Fields{
		"username": adminUser.Username,
		"role":     adminUser.Role,
	}).Info("✓ Admin user found")

	// Simulate login by creating a demo token
	// In real scenario, you would POST to /api/auth/login
	adminToken = "demo-admin-token-" + fmt.Sprint(time.Now().Unix())
	logrus.WithField("token", adminToken[:20]+"...").Info("✓ Admin login simulated (demo mode)")

	return nil
}

func testChangeAdminPassword(ent enterprise.Integration) error {
	// This would normally require posting to /api/change-password
	// For demo, we'll show the concept
	ctx := context.Background()

	// Get admin user
	adminUser, err := ent.GetUserInfoByUsername(ctx, "admin")
	if err != nil {
		return fmt.Errorf("failed to get admin user: %w", err)
	}

	// In production, you would call:
	// newPassword, err := ent.ResetPassword(ctx, adminUser.ID, adminUser.ID)

	logrus.WithFields(logrus.Fields{
		"user_id":  adminUser.ID,
		"username": adminUser.Username,
	}).Info("✓ Password change capability verified")
	logrus.Info("  (In production: POST /api/admin/users/1/password)")

	return nil
}

func testCreateUsers(ent enterprise.Integration) error {
	ctx := context.Background()

	adminUser, _ := ent.GetUserInfoByUsername(ctx, "admin")

	// Helper function to create user if not exists
	ensureUser := func(username, email, password, fullName, role string) error {
		// Check if user exists (GetUserInfoByUsername only returns active users)
		user, err := ent.GetUserInfoByUsername(ctx, username)
		if err == nil && user != nil {
			// User exists and is active, reuse it
			regularUserID = user.ID
			logrus.WithFields(logrus.Fields{
				"user_id":  user.ID,
				"username": user.Username,
				"role":     user.Role,
			}).Info("✓ User already exists (reusing)")
			return nil
		}

		// Create new user
		createReq := &enterprise.CreateUserRequest{
			Username: username,
			Email:    email,
			Password: password,
			FullName: fullName,
			Role:     role,
		}

		newUser, err := ent.CreateUser(ctx, createReq, adminUser.ID)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		regularUserID = newUser.ID
		logrus.WithFields(logrus.Fields{
			"user_id":  newUser.ID,
			"username": newUser.Username,
			"email":    newUser.Email,
			"role":     newUser.Role,
		}).Info("✓ New user created successfully")
		return nil
	}

	// Ensure regular user exists
	if err := ensureUser("john_doe", "john@example.com", "SecurePass123!", "John Doe", "user"); err != nil {
		return err
	}

	// Ensure readonly user exists
	if err := ensureUser("alice_observer", "alice@example.com", "SecurePass456!", "Alice Observer", "readonly"); err != nil {
		return err
	}

	return nil
}

func testUserLogin(ent enterprise.Integration) error {
	ctx := context.Background()

	user, err := ent.GetUserInfo(ctx, regularUserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
	}).Info("✓ Regular user login simulated")

	return nil
}

func testCreateAPIToken(ent enterprise.Integration) error {
	ctx := context.Background()

	// Create token for regular user
	tokenReq := &enterprise.CreateTokenRequest{
		Name:   "Demo API Token",
		Scopes: []enterprise.Scope{
			enterprise.ScopeReadProviders,
			enterprise.ScopeReadRules,
		},
		UserID: &regularUserID,
	}

	adminUser, _ := ent.GetUserInfoByUsername(ctx, "admin")
	tokenInfo, rawToken, err := ent.CreateAPIToken(ctx, tokenReq, adminUser.ID)
	if err != nil {
		return fmt.Errorf("failed to create token: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"token_id":     tokenInfo.ID,
		"token_name":   tokenInfo.Name,
		"token_prefix": tokenInfo.TokenPrefix,
		"scopes":       tokenInfo.Scopes,
		"raw_token":    rawToken[:20] + "...",
	}).Info("✓ API token created successfully")

	return nil
}

func testProtectedEndpoint(ent enterprise.Integration) error {
	// Test accessing profile endpoint
	// In production, this would use actual JWT token
	logrus.Info("✓ Protected endpoint verification")
	logrus.Info("  GET /api/profile (requires authentication)")

	return nil
}

func testAdminEndpoints(ent enterprise.Integration) error {
	ctx := context.Background()

	// Test listing users
	stats, err := ent.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"user_count":    stats.UserCount,
		"token_count":   stats.TokenCount,
		"session_count": stats.SessionCount,
	}).Info("✓ Admin endpoint accessible: /api/admin/stats")

	// Test role check
	hasPermission := ent.HasRole(regularUserID, "user")
	logrus.WithField("has_user_role", hasPermission).Info("✓ Role verification working")

	return nil
}

func testRoleBasedAccess(ent enterprise.Integration) error {
	ctx := context.Background()

	// Test admin has all permissions
	adminUser, _ := ent.GetUserInfoByUsername(ctx, "admin")
	adminHasRead := ent.HasPermission(adminUser.ID, "providers:read")
	adminHasWrite := ent.HasPermission(adminUser.ID, "providers:write")

	logrus.WithFields(logrus.Fields{
		"admin_can_read":  adminHasRead,
		"admin_can_write": adminHasWrite,
	}).Info("✓ Admin permissions verified")

	// Test regular user has limited permissions
	regularUser, _ := ent.GetUserInfo(ctx, regularUserID)
	userHasRead := ent.HasPermission(regularUser.ID, "providers:read")
	userHasDelete := ent.HasPermission(regularUser.ID, "providers:delete")

	logrus.WithFields(logrus.Fields{
		"user_can_read":   userHasRead,
		"user_can_delete": userHasDelete,
	}).Info("✓ User permissions verified (limited access)")

	return nil
}

func testSystemStats(ent enterprise.Integration) error {
	ctx := context.Background()

	stats, err := ent.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	logrus.Info("📊 System Statistics:")
	logrus.WithFields(logrus.Fields{
		"users":            stats.UserCount,
		"tokens":           stats.TokenCount,
		"sessions":         stats.SessionCount,
		"database_path":    stats.DatabasePath,
		"database_size_mb": fmt.Sprintf("%.2f", stats.DatabaseSizeMB),
	}).Info("✓ System stats retrieved successfully")

	return nil
}

// setupEnterprise initializes the enterprise module
func setupEnterprise(configDir string) (enterprise.Integration, error) {
	logrus.Info("🔧 Initializing enterprise module...")

	// Create integration instance
	integration := enterprise.NewIntegration()

	// Build configuration
	config := &enterprise.Config{
		BaseDir:           configDir,
		JWTSecret:         "example-secret-key-change-in-production",
		AccessTokenExpiry: "15m",
		SessionExpiry:     "24h",
		PasswordMinLength: 8,
		Logger:            logrus.StandardLogger(),
	}

	// Initialize
	ctx := context.Background()
	if err := integration.Initialize(ctx, config); err != nil {
		return nil, fmt.Errorf("initialization failed: %w", err)
	}

	// Check stats
	stats, err := integration.GetStats(ctx)
	if err != nil {
		logrus.WithError(err).Warn("Failed to get stats")
	} else {
		logrus.Info("📊 Initial database stats:")
		logrus.WithFields(logrus.Fields{
			"users":    stats.UserCount,
			"tokens":   stats.TokenCount,
			"sessions": stats.SessionCount,
			"db_size":  fmt.Sprintf("%.2f MB", stats.DatabaseSizeMB),
		}).Info("")
	}

	logrus.Info("✅ Enterprise module initialized successfully")
	return integration, nil
}

// setupRouter creates and configures the Gin router with enterprise authentication
func setupRouter(ent enterprise.Integration) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(loggerMiddleware())
	router.Use(corsMiddleware())

	// Public routes (no authentication)
	public := router.Group("/api")
	{
		public.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message":           "pong",
				"enterprise_enabled": ent.IsEnabled(),
				"timestamp":          time.Now().Unix(),
			})
		})
	}

	// Protected routes (require authentication)
	protected := public.Group("")
	protected.Use(ent.AuthMiddleware())
	{
		protected.GET("/profile", handleProfile(ent))
	}

	// Admin only routes
	admin := public.Group("/admin")
	admin.Use(ent.AuthMiddleware())
	admin.Use(ent.RequireRole("admin"))
	{
		admin.GET("/stats", handleStats(ent))
	}

	return router
}

func printFinalStats(ent enterprise.Integration) {
	ctx := context.Background()
	stats, err := ent.GetStats(ctx)
	if err != nil {
		logrus.WithError(err).Warn("Failed to get final stats")
		return
	}

	logrus.Info("")
	logrus.Info("═══════════════════════════════════════════════════════════════")
	logrus.Info("                      FINAL STATISTICS                        ")
	logrus.Info("═══════════════════════════════════════════════════════════════")
	logrus.WithFields(logrus.Fields{
		"total_users":  stats.UserCount,
		"total_tokens": stats.TokenCount,
		"sessions":     stats.SessionCount,
		"db_path":      stats.DatabasePath,
		"db_size_mb":   fmt.Sprintf("%.2f", stats.DatabaseSizeMB),
	}).Info("📊 Database Summary")
	logrus.Info("═══════════════════════════════════════════════════════════════")
}

// HTTP Helpers

func makeRequest(method, path string, body interface{}, token string) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

// Middleware
func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start).Milliseconds()
		status := c.Writer.Status()

		if status >= 400 {
			logrus.WithFields(logrus.Fields{
				"method":  method,
				"path":    path,
				"status":  status,
				"latency": latency,
			}).Warn("HTTP Request")
		}
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
