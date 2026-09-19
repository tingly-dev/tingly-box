package managedagent

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// gate rejects every managed-agent request while the feature flag is off,
// so disabling it in settings actually turns the surface off rather than
// only hiding its nav entry — this runs an arbitrary local coding agent on
// the host, so "hidden but reachable" is not an acceptable disabled state.
func gate(enabled func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled == nil || !enabled() {
			apierr.Send(c, http.StatusNotFound, errors.New("managed agent is disabled"), "not_found_error")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RegisterRoutes wires /managed-agent onto an authenticated route group.
// enabled reports whether the experimental_managed_agent flag is on; every
// route 404s while it is off (see gate).
func RegisterRoutes(apiV1 *swagger.RouteGroup, h *Handler, enabled func() bool) {
	mw := swagger.WithMiddleware(gate(enabled))

	apiV1.GET("/managed-agent/folders/recent", h.RecentFolders,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("List folders web sessions have recently worked in"),
		swagger.WithQuery("limit", "int", "Maximum folders to return (0 = no limit)"),
		swagger.WithResponseModel(RecentFoldersResponse{}),
		mw,
	)
	apiV1.GET("/managed-agent/permission-modes", h.PermissionModes,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("List the selectable Claude Code permission modes"),
		swagger.WithResponseModel(PermissionModesResponse{}),
		mw,
	)

	apiV1.POST("/managed-agent/sessions", h.CreateSession,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Open a session in a folder and start its first turn"),
		swagger.WithRequestModel(CreateSessionRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.GET("/managed-agent/sessions", h.ListSessions,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("List this web front door's own sessions, most recently active first"),
		swagger.WithQuery("active", "bool", "Only include sessions that are pending, running, completed, or failed"),
		swagger.WithResponseModel(SessionListResponse{}),
		mw,
	)
	apiV1.GET("/managed-agent/sessions/:session_id", h.GetSession,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Get a session"),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.GET("/managed-agent/sessions/:session_id/messages", h.Messages,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Get a session's transcript"),
		swagger.WithResponseModel(MessageListResponse{}),
		mw,
	)
	apiV1.POST("/managed-agent/sessions/:session_id/messages", h.SendMessage,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Send a message and start the next turn"),
		swagger.WithRequestModel(SendMessageRequest{}),
		mw,
	)
	apiV1.POST("/managed-agent/sessions/:session_id/respond", h.Respond,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Answer a pending approval or ask request"),
		swagger.WithRequestModel(RespondRequest{}),
		mw,
	)
	apiV1.PUT("/managed-agent/sessions/:session_id/permission-mode", h.SetPermissionMode,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Change a session's permission mode for its next turn"),
		swagger.WithRequestModel(SetPermissionModeRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.POST("/managed-agent/sessions/:session_id/interrupt", h.Interrupt,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Stop the current turn; the session stays resumable"),
		mw,
	)
	apiV1.POST("/managed-agent/sessions/:session_id/archive", h.Archive,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("End a session for good; the folder and transcript are untouched"),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
}
