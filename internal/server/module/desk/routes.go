package desk

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// gate rejects every Desk request while the feature flag is off,
// so disabling it in settings actually turns the surface off rather than
// only hiding its nav entry — this runs an arbitrary local coding agent on
// the host, so "hidden but reachable" is not an acceptable disabled state.
func gate(enabled func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled == nil || !enabled() {
			apierr.Send(c, http.StatusNotFound, errors.New("desk is disabled"), "not_found_error")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RegisterRoutes wires /desk onto an authenticated route group.
// enabled reports whether the desk feature flag is on; every
// route 404s while it is off (see gate).
func RegisterRoutes(apiV1 *swagger.RouteGroup, h *Handler, enabled func() bool) {
	mw := swagger.WithMiddleware(gate(enabled))

	apiV1.GET("/desk/folders/recent", h.RecentFolders,
		swagger.WithTags("desk"),
		swagger.WithDescription("List folders web sessions have recently worked in"),
		swagger.WithQuery("limit", "int", "Maximum folders to return (0 = no limit)"),
		swagger.WithResponseModel(RecentFoldersResponse{}),
		mw,
	)
	apiV1.GET("/desk/permission-modes", h.PermissionModes,
		swagger.WithTags("desk"),
		swagger.WithDescription("List the selectable Claude Code permission modes"),
		swagger.WithResponseModel(PermissionModesResponse{}),
		mw,
	)

	apiV1.POST("/desk/sessions", h.CreateSession,
		swagger.WithTags("desk"),
		swagger.WithDescription("Open a session in a folder and start its first turn"),
		swagger.WithRequestModel(CreateSessionRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.GET("/desk/sessions", h.ListSessions,
		swagger.WithTags("desk"),
		swagger.WithDescription("List this web front door's own sessions, most recently active first"),
		swagger.WithQuery("active", "bool", "Only include sessions that are pending, running, completed, or failed"),
		swagger.WithResponseModel(SessionListResponse{}),
		mw,
	)
	apiV1.GET("/desk/sessions/:session_id", h.GetSession,
		swagger.WithTags("desk"),
		swagger.WithDescription("Get a session"),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.GET("/desk/sessions/:session_id/messages", h.Messages,
		swagger.WithTags("desk"),
		swagger.WithDescription("Get a session's transcript"),
		swagger.WithResponseModel(MessageListResponse{}),
		mw,
	)
	apiV1.POST("/desk/sessions/:session_id/messages", h.SendMessage,
		swagger.WithTags("desk"),
		swagger.WithDescription("Send a message and start the next turn"),
		swagger.WithRequestModel(SendMessageRequest{}),
		mw,
	)
	apiV1.POST("/desk/sessions/:session_id/respond", h.Respond,
		swagger.WithTags("desk"),
		swagger.WithDescription("Answer a pending approval or ask request"),
		swagger.WithRequestModel(RespondRequest{}),
		mw,
	)
	apiV1.POST("/desk/sessions/:session_id/handoff", h.Handoff,
		swagger.WithTags("desk"),
		swagger.WithDescription("Release a session so it can be continued in a terminal, returning the command"),
		swagger.WithResponseModel(HandoffResponse{}),
		mw,
	)
	apiV1.GET("/desk/sessions/:session_id/status", h.Status,
		swagger.WithTags("desk"),
		swagger.WithDescription("Where a session's model requests are routed and the quota they draw on"),
		swagger.WithResponseModel(SessionStatusResponse{}),
		mw,
	)
	apiV1.PUT("/desk/sessions/:session_id/profile", h.SetProfile,
		swagger.WithTags("desk"),
		swagger.WithDescription("Change which Claude Code profile a session's next turn runs with"),
		swagger.WithRequestModel(SetProfileRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.PUT("/desk/sessions/:session_id/model", h.SetModel,
		swagger.WithTags("desk"),
		swagger.WithDescription("Change which model tier a session's next turn asks for"),
		swagger.WithRequestModel(SetModelRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.PUT("/desk/sessions/:session_id/permission-mode", h.SetPermissionMode,
		swagger.WithTags("desk"),
		swagger.WithDescription("Change a session's permission mode for its next turn"),
		swagger.WithRequestModel(SetPermissionModeRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
	apiV1.POST("/desk/sessions/:session_id/interrupt", h.Interrupt,
		swagger.WithTags("desk"),
		swagger.WithDescription("Stop the current turn; the session stays resumable"),
		mw,
	)
	apiV1.POST("/desk/sessions/:session_id/archive", h.Archive,
		swagger.WithTags("desk"),
		swagger.WithDescription("End a session for good; the folder and transcript are untouched"),
		swagger.WithResponseModel(SessionInfo{}),
		mw,
	)
}
