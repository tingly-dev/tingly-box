package config

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/remote_coder/middleware"
	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// readJWTSecretFromMainConfig reads the JWT secret from the main service's config database or JSON config
func readJWTSecretFromMainConfig() string {
	// Try to find the main service's config database
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	configPaths := []string{
		filepath.Join(homeDir, ".tingly-box", "db", "tingly.db"),
		filepath.Join(homeDir, ".tingly-box", "tingly.db"),
		"./data/tingly.db",
	}

	for _, configPath := range configPaths {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			continue
		}

		// Open database connection
		db, err := gorm.Open(sqlite.Open(configPath), &gorm.Config{})
		if err != nil {
			logrus.Debugf("Failed to open main service config db at %s: %v", configPath, err)
			continue
		}

		// Get underlying SQL DB for proper cleanup
		sqlDB, err := db.DB()
		if err != nil {
			logrus.Debugf("Failed to get SQL DB from GORM: %v", err)
			continue
		}

		// Query for JWT secret
		type ConfigRow struct {
			Key   string
			Value string
		}
		var result ConfigRow
		if err := db.Table("config").Where("key = ?", "jwt_secret").Select("value").First(&result).Error; err == nil && result.Value != "" {
			sqlDB.Close()
			logrus.Infof("Loaded JWT secret from main service config")
			return result.Value
		}

		sqlDB.Close()
	}

	logrus.Warn("Could not find JWT secret in main service config, using default or environment variable")
	// Fallback to JSON config
	if secret := readConfigJSONValue("jwt_secret"); secret != "" {
		logrus.Infof("Loaded JWT secret from main service config.json")
		return secret
	}
	return ""
}

// readUserTokenFromMainConfig reads the user token from the main service's JSON config
func readUserTokenFromMainConfig() string {
	if token := readConfigJSONValue("user_token"); token != "" {
		logrus.Infof("Loaded user token from main service config.json")
		return token
	}
	return ""
}

