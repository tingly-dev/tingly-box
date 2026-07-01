package server

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// profileAliasMiddleware rewrites a profile alias in the ":scenario" path
// segment to its canonical "base:pN" form before contextMiddleware runs.
//
// Profile endpoints are addressed as "/tingly/claude_code:p1", but the "p1"
// suffix has low recognizability. This middleware lets callers use the
// profile's name instead — "/tingly/claude_code:mine" — by resolving the
// suffix against the configured profiles and rewriting the path param to the
// profile ID in place. Everything downstream (contextMiddleware, auth,
// routing, usage records) only ever sees the canonical "base:pN", so no other
// stage needs to learn about aliases.
//
// Resolution is best-effort and non-fatal: if the suffix is already a valid
// ID, or cannot be resolved to a simple/URL-friendly profile name, the path is
// left untouched and contextMiddleware performs validation (and error
// reporting) exactly as before.
func (s *Server) profileAliasMiddleware(c *gin.Context) {
	rawScenario := c.Param("scenario")
	base, suffix := typ.ParseScenarioProfile(typ.RuleScenario(rawScenario))
	// Only profiled scenarios ("base:suffix") are eligible — a missing suffix
	// is a plain scenario with nothing to resolve.
	if base == "" || suffix == "" || s.config == nil {
		c.Next()
		return
	}

	id, ok := s.config.ResolveProfileAlias(base, suffix)
	if !ok || id == suffix {
		// Unknown alias, non-simple name, or already canonical — leave as-is.
		c.Next()
		return
	}

	rewritten := string(typ.ProfiledScenarioName(base, id))

	// Rewrite the routed path param — covers every handler and the
	// contextMiddleware that derives the request-context scenario from c.Param.
	for i := range c.Params {
		if c.Params[i].Key == "scenario" {
			c.Params[i].Value = rewritten
		}
	}

	// Also rewrite the URL path so consumers that re-derive the scenario from
	// the raw path agree on the canonical form. The usage tracker
	// (extractScenarioFromPath) is the one that matters: without this, requests
	// via the alias would be recorded under "claude_code:mine" instead of
	// "claude_code:p1", splitting analytics across the alias and the ID.
	originalPath := c.Request.URL.Path
	oldSeg := "/tingly/" + rawScenario
	newSeg := "/tingly/" + rewritten
	rewriteSeg := func(p string) string {
		if rest, found := strings.CutPrefix(p, oldSeg); found {
			return newSeg + rest
		}
		return p
	}
	c.Request.URL.Path = rewriteSeg(c.Request.URL.Path)
	if c.Request.URL.RawPath != "" {
		c.Request.URL.RawPath = rewriteSeg(c.Request.URL.RawPath)
	}

	// Record the mapping. After this point the original alias is gone from the
	// path, usage records, and access logs — all of which now show the
	// canonical ID. The before→after fields keep SRE able to correlate a client
	// that called "/tingly/claude_code:mine/..." with records tagged
	// "claude_code:p1".
	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"profile_alias":  rawScenario,
		"scenario":       rewritten,
		"original_path":  originalPath,
		"rewritten_path": c.Request.URL.Path,
	}).Infof("[profile-alias] resolved %q -> %q", rawScenario, rewritten)

	c.Next()
}

// getUserAuthMiddleware returns the user auth middleware to use
func (s *Server) getUserAuthMiddleware() gin.HandlerFunc {
	if s.customUserAuthMiddleware != nil {
		return s.customUserAuthMiddleware
	}
	return s.authMW.UserAuthMiddleware()
}

// getModelAuthMiddleware returns the model auth middleware to use
func (s *Server) getModelAuthMiddleware() gin.HandlerFunc {
	if s.customModelAuthMiddleware != nil {
		return s.customModelAuthMiddleware
	}
	return s.authMW.ModelAuthMiddleware()
}
