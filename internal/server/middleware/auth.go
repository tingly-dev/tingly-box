package middleware

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
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

type enterpriseContextClaims struct {
	UserID       string
	DepartmentID string
	KeyPrefix    string
	Tier         string
	JTI          string
}

func containsFold(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func resolveSecretRef(raw string) (string, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "", nil
	}
	if strings.HasPrefix(ref, "env:") {
		return strings.TrimSpace(os.Getenv(strings.TrimSpace(strings.TrimPrefix(ref, "env:")))), nil
	}
	if strings.HasPrefix(ref, "file:") {
		path := strings.TrimSpace(strings.TrimPrefix(ref, "file:"))
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(content)), nil
	}
	if st, err := os.Stat(ref); err == nil && !st.IsDir() {
		content, readErr := os.ReadFile(ref)
		if readErr != nil {
			return "", readErr
		}
		return strings.TrimSpace(string(content)), nil
	}
	return ref, nil
}

func parseRSAPublicKeyFromPEM(raw string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("invalid pem block")
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("public key is not RSA")
		}
		return rsaKey, nil
	}
	cert, certErr := x509.ParseCertificate(block.Bytes)
	if certErr == nil {
		if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	return nil, errors.New("failed to parse rsa public key")
}

func verifyEnterpriseContextJWT(cfg *config.Config, rawToken string) (*enterpriseContextClaims, error) {
	tokenString := strings.TrimSpace(rawToken)
	if tokenString == "" || cfg == nil {
		return nil, nil
	}
	jwtCfg := cfg.EnterpriseContextJWT
	if !jwtCfg.Enabled {
		return nil, nil
	}

	clockSkew := time.Duration(jwtCfg.ClockSkewSeconds) * time.Second
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		alg := strings.ToUpper(strings.TrimSpace(token.Method.Alg()))
		if len(jwtCfg.AlgAllowlist) > 0 && !containsFold(jwtCfg.AlgAllowlist, alg) {
			return nil, fmt.Errorf("algorithm %s is not allowed", alg)
		}
		switch alg {
		case "HS256":
			secret, secretErr := resolveSecretRef(jwtCfg.HS256SecretRef)
			if secretErr != nil {
				return nil, secretErr
			}
			if strings.TrimSpace(secret) == "" {
				return nil, errors.New("hs256 secret is empty")
			}
			return []byte(secret), nil
		case "RS256":
			// If no public keys were provided, fall back to RS256PublicKeyRef.
			if len(jwtCfg.PublicKeys) == 0 && strings.TrimSpace(jwtCfg.RS256PublicKeyRef) != "" {
				ref, refErr := resolveSecretRef(jwtCfg.RS256PublicKeyRef)
				if refErr == nil && strings.TrimSpace(ref) != "" {
					if pub, parseErr := parseRSAPublicKeyFromPEM(ref); parseErr == nil {
						return pub, nil
					}
				}
			}
			kid, _ := token.Header["kid"].(string)
			kid = strings.TrimSpace(kid)
			for _, item := range jwtCfg.PublicKeys {
				if kid != "" && strings.TrimSpace(item.KID) != kid {
					continue
				}
				pub, parseErr := parseRSAPublicKeyFromPEM(item.PEM)
				if parseErr != nil {
					continue
				}
				return pub, nil
			}
			if strings.TrimSpace(jwtCfg.JWKSURL) != "" {
				return nil, errors.New("jwks_url is configured but dynamic jwks fetch is not implemented yet")
			}
			return nil, errors.New("no rs256 public key matched")
		default:
			return nil, fmt.Errorf("unsupported jwt alg: %s", alg)
		}
	}, jwt.WithLeeway(clockSkew))
	if err != nil || !parsed.Valid {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("invalid context jwt")
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid jwt claims")
	}

	if len(jwtCfg.AllowedIssuers) > 0 {
		issuer, _ := mapClaims.GetIssuer()
		if !containsFold(jwtCfg.AllowedIssuers, issuer) {
			return nil, fmt.Errorf("issuer %s is not allowed", issuer)
		}
	}

	if len(jwtCfg.AllowedAudiences) > 0 {
		aud, audErr := mapClaims.GetAudience()
		if audErr != nil {
			return nil, audErr
		}
		found := false
		for _, candidate := range aud {
			if containsFold(jwtCfg.AllowedAudiences, candidate) {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("audience is not allowed")
		}
	}

	userID, _ := mapClaims["sub"].(string)
	departmentID, _ := mapClaims["dept_id"].(string)
	keyPrefix, _ := mapClaims["key_prefix"].(string)
	tier, _ := mapClaims["tier"].(string)
	jti, _ := mapClaims["jti"].(string)

	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("missing sub claim")
	}
	if jwtCfg.RequireJTI && strings.TrimSpace(jti) == "" {
		return nil, errors.New("missing jti claim")
	}

	return &enterpriseContextClaims{
		UserID:       strings.TrimSpace(userID),
		DepartmentID: strings.TrimSpace(departmentID),
		KeyPrefix:    strings.TrimSpace(keyPrefix),
		Tier:         strings.TrimSpace(tier),
		JTI:          strings.TrimSpace(jti),
	}, nil
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
			c.Set("client_id", "model_authenticated")
			contextJWT := strings.TrimSpace(c.GetHeader("X-TBE-Context-JWT"))
			if contextJWT != "" {
				claims, verifyErr := verifyEnterpriseContextJWT(cfg, contextJWT)
				if verifyErr != nil {
					c.JSON(http.StatusUnauthorized, ErrorResponse{
						Error: ErrorDetail{
							Message: "Invalid enterprise context jwt",
							Type:    "invalid_request_error",
						},
					})
					c.Abort()
					return
				}
				if claims != nil {
					c.Set("enterprise_user_id", claims.UserID)
					c.Set("enterprise_department_id", claims.DepartmentID)
					c.Set("enterprise_key_prefix", claims.KeyPrefix)
					c.Set("enterprise_user_tier", claims.Tier)
					c.Set("enterprise_context_jti", claims.JTI)
					c.Set("enterprise_context_verified", true)
				}
			}
			c.Next()
			return
		}

		// Enterprise short-lived access token authentication.
		requestToken := token
		if requestToken == "" {
			requestToken = xApiKey
		}
		if strings.HasPrefix(strings.TrimSpace(requestToken), "sk-tbe-") {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error: ErrorDetail{
					Message: "Virtual key must be used through TBE /tbe/* endpoints",
					Type:    "invalid_request_error",
				},
			})
			c.Abort()
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
