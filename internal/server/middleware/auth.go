package middleware

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

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

type cachedJWKS struct {
	expiresAt time.Time
	keys      map[string]*rsa.PublicKey
}

var runtimeJWKSCache struct {
	mu   sync.RWMutex
	data map[string]cachedJWKS // key: request host
}

const runtimeJWKSTTL = 5 * time.Minute

func decodeStringSliceClaim(claims jwt.MapClaims, key string) []string {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func parseJWKSKey(item struct {
	KTY string `json:"kty"`
	ALG string `json:"alg"`
	KID string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}) (*rsa.PublicKey, string, bool) {
	if strings.ToUpper(strings.TrimSpace(item.KTY)) != "RSA" {
		return nil, "", false
	}
	kid := strings.TrimSpace(item.KID)
	if kid == "" {
		return nil, "", false
	}
	nb, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(item.N))
	if err != nil || len(nb) == 0 {
		return nil, "", false
	}
	eb, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(item.E))
	if err != nil || len(eb) == 0 {
		return nil, "", false
	}
	eInt := new(big.Int).SetBytes(eb).Int64()
	if eInt <= 0 {
		return nil, "", false
	}
	pub := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: int(eInt),
	}
	return pub, kid, true
}

func fetchRuntimeJWKS(host string) map[string]*rsa.PublicKey {
	client := &http.Client{Timeout: 2 * time.Second}
	endpoint := fmt.Sprintf("http://%s/api/enterprise/runtime/jwks.json", host)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	var jwks struct {
		Keys []struct {
			KTY string `json:"kty"`
			ALG string `json:"alg"`
			KID string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil
	}
	out := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, item := range jwks.Keys {
		pub, kid, ok := parseJWKSKey(item)
		if !ok {
			continue
		}
		out[kid] = pub
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func getRuntimeJWKS(host string) map[string]*rsa.PublicKey {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1:12580"
	}

	now := time.Now()
	runtimeJWKSCache.mu.RLock()
	if runtimeJWKSCache.data != nil {
		if c, ok := runtimeJWKSCache.data[host]; ok && now.Before(c.expiresAt) && len(c.keys) > 0 {
			keys := c.keys
			runtimeJWKSCache.mu.RUnlock()
			return keys
		}
	}
	runtimeJWKSCache.mu.RUnlock()

	keys := fetchRuntimeJWKS(host)
	if len(keys) == 0 {
		return nil
	}

	runtimeJWKSCache.mu.Lock()
	if runtimeJWKSCache.data == nil {
		runtimeJWKSCache.data = make(map[string]cachedJWKS)
	}
	runtimeJWKSCache.data[host] = cachedJWKS{
		expiresAt: now.Add(runtimeJWKSTTL),
		keys:      keys,
	}
	runtimeJWKSCache.mu.Unlock()
	return keys
}

func validateEnterpriseAccessToken(rawToken, scenario, requestHost string) (bool, string, []string, string) {
	tokenString := strings.TrimSpace(rawToken)
	if tokenString == "" || strings.HasPrefix(tokenString, "sk-tbe-") {
		return false, "", nil, ""
	}
	keys := getRuntimeJWKS(requestHost)
	if len(keys) == 0 {
		return false, "", nil, ""
	}

	parsedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrTokenUnverifiable
		}
		kid, _ := token.Header["kid"].(string)
		kid = strings.TrimSpace(kid)
		if kid == "" {
			return nil, jwt.ErrTokenUnverifiable
		}
		pub, ok := keys[kid]
		if !ok {
			// one forced refresh for rotated keys.
			keys = getRuntimeJWKS(requestHost)
			pub, ok = keys[kid]
			if !ok {
				return nil, jwt.ErrTokenUnverifiable
			}
		}
		return pub, nil
	})
	if err != nil || !parsedToken.Valid {
		return false, "", nil, ""
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return false, "", nil, ""
	}
	if typ, _ := claims["typ"].(string); strings.TrimSpace(typ) != "tbe_tb_access" {
		return false, "", nil, ""
	}
	userID, _ := claims["uid"].(string)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, "", nil, ""
	}
	allowedModels := decodeStringSliceClaim(claims, "am")
	if len(allowedModels) == 0 {
		return false, "", nil, ""
	}
	if !scenarioAllowed(allowedModels, scenario) {
		return false, "", nil, ""
	}
	keyPrefix, _ := claims["kp"].(string)
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		keyPrefix = "tbe-exchange"
	}
	return true, userID, allowedModels, keyPrefix
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

		// Enterprise short-lived access token authentication.
		enterpriseToken := token
		if enterpriseToken == "" {
			enterpriseToken = xApiKey
		}
		if strings.HasPrefix(strings.TrimSpace(enterpriseToken), "sk-tbe-") {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Virtual key must be exchanged at /api/enterprise/runtime/token/exchange before calling TB APIs",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
			return
		}
		if ok, userID, allowedModels, keyPrefix := validateEnterpriseAccessToken(enterpriseToken, inferScenarioFromRequest(c), c.Request.Host); ok {
			c.Set("client_id", "enterprise_access_token")
			c.Set("enterprise_user_id", userID)
			c.Set("enterprise_allowed_models", allowedModels)
			c.Set("enterprise_key_prefix", keyPrefix)
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
