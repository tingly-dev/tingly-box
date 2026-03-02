package middleware

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// AuthMiddleware provides authentication middleware for different types of authentication
type AuthMiddleware struct {
	config     *config.Config
	jwtManager *auth.JWTManager
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(cfg *config.Config, jwtManager *auth.JWTManager) *AuthMiddleware {
	return &AuthMiddleware{
		config:     cfg,
		jwtManager: jwtManager,
	}
}

func hashVirtualKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeScenario(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	aliases := map[string]string{
		"claude":        "claude_code",
		"claudecode":    "claude_code",
		"open_code":     "opencode",
		"openai_sdk":    "openai",
		"anthropic_sdk": "anthropic",
	}
	if mapped, ok := aliases[v]; ok {
		return mapped
	}
	return v
}

func inferScenarioFromRequest(c *gin.Context) string {
	if scenario := strings.TrimSpace(c.Param("scenario")); scenario != "" {
		return normalizeScenario(scenario)
	}
	path := strings.ToLower(c.Request.URL.Path)
	switch {
	case strings.Contains(path, "/passthrough/openai"):
		return "openai"
	case strings.Contains(path, "/passthrough/anthropic"):
		return "anthropic"
	default:
		return ""
	}
}

func resolveEnterpriseDBPath() string {
	seen := map[string]struct{}{}
	add := func(candidates *[]string, candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		cleaned := filepath.Clean(candidate)
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		*candidates = append(*candidates, cleaned)
	}
	addParents := func(candidates *[]string, base string, depth int) {
		current := filepath.Clean(base)
		for i := 0; i <= depth; i++ {
			add(candidates, filepath.Join(current, "data", "enterprise.db"))
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	var candidates []string
	// Highest priority: explicit env override for db file.
	if envPath := strings.TrimSpace(os.Getenv("TBE_ENTERPRISE_DB_PATH")); envPath != "" {
		add(&candidates, envPath)
	}
	// Next: explicit data dir override, if provided.
	if envDataDir := strings.TrimSpace(os.Getenv("TBE_DATA_DIR")); envDataDir != "" {
		add(&candidates, filepath.Join(envDataDir, "enterprise.db"))
	}

	// Search from cwd and parent directories (covers service launched from subfolders).
	if cwd, err := os.Getwd(); err == nil {
		addParents(&candidates, cwd, 6)
	}
	// Search around executable path as fallback.
	if exe, err := os.Executable(); err == nil {
		addParents(&candidates, filepath.Dir(exe), 6)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func parseAllowedModels(raw string) []string {
	var out []string
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func scenarioAllowed(allowed []string, scenario string) bool {
	if scenario == "" {
		return true
	}
	target := normalizeScenario(scenario)
	for _, item := range allowed {
		if normalizeScenario(item) == target {
			return true
		}
	}
	return false
}

func validateEnterpriseVirtualKey(rawToken, scenario string) (bool, string, []string) {
	if strings.TrimSpace(rawToken) == "" || !strings.HasPrefix(rawToken, "sk-tbe-") {
		return false, "", nil
	}
	dbPath := resolveEnterpriseDBPath()
	if dbPath == "" {
		return false, "", nil
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return false, "", nil
	}
	defer db.Close()

	var (
		userID       string
		allowedJSON  string
		isActive     int
		expiresAtRaw sql.NullString
	)
	row := db.QueryRow(
		`SELECT user_id, allowed_models, is_active, expires_at
		 FROM virtual_keys
		 WHERE key_hash = ?
		 LIMIT 1`,
		hashVirtualKey(rawToken),
	)
	if err := row.Scan(&userID, &allowedJSON, &isActive, &expiresAtRaw); err != nil {
		return false, "", nil
	}
	if isActive == 0 {
		return false, "", nil
	}
	if expiresAtRaw.Valid && strings.TrimSpace(expiresAtRaw.String) != "" {
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05-07:00",
			"2006-01-02 15:04:05",
		}
		var parsed time.Time
		var parseErr error
		for _, layout := range layouts {
			parsed, parseErr = time.Parse(layout, expiresAtRaw.String)
			if parseErr == nil {
				break
			}
		}
		if parseErr == nil && parsed.Before(time.Now()) {
			return false, "", nil
		}
	}

	allowed := parseAllowedModels(allowedJSON)
	if !scenarioAllowed(allowed, scenario) {
		return false, "", nil
	}
	return true, userID, allowed
}

// UserAuthMiddleware middleware for UI and control API authentication
func (am *AuthMiddleware) UserAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Authorization header required",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>" format
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Invalid authorization header format. Expected: 'Bearer <token>'",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		token := tokenParts[1]

		// Check against global config user token first
		cfg := am.config
		if cfg != nil && cfg.HasUserToken() {
			configToken := cfg.GetUserToken()

			// Remove "Bearer " prefix if present in the token
			if strings.HasPrefix(token, "Bearer ") {
				token = token[7:]
			}

			// Direct token comparison
			if token == configToken || strings.TrimPrefix(token, "Bearer ") == configToken {
				// Token matches the one in global config, allow access
				c.Set("client_id", "user_authenticated")
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid authorization header format. Expected: 'Bearer <token>'",
				Type:    "invalid_request_error",
			},
		})
		c.Abort()
		return
	}
}

// ModelAuthMiddleware middleware for OpenAI and Anthropic API authentication
// The auth will support both `Authorization` and `X-Api-Key`
func (am *AuthMiddleware) ModelAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		xApiKey := c.GetHeader("X-Api-Key")
		if authHeader == "" && xApiKey == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Authorization header required",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		token := authHeader
		// Remove "Bearer " prefix if present in the token
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}
		token = strings.TrimSpace(token)
		xApiKey = strings.TrimSpace(xApiKey)

		// Check against global config model token first
		cfg := am.config
		if cfg == nil || !cfg.HasModelToken() {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "config or config model token missing",
					Type:    "invalid_request_error",
				},
			})
			return
		}

		configToken := cfg.GetModelToken()

		// Direct token comparison
		if token == configToken || xApiKey == configToken {
			// Token matches the one in global config, allow access
			c.Set("client_id", "model_authenticated")
			c.Next()
			return
		}

		// Enterprise virtual key authentication.
		rawVirtualKey := token
		if rawVirtualKey == "" {
			rawVirtualKey = xApiKey
		}
		if ok, userID, allowedModels := validateEnterpriseVirtualKey(rawVirtualKey, inferScenarioFromRequest(c)); ok {
			c.Set("client_id", "enterprise_virtual_key")
			c.Set("enterprise_user_id", userID)
			c.Set("enterprise_allowed_models", allowedModels)
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid authorization header format. Expected: 'Bearer <token>'",
				Type:    "invalid_request_error",
			},
		})
		c.Abort()
		return
	}
}

