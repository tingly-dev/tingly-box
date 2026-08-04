package subscriptionapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/remote/subscription"
)

// CtxKeyAuthKind marks how a data-plane request authenticated: "operator"
// (UserToken) or "subscription" (tb-sub- token).
const CtxKeyAuthKind = "subscription_auth_kind"

// DataAuthMiddleware guards the subscription data plane. Two credentials are
// accepted (spec §4):
//
//   - the operator UserToken (isOperatorToken), so a human can test with the
//     credential they already have;
//   - the subscription's own tb-sub- token — valid ONLY when it belongs to
//     the {id} in the path and the subscription is enabled.
//
// Wrong token, foreign token, and disabled subscription all return the same
// 401 body so the data plane doesn't leak which subscriptions exist.
func DataAuthMiddleware(store subscription.Store, isOperatorToken func(string) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			unauthorized(c)
			return
		}
		if isOperatorToken != nil && isOperatorToken(token) {
			c.Set(CtxKeyAuthKind, "operator")
			c.Next()
			return
		}
		if strings.HasPrefix(token, subscription.TokenPrefix) && store != nil {
			sub, err := store.GetByToken(subscription.HashToken(token))
			if err == nil && sub.Enabled && sub.UUID == c.Param("id") &&
				subscription.VerifyToken(token, sub.TokenHash) {
				c.Set(CtxKeyAuthKind, "subscription")
				c.Next()
				return
			}
		}
		unauthorized(c)
	}
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid subscription authorization token"})
}
