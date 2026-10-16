package github

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// gate rejects every request while the `github` flag is off, failing closed
// when no predicate was given (same contract as the desk module).
func gate(enabled func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled == nil || !enabled() {
			apierr.Send(c, http.StatusNotFound, errors.New("github integration is disabled"), "not_found_error")
			c.Abort()
			return
		}
		c.Next()
	}
}

var _ module.Module = (*Handler)(nil)

// RegisterRoutes wires /api/v1/github (authenticated) plus the three routes
// that live outside the authenticated API: GitHub's two browser redirects
// and the agents' MCP endpoint (authenticated by its own session bearer).
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	apiV1 := rt.V1
	mw := swagger.WithMiddleware(gate(h.enabled))

	apiV1.GET("/github/status", h.Status,
		swagger.WithTags("github"),
		swagger.WithDescription("The GitHub connection: App, installed accounts, fallback token"),
		swagger.WithQuery("live", "bool", "Also ask GitHub for each installation's repository count"),
		swagger.WithResponseModel(StatusResponse{}),
		mw,
	)
	apiV1.POST("/github/connect", h.Connect,
		swagger.WithTags("github"),
		swagger.WithDescription("Start creating this Tingly Box's GitHub App; returns the form to post to GitHub"),
		swagger.WithRequestModel(ConnectRequest{}),
		swagger.WithResponseModel(ConnectResponse{}),
		mw,
	)
	apiV1.POST("/github/sync", h.Sync,
		swagger.WithTags("github"),
		swagger.WithDescription("Refresh the App's installations from GitHub"),
		swagger.WithResponseModel(OKResponse{}),
		mw,
	)
	apiV1.DELETE("/github", h.Disconnect,
		swagger.WithTags("github"),
		swagger.WithDescription("Forget the GitHub App, its installations and the fallback token"),
		swagger.WithResponseModel(OKResponse{}),
		mw,
	)
	apiV1.PUT("/github/pat", h.SetPAT,
		swagger.WithTags("github"),
		swagger.WithDescription("Store a fine-grained personal access token as the fallback"),
		swagger.WithRequestModel(PATRequest{}),
		swagger.WithResponseModel(OKResponse{}),
		mw,
	)
	apiV1.DELETE("/github/pat", h.ClearPAT,
		swagger.WithTags("github"),
		swagger.WithDescription("Remove the fallback token"),
		swagger.WithResponseModel(OKResponse{}),
		mw,
	)

	if e := rt.Engine; e != nil {
		g := gate(h.enabled)
		e.GET("/github/manifest/callback", g, h.ManifestCallback)
		e.GET("/github/install/callback", g, h.InstallCallback)
		if h.svc != nil {
			e.Any("/github/mcp", g, gin.WrapH(h.svc.MCPHandler()))
		}
	}
}
