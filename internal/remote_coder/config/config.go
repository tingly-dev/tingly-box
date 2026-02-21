package config

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/middleware"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// Config holds the configuration for remote-coder service
// Remote-coder specific configuration only - AgentBoot config is loaded directly by agentboot
type Config struct {
	Port             int           // HTTP server port
	JWTSecret        string        // JWT secret for token validation
	UserToken        string        // User token from main service (legacy auth)
	DBPath           string        // SQLite database path for remote-coder
	SessionTimeout   time.Duration // Session timeout duration
	MessageRetention time.Duration // How long to retain messages
	RateLimitMax     int           // Max auth attempts before block
	RateLimitWindow  time.Duration // Time window for rate limiting
	RateLimitBlock   time.Duration // Block duration after exceeding limit
	jwtManager       *auth.JWTManager
}

// Options allows overrides when building remote-coder configuration.
type Options struct {
	Port                 *int
	DBPath               *string
	SessionTimeout       *time.Duration
	MessageRetentionDays *int
	RateLimitMax         *int
	RateLimitWindow      *time.Duration
	RateLimitBlock       *time.Duration
	JWTSecret            *string
}

// LoadFromAppConfig builds remote-coder config from the main app config with env/override support.
func LoadFromAppConfig(appCfg *serverconfig.Config, opts Options) (*Config, error) {
	if appCfg == nil {
		return nil, &ConfigError{
			Field:   "app_config",
			Message: "must be provided",
		}
	}

	remoteCfg := appCfg.RemoteCoder

	port := remoteCfg.Port
	if port == 0 {
		port = 18080
	}
	if env := os.Getenv("RCC_PORT"); env != "" {
		if parsed, err := strconv.Atoi(env); err == nil {
			port = parsed
		} else {
			return nil, &ConfigError{
				Field:   "port",
				Message: "must be a valid port number (1-65535)",
			}
		}
	}
	if opts.Port != nil {
		port = *opts.Port
	}
	if port <= 0 || port > 65535 {
		return nil, &ConfigError{
			Field:   "port",
			Message: "must be a valid port number (1-65535)",
		}
	}

	jwtSecret := appCfg.JWTSecret
	if env := os.Getenv("RCC_JWT_SECRET"); env != "" {
		jwtSecret = env
	}
	if opts.JWTSecret != nil {
		jwtSecret = *opts.JWTSecret
	}
	if jwtSecret == "" {
		return nil, &ConfigError{
			Field:   "jwt_secret",
			Message: "must be set (main config or RCC_JWT_SECRET)",
		}
	}

	dbPath := remoteCfg.DBPath
	if dbPath == "" {
		if appCfg.ConfigDir != "" {
			dbPath = constant.GetDBFile(appCfg.ConfigDir)
		}
	}
	if env := os.Getenv("RCC_DB_PATH"); env != "" {
		dbPath = env
	}
	if opts.DBPath != nil {
		dbPath = *opts.DBPath
	}
	if dbPath == "" {
		return nil, &ConfigError{
			Field:   "db_path",
			Message: "must be set",
		}
	}

	sessionTimeout := 30 * time.Minute
	if remoteCfg.SessionTimeout != "" {
		parsed, err := time.ParseDuration(remoteCfg.SessionTimeout)
		if err != nil {
			return nil, &ConfigError{
				Field:   "session_timeout",
				Message: "must be a valid duration (e.g., 30m, 1h)",
			}
		}
		sessionTimeout = parsed
	}
	if env := os.Getenv("RCC_SESSION_TIMEOUT"); env != "" {
		parsed, err := time.ParseDuration(env)
		if err != nil {
			return nil, &ConfigError{
				Field:   "session_timeout",
				Message: "must be a valid duration (e.g., 30m, 1h)",
			}
		}
		sessionTimeout = parsed
	}
	if opts.SessionTimeout != nil {
		sessionTimeout = *opts.SessionTimeout
	}

	retentionDays := remoteCfg.MessageRetentionDays
	if retentionDays == 0 {
		retentionDays = 7
	}
	if env := os.Getenv("RCC_MESSAGE_RETENTION_DAYS"); env != "" {
		if parsed, err := strconv.Atoi(env); err == nil {
			retentionDays = parsed
		} else {
			return nil, &ConfigError{
				Field:   "message_retention_days",
				Message: "must be a positive integer",
			}
		}
	}
	if opts.MessageRetentionDays != nil {
		retentionDays = *opts.MessageRetentionDays
	}
	if retentionDays <= 0 {
		return nil, &ConfigError{
			Field:   "message_retention_days",
			Message: "must be a positive integer",
		}
	}
	retention := time.Duration(retentionDays) * 24 * time.Hour

	rateLimitMax := remoteCfg.RateLimitMax
	if rateLimitMax == 0 {
		rateLimitMax = 5
	}
	if env := os.Getenv("RCC_RATE_LIMIT_MAX"); env != "" {
		if parsed, err := strconv.Atoi(env); err == nil {
			rateLimitMax = parsed
		} else {
			return nil, &ConfigError{
				Field:   "rate_limit_max",
				Message: "must be a positive integer",
			}
		}
	}
	if opts.RateLimitMax != nil {
		rateLimitMax = *opts.RateLimitMax
	}
	if rateLimitMax <= 0 {
		return nil, &ConfigError{
			Field:   "rate_limit_max",
			Message: "must be a positive integer",
		}
	}

	rateLimitWindow := 5 * time.Minute
	if remoteCfg.RateLimitWindow != "" {
		parsed, err := time.ParseDuration(remoteCfg.RateLimitWindow)
		if err != nil {
			return nil, &ConfigError{
				Field:   "rate_limit_window",
				Message: "must be a valid duration (e.g., 5m, 10m)",
			}
		}
		rateLimitWindow = parsed
	}
	if env := os.Getenv("RCC_RATE_LIMIT_WINDOW"); env != "" {
		parsed, err := time.ParseDuration(env)
		if err != nil {
			return nil, &ConfigError{
				Field:   "rate_limit_window",
				Message: "must be a valid duration (e.g., 5m, 10m)",
			}
		}
		rateLimitWindow = parsed
	}
	if opts.RateLimitWindow != nil {
		rateLimitWindow = *opts.RateLimitWindow
	}

	rateLimitBlock := 5 * time.Minute
	if remoteCfg.RateLimitBlock != "" {
		parsed, err := time.ParseDuration(remoteCfg.RateLimitBlock)
		if err != nil {
			return nil, &ConfigError{
				Field:   "rate_limit_block",
				Message: "must be a valid duration (e.g., 5m, 10m)",
			}
		}
		rateLimitBlock = parsed
	}
	if env := os.Getenv("RCC_RATE_LIMIT_BLOCK"); env != "" {
		parsed, err := time.ParseDuration(env)
		if err != nil {
			return nil, &ConfigError{
				Field:   "rate_limit_block",
				Message: "must be a valid duration (e.g., 5m, 10m)",
			}
		}
		rateLimitBlock = parsed
	}
	if opts.RateLimitBlock != nil {
		rateLimitBlock = *opts.RateLimitBlock
	}

	jwtManager := auth.NewJWTManager(jwtSecret)

	cfg := &Config{
		Port:             port,
		JWTSecret:        jwtSecret,
		UserToken:        appCfg.UserToken,
		DBPath:           dbPath,
		SessionTimeout:   sessionTimeout,
		MessageRetention: retention,
		RateLimitMax:     rateLimitMax,
		RateLimitWindow:  rateLimitWindow,
		RateLimitBlock:   rateLimitBlock,
		jwtManager:       jwtManager,
	}

	logrus.Infof("Remote-coder config: port=%d, session_timeout=%v, db_path=%s, message_retention=%v, rate_limit_max=%d, rate_limit_window=%v, rate_limit_block=%v",
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
