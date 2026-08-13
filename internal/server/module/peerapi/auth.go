package peerapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/remote/peer"
)

// CtxKeyAuthKind marks how a data-plane request authenticated: "operator"
// (UserToken) or "peer" (tb-peer- token).
const CtxKeyAuthKind = "peer_auth_kind"

// DataAuthMiddleware guards the peer data plane. Two credentials are
// accepted (spec §4):
//
//   - the operator UserToken (isOperatorToken), so a human can test with the
//     credential they already have;
//   - the peer's own tb-peer- token — valid ONLY when it belongs to the {id}
//     in the path and the peer is enabled.
//
// Wrong token, foreign token, and disabled peer all return the same 401 body
// so the data plane doesn't leak which peers exist.
func DataAuthMiddleware(store peer.Store, isOperatorToken func(string) bool) gin.HandlerFunc {
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
		if strings.HasPrefix(token, peer.TokenPrefix) && store != nil {
			p, err := store.GetByToken(peer.HashToken(token))
			if err == nil && p.Enabled && p.UUID == c.Param("id") &&
				peer.VerifyToken(token, p.TokenHash) {
				c.Set(CtxKeyAuthKind, "peer")
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
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid peer authorization token"})
}
