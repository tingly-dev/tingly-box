package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/ratelimit"
)

// SharingKeyRateLimitMiddleware enforces a sharing key's requests-per-minute
// budget (TEP-1851). It must run after ModelAuthMiddleware, which puts the
// key id and its rpm_limit on the context for sharing-key callers.
//
// Scope, by design:
//   - Only requests authenticated with a sharing key are counted; the global
//     model token and enterprise callers pass through untouched.
//   - Only POST requests count. GET /models and GET /quota are discovery
//     calls a client may poll, and the budget is meant for model invocations.
//   - POST /messages/count_tokens is exempt as well: it tokenizes the request
//     without running the model, and Claude Code calls it to size its
//     context, so counting it would spend the budget on bookkeeping.
//   - A key whose rpm_limit is 0 is unlimited and never reaches the limiter.
//
// On exceed the request is answered with 429, a Retry-After header (whole
// seconds, rounded up) and the standard error envelope, so SDK clients back
// off the way they do for upstream rate limits.
func SharingKeyRateLimitMiddleware(limiter ratelimit.Limiter) gin.HandlerFunc {
	if limiter == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost ||
			c.GetString(constant.CtxKeyAuthKind) != constant.AuthKindSharingKey ||
			isRateLimitExemptRoute(c.FullPath()) {
			c.Next()
			return
		}
		limit := sharingKeyRPMLimit(c)
		keyID := c.GetString(constant.CtxKeySharingKeyID)
		if limit <= 0 || keyID == "" {
			c.Next()
			return
		}

		d := limiter.Allow(keyID, limit)
		if d.Allowed {
			c.Next()
			return
		}

		retryAfter := int(math.Ceil(d.RetryAfter.Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		abortWithError(c, http.StatusTooManyRequests,
			fmt.Sprintf("Sharing key rate limit exceeded (%d requests/minute). Retry after %d second(s).", limit, retryAfter),
			"rate_limit_error")
	}
}

// rateLimitExemptRoutes lists the registered route templates of POST
// endpoints that do not invoke a model and therefore do not consume the
// budget. Matched exactly against gin's FullPath (the template, not the raw
// URL), so a client cannot dress a model call up as one of these and a
// future route that merely ends in the same suffix is not exempted by
// accident. Both the bare and the /v1 shape of the model surface are listed
// (see SetupMixinEndpoints in internal/protocolserver/routes.go).
var rateLimitExemptRoutes = map[string]bool{
	"/tingly/:scenario/messages/count_tokens":    true,
	"/tingly/:scenario/v1/messages/count_tokens": true,
}

func isRateLimitExemptRoute(fullPath string) bool {
	return rateLimitExemptRoutes[fullPath]
}

// rpmLimitTypeWarning fires once per process: the contract with auth.go is
// "an int", and a drift there would otherwise turn every key unlimited with
// no trace in the logs.
var rpmLimitTypeWarning sync.Once

// sharingKeyRPMLimit reads the budget ModelAuthMiddleware stored on the
// context. Unlike gin's GetInt it does not swallow a type mismatch: a value
// of any other dynamic type is reported once and treated as unlimited
// (fail-open — the failure mode is "less limiting", never "whole team
// blocked").
func sharingKeyRPMLimit(c *gin.Context) int {
	v, ok := c.Get(constant.CtxKeySharingKeyRPMLimit)
	if !ok {
		return 0
	}
	limit, ok := v.(int)
	if !ok {
		rpmLimitTypeWarning.Do(func() {
			logrus.Warnf("sharing key rate limit: context value %q has type %T, want int; treating as unlimited",
				constant.CtxKeySharingKeyRPMLimit, v)
		})
		return 0
	}
	return limit
}
