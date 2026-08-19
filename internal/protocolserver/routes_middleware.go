package protocolserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// rewriteScenarioParam rewrites the ":scenario" path param (and the request's
// URL path/RawPath, so anything that re-derives the scenario from the raw
// path agrees) from rawScenario to rewritten. Shared by profileAliasMiddleware
// and legacyScenarioAliasMiddleware — both need every downstream stage
// (contextMiddleware, auth, routing, usage records) to see only the
// canonical form, never the alias a caller actually used.
func rewriteScenarioParam(c *gin.Context, rawScenario, rewritten string) {
	for i := range c.Params {
		if c.Params[i].Key == "scenario" {
			c.Params[i].Value = rewritten
		}
	}

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
}

// legacyScenarioAliasMiddleware rewrites a deprecated scenario id (e.g.
// "agent", renamed to "custom") to its canonical id before anything else
// runs. Unlike profileAliasMiddleware this is not a UX nicety — it is a
// permanent compatibility guarantee: a client hardcoded against the old id
// (an external integration, a bookmarked base URL) must keep resolving
// exactly like the new one forever, even though stored rules are migrated to
// the new scenario the moment the config loads (see
// migrateAgentScenarioToCustom) and nothing is ever stored under the old id
// again.
//
// No profile-suffix handling here (contrast profileAliasMiddleware): every
// scenario in legacyScenarioAliases predates and never supported profiles
// (SupportsProfiles is false for all of them), so a raw scenario param is
// matched as-is — a "legacy:suffix" shape simply isn't a known alias and
// falls through to fail normally downstream.
func (ph *ProtocolHandler) legacyScenarioAliasMiddleware(c *gin.Context) {
	rawScenario := c.Param("scenario")

	canonical, ok := typ.ResolveScenarioAlias(typ.RuleScenario(rawScenario))
	if !ok {
		c.Next()
		return
	}
	rewritten := string(canonical)

	rewriteScenarioParam(c, rawScenario, rewritten)

	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"legacy_scenario": rawScenario,
		"scenario":        rewritten,
	}).Infof("[legacy-scenario-alias] resolved %q -> %q", rawScenario, rewritten)

	c.Next()
}

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
func (ph *ProtocolHandler) profileAliasMiddleware(c *gin.Context) {
	rawScenario := c.Param("scenario")
	base, suffix := typ.ParseScenarioProfile(typ.RuleScenario(rawScenario))
	// Only profiled scenarios ("base:suffix") are eligible — a missing suffix
	// is a plain scenario with nothing to resolve.
	if base == "" || suffix == "" || ph.deps.Config == nil {
		c.Next()
		return
	}

	id, ok := ph.deps.Config.ResolveProfileAlias(base, suffix)
	if !ok || id == suffix {
		// Unknown alias, non-simple name, or already canonical — leave as-is.
		c.Next()
		return
	}

	rewritten := string(typ.ProfiledScenarioName(base, id))
	rewriteScenarioParam(c, rawScenario, rewritten)

	// Record the mapping. After this point the original alias is gone from the
	// path, usage records, and access logs — all of which now show the
	// canonical ID. The before→after fields keep SRE able to correlate a client
	// that called "/tingly/claude_code:mine/..." with records tagged
	// "claude_code:p1".
	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"profile_alias":  rawScenario,
		"scenario":       rewritten,
		"rewritten_path": c.Request.URL.Path,
	}).Infof("[profile-alias] resolved %q -> %q", rawScenario, rewritten)

	c.Next()
}

// contextMiddleware is a middleware that extracts the scenario parameter from the URL path
// and injects it into the request context for use by downstream components (e.g., RecordRoundTripper).
// It also validates profile suffixes (e.g., "claude_code:p1") if present.
func (ph *ProtocolHandler) contextMiddleware(c *gin.Context) {
	rawScenario := c.Param("scenario")
	ctx := context.WithValue(c.Request.Context(), client.ScenarioContextKey, rawScenario)
	c.Request = c.Request.WithContext(ctx)

	// Validate profile if present (e.g., "claude_code:p1")
	if typ.IsProfiledScenario(typ.RuleScenario(rawScenario)) {
		base, profileID := typ.ParseScenarioProfile(typ.RuleScenario(rawScenario))
		if base == "" || profileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("invalid scenario format: '%s'", rawScenario),
			})
			c.Abort()
			return
		}

		// Check base scenario exists in registry
		if _, ok := typ.GetScenarioDescriptor(typ.RuleScenario(rawScenario)); !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("unknown scenario '%s'", base),
			})
			c.Abort()
			return
		}

		// Check profile exists in config
		if ph.deps.Config != nil {
			if _, ok := ph.deps.Config.GetProfile(typ.RuleScenario(base), profileID); !ok {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   fmt.Sprintf("unknown profile '%s' for scenario '%s'", profileID, base),
				})
				c.Abort()
				return
			}
		}
	}

	c.Next()
}

// teamScopeMiddleware converts the public, stable /tingly/team surface into
// the routing scope authorized by the credential. It runs after model auth,
// so the client cannot choose this value. The original URL remains unchanged:
// usage and tracing keep the low-cardinality public scenario "team", while
// rule lookup consumes the rewritten Gin parameter.
//
// The default team intentionally keeps the legacy bare "team" scope so all
// existing rules remain usable without a destructive migration. Additional
// teams use the isolated internal form "team:<stable-id>".
func (ph *ProtocolHandler) teamScopeMiddleware(c *gin.Context) {
	if c.Param("scenario") != string(typ.ScenarioTeam) {
		c.Next()
		return
	}

	teamID := c.GetString(constant.CtxKeyTeamID)
	if teamID == "" {
		// Global model auth has no explicit team claim. Preserve the historic
		// /tingly/team behavior by treating it as the default team.
		teamID = db.DefaultTeamID
		c.Set(constant.CtxKeyTeamID, teamID)
	}
	if teamID != db.DefaultTeamID {
		for i := range c.Params {
			if c.Params[i].Key == "scenario" {
				c.Params[i].Value = string(typ.ProfiledScenarioName(typ.ScenarioTeam, teamID))
				break
			}
		}
	}
	c.Next()
}