// VirtualModelAuthMiddleware middleware for virtual model API authentication
// Uses an independent token separate from the main model token
func (am *AuthMiddleware) VirtualModelAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		xApiKey := c.GetHeader("X-Api-Key")
		if authHeader == "" && xApiKey == "" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Authorization header required for virtual model access",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		token := authHeader
		// Remove "Bearer " prefix if present in the token
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		}

		// Check against virtual model token
		cfg := am.config
		if cfg == nil || !cfg.HasVirtualModelToken() {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "virtual model token not configured",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}

		configToken := cfg.GetVirtualModelToken()

		// Direct token comparison
		if token == configToken || xApiKey == configToken {
			// Token matches, allow access
			c.Set("client_id", "virtual_model_authenticated")
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid virtual model authorization",
				Type:    "invalid_request_error",
			},
		})
		c.Abort()
		return
	}
}

// AuthMiddleware validates the authentication token
func (am *AuthMiddleware) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the auth token from global config
		cfg := am.config
		if cfg == nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Global config not available",
			})
			c.Abort()
			return
		}

		expectedToken := cfg.GetUserToken()
		if expectedToken == "" {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "User auth token not configured",
			})
			c.Abort()
			return
		}

		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header required",
			})
			c.Abort()
			return
		}

		// Support both "Bearer token" and just "token" formats
		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)

		if token != expectedToken {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid authentication token",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
