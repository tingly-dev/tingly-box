package localsessions

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	r := rt.V2
	r.GET("/local-sessions/status", h.GetStatus, swagger.WithAuth(), swagger.WithTags("local-sessions"), swagger.WithDescription("Report whether local session browsing is enabled and this connection is local. Does not scan files."), swagger.WithResponseModel(LocalSessionStatusResponse{}))
	r.GET("/local-sessions/projects", h.ListProjects, swagger.WithAuth(), swagger.WithTags("local-sessions"), swagger.WithDescription("List Claude Code project directories on the server computer. Requires authenticated direct loopback access, a loopback-only server bind and the local_sessions feature flag."), swagger.WithQueryConfig("cursor", swagger.QueryParamConfig{Type: "string", Description: "Opaque next-page cursor"}), swagger.WithResponseModel(LocalSessionListResponse{}))
	r.GET("/local-sessions/projects/:project/sessions", h.ListSessions, swagger.WithAuth(), swagger.WithTags("local-sessions"), swagger.WithDescription("List 25 session files by modification time, reading only bounded title prefixes for the displayed page."), swagger.WithQueryConfig("cursor", swagger.QueryParamConfig{Type: "string", Description: "Opaque next-page cursor"}), swagger.WithResponseModel(LocalSessionListResponse{}))
	r.GET("/local-sessions/sessions/:session/events", h.Preview, swagger.WithAuth(), swagger.WithTags("local-sessions"), swagger.WithDescription("Read a bounded native JSONL preview: at most 2 MiB scanned, 50 events, 192 KiB of event JSON. File changes invalidate cursors."), swagger.WithQueryConfig("cursor", swagger.QueryParamConfig{Type: "string", Description: "Opaque file-version-bound cursor"}), swagger.WithQueryConfig("direction", swagger.QueryParamConfig{Type: "string", Description: "backward (recent) or forward (beginning); only used without a cursor"}), swagger.WithResponseModel(LocalSessionPreviewResponse{}))
}
