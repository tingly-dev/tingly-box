package managedagent

import (
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes wires /managed-agent onto an authenticated route group.
func RegisterRoutes(apiV1 *swagger.RouteGroup, h *Handler) {
	apiV1.GET("/managed-agent/folders/recent", h.RecentFolders,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("List folders web sessions have recently worked in"),
		swagger.WithQuery("limit", "int", "Maximum folders to return (0 = no limit)"),
		swagger.WithResponseModel(RecentFoldersResponse{}),
	)
	apiV1.GET("/managed-agent/fs/dirs", h.ListDirs,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("List the subdirectories of a path, for picking a folder to work in"),
		swagger.WithQuery("path", "string", "Absolute directory to list; empty defaults to the home directory"),
		swagger.WithResponseModel(ListDirsResponse{}),
	)
	apiV1.GET("/managed-agent/permission-modes", h.PermissionModes,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("List the selectable Claude Code permission modes"),
		swagger.WithResponseModel(PermissionModesResponse{}),
	)

	apiV1.POST("/managed-agent/sessions", h.CreateSession,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Open a session in a folder and start its first turn"),
		swagger.WithRequestModel(CreateSessionRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
	)
	apiV1.GET("/managed-agent/sessions", h.ListSessions,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("List this web front door's own sessions, most recently active first"),
		swagger.WithQuery("active", "bool", "Only include sessions that are pending, running, completed, or failed"),
		swagger.WithResponseModel(SessionListResponse{}),
	)
	apiV1.GET("/managed-agent/sessions/:session_id", h.GetSession,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Get a session"),
		swagger.WithResponseModel(SessionInfo{}),
	)
	apiV1.GET("/managed-agent/sessions/:session_id/messages", h.Messages,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Get a session's transcript"),
		swagger.WithResponseModel(MessageListResponse{}),
	)
	apiV1.POST("/managed-agent/sessions/:session_id/messages", h.SendMessage,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Send a message and start the next turn"),
		swagger.WithRequestModel(SendMessageRequest{}),
	)
	apiV1.POST("/managed-agent/sessions/:session_id/respond", h.Respond,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Answer a pending approval or ask request"),
		swagger.WithRequestModel(RespondRequest{}),
	)
	apiV1.PUT("/managed-agent/sessions/:session_id/permission-mode", h.SetPermissionMode,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Change a session's permission mode for its next turn"),
		swagger.WithRequestModel(SetPermissionModeRequest{}),
		swagger.WithResponseModel(SessionInfo{}),
	)
	apiV1.POST("/managed-agent/sessions/:session_id/interrupt", h.Interrupt,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Stop the current turn; the session stays resumable"),
	)
	apiV1.POST("/managed-agent/sessions/:session_id/archive", h.Archive,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("End a session for good; the folder and transcript are untouched"),
		swagger.WithResponseModel(SessionInfo{}),
	)
	apiV1.GET("/managed-agent/sessions/:session_id/diff", h.Diff,
		swagger.WithTags("managed-agent"),
		swagger.WithDescription("Summarise what the agent changed in the folder"),
		swagger.WithResponseModel(managedagent.Diff{}),
	)
}
