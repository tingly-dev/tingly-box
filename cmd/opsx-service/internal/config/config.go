package config

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/pkg/auth"
	"github.com/tingly-dev/tingly-box/cmd/opsx-service/internal/middleware"
)

// Config holds the configuration for opsx-service
type Config struct {
	Port              int           // HTTP server port
	JWTSecret         string        // JWT secret for token validation
	SessionTimeout    time.Duration // Session timeout duration
	RateLimitMax      int           // Max auth attempts before block
	RateLimitWindow   time.Duration // Time window for rate limiting
	RateLimitBlock    time.Duration // Block duration after exceeding limit
	EnableRemoteCC    bool          // Enable remote-cc feature (default false)
	jwtManager        *auth.JWTManager
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Port - required
	portStr := os.Getenv("OPSX_PORT")
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

	// JWT Secret - required
	jwtSecret := os.Getenv("OPSX_JWT_SECRET")
	if jwtSecret == "" {
		return nil, &ConfigError{
			Field:   "jwt_secret",
			Message: "must be set (environment variable OPSX_JWT_SECRET)",
		}
	}

	// Session timeout - optional, defaults to 30 minutes
	sessionTimeoutStr := os.Getenv("OPSX_SESSION_TIMEOUT")
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

	// Rate limit max attempts - optional, defaults to 5
	rateLimitMaxStr := os.Getenv("OPSX_RATE_LIMIT_MAX")
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
	rateLimitWindowStr := os.Getenv("OPSX_RATE_LIMIT_WINDOW")
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
	rateLimitBlockStr := os.Getenv("OPSX_RATE_LIMIT_BLOCK")
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

	// Enable remote-cc feature - optional, defaults to false
	enableRemoteCCStr := os.Getenv("OPSX_ENABLE_REMOTE_CC")
	enableRemoteCC := enableRemoteCCStr == "true" || enableRemoteCCStr == "1"

	// Create JWT manager
	jwtManager := auth.NewJWTManager(jwtSecret)

	cfg := &Config{
		Port:             port,
		JWTSecret:        jwtSecret,
		SessionTimeout:   sessionTimeout,
		RateLimitMax:     rateLimitMax,
		RateLimitWindow:  rateLimitWindow,
		RateLimitBlock:   rateLimitBlock,
		EnableRemoteCC:   enableRemoteCC,
		jwtManager:       jwtManager,
	}

	logrus.Infof("Configuration loaded: port=%d, session_timeout=%v, rate_limit_max=%d, rate_limit_window=%v, rate_limit_block=%v, enable_remote_cc=%v",
		port, sessionTimeout, rateLimitMax, rateLimitWindow, rateLimitBlock, enableRemoteCC)

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