func readConfigJSONValue(key string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	configPath := filepath.Join(homeDir, ".tingly-box", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	if val, ok := cfg[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// Config holds the configuration for remote-cc service
type Config struct {
	Port             int           // HTTP server port
	JWTSecret        string        // JWT secret for token validation
	UserToken        string        // User token from main service (legacy auth)
	DBPath           string        // SQLite database path for remote-cc
	SessionTimeout   time.Duration // Session timeout duration
	MessageRetention time.Duration // How long to retain messages
	RateLimitMax     int           // Max auth attempts before block
	RateLimitWindow  time.Duration // Time window for rate limiting
	RateLimitBlock   time.Duration // Block duration after exceeding limit
	jwtManager       *auth.JWTManager
}

// Load reads configuration from environment variables and database
func Load() (*Config, error) {
	// Port - required
	portStr := os.Getenv("RCC_PORT")
	if portStr == "" {
		portStr = "18080" // default port
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, &ConfigError{
			Field:   "port",
			Message: "must be a valid port number (1-65535)",
		}
	}

	// JWT Secret - try RCC_JWT_SECRET first, then read from main service's config
	jwtSecret := os.Getenv("RCC_JWT_SECRET")
	if jwtSecret == "" {
		// Try to read from main service's config database
		jwtSecret = readJWTSecretFromMainConfig()
		if jwtSecret == "" {
			return nil, &ConfigError{
				Field:   "jwt_secret",
				Message: "must be set (environment variable RCC_JWT_SECRET or main service config)",
			}
		}
	}

	// User token - optional, for legacy auth fallback
	userToken := readUserTokenFromMainConfig()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, &ConfigError{
			Field:   "home_dir",
			Message: "could not resolve user home directory",
		}
	}

	// Session timeout - optional, defaults to 30 minutes
	sessionTimeoutStr := os.Getenv("RCC_SESSION_TIMEOUT")
	var sessionTimeout time.Duration
	if sessionTimeoutStr == "" {
		sessionTimeout = 30 * time.Minute
	} else {
		timeout, err := time.ParseDuration(sessionTimeoutStr)
		if err != nil {
			return nil, &ConfigError{
				Field:   "session_timeout",
				Message: "must be a valid duration (e.g., 30m, 1h)",
			}
		}
		sessionTimeout = timeout
	}

	// DB Path - optional, defaults to ~/.tingly-box/remote-cc.db
	defaultDBPath := filepath.Join(homeDir, ".tingly-box", "remote-cc.db")
	dbPath := os.Getenv("RCC_DB_PATH")
	if dbPath == "" {
		dbPath = defaultDBPath
	}

	// Message retention - optional, defaults to 7 days
	retentionDaysStr := os.Getenv("RCC_MESSAGE_RETENTION_DAYS")
	retention := 7 * 24 * time.Hour
	if retentionDaysStr != "" {
		days, err := strconv.Atoi(retentionDaysStr)
		if err != nil || days <= 0 {
			return nil, &ConfigError{
				Field:   "message_retention_days",
				Message: "must be a positive integer",
			}
		}
		retention = time.Duration(days) * 24 * time.Hour
	}

	// Rate limit max attempts - optional, defaults to 5
	rateLimitMaxStr := os.Getenv("RCC_RATE_LIMIT_MAX")
	var rateLimitMax int
	if rateLimitMaxStr == "" {
		rateLimitMax = 5
	} else {
		max, err := strconv.Atoi(rateLimitMaxStr)
		if err != nil || max <= 0 {
			return nil, &ConfigError{
				Field:   "rate_limit_max",
				Message: "must be a positive integer",
			}
		}
		rateLimitMax = max
	}

	// Rate limit window - optional, defaults to 5 minutes
	rateLimitWindowStr := os.Getenv("RCC_RATE_LIMIT_WINDOW")
	var rateLimitWindow time.Duration
	if rateLimitWindowStr == "" {
		rateLimitWindow = 5 * time.Minute
	} else {
		window, err := time.ParseDuration(rateLimitWindowStr)
		if err != nil {
			return nil, &ConfigError{
				Field:   "rate_limit_window",
				Message: "must be a valid duration (e.g., 5m, 10m)",
			}
		}
		rateLimitWindow = window
	}

	// Rate limit block duration - optional, defaults to 5 minutes
	rateLimitBlockStr := os.Getenv("RCC_RATE_LIMIT_BLOCK")
	var rateLimitBlock time.Duration
	if rateLimitBlockStr == "" {
		rateLimitBlock = 5 * time.Minute
	} else {
		block, err := time.ParseDuration(rateLimitBlockStr)
		if err != nil {
			return nil, &ConfigError{
				Field:   "rate_limit_block",
				Message: "must be a valid duration (e.g., 5m, 10m)",
			}
		}
		rateLimitBlock = block
	}

	// Create JWT manager
	jwtManager := auth.NewJWTManager(jwtSecret)

	cfg := &Config{
		Port:             port,
		JWTSecret:        jwtSecret,
		UserToken:        userToken,
		DBPath:           dbPath,
		SessionTimeout:   sessionTimeout,
		MessageRetention: retention,
		RateLimitMax:     rateLimitMax,
		RateLimitWindow:  rateLimitWindow,
		RateLimitBlock:   rateLimitBlock,
		jwtManager:       jwtManager,
	}

	logrus.Infof("Configuration loaded: port=%d, session_timeout=%v, db_path=%s, message_retention=%v, rate_limit_max=%d, rate_limit_window=%v, rate_limit_block=%v",
		port, sessionTimeout, dbPath, retention, rateLimitMax, rateLimitWindow, rateLimitBlock)

	return cfg, nil
}

// ConfigError represents a configuration error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "invalid configuration for '" + e.Field + "': " + e.Message
}

// AuthMiddleware creates a Gin middleware for JWT authentication
func AuthMiddleware(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Authorization header required",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		// Validate token using JWT manager
		claims, err := cfg.jwtManager.ValidateAPIKey(authHeader)
		if err != nil {
			// Fallback: accept legacy user token from main config.json
			normalized := authHeader
			if strings.HasPrefix(normalized, "Bearer ") {
				normalized = normalized[7:]
			}
			if cfg.UserToken != "" && normalized == cfg.UserToken {
				c.Set("client_id", "user")
				c.Set("claims", &auth.Claims{ClientID: "user"})
				c.Next()
				return
			}

			logrus.Warnf("Token validation failed: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Invalid authorization token: " + err.Error(),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		// Store claims in context
		c.Set("client_id", claims.ClientID)
		c.Set("claims", claims)

		logrus.Debugf("Authenticated request from client: %s", claims.ClientID)

		c.Next()
	}
}

// NewRateLimiter creates a rate limiter from config
func (cfg *Config) NewRateLimiter() *middleware.RateLimiter {
	return middleware.NewRateLimiter(
		cfg.RateLimitMax,
		cfg.RateLimitWindow,
		cfg.RateLimitBlock,
	)
}

// GenerateToken generates a new API token for a client
func (cfg *Config) GenerateToken(clientID string, expiryHours int) (string, error) {
	var expiry time.Duration
	if expiryHours <= 0 {
		expiry = 24 * time.Hour // Default 24 hours
	} else {
		expiry = time.Duration(expiryHours) * time.Hour
	}

	token, err := cfg.jwtManager.GenerateTokenWithExpiry(clientID, expiry)
	if err != nil {
		return "", err
	}

	// Wrap in API key format
	return cfg.jwtManager.GenerateAPIKey(token)
}

// ValidateToken validates an API token and returns the claims
func (cfg *Config) ValidateToken(tokenString string) (*auth.Claims, error) {
	return cfg.jwtManager.ValidateAPIKey(tokenString)
}
