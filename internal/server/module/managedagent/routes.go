package managedagent

import (
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/swagger"
)

const tag = "agent"

// RegisterRoutes mounts /agent/* on an authenticated /api/v1 group.
func RegisterRoutes(group *swagger.RouteGroup, h *Handler) {
	// Sources
	group.GET("/agent/sources", h.ListSources,
		swagger.WithTags(tag),
		swagger.WithDescription("List git sources the agent can work on"),
		swagger.WithResponseModel(SourceListResponse{}))
	group.POST("/agent/sources", h.CreateSource,
		swagger.WithTags(tag),
		swagger.WithDescription("Add a git source"),
		swagger.WithRequestModel(SourceRequest{}),
		swagger.WithResponseModel(managedagent.Source{}))
	group.GET("/agent/sources/:source_id", h.GetSource,
		swagger.WithTags(tag),
		swagger.WithDescription("Get a source"),
		swagger.WithResponseModel(managedagent.Source{}))
	group.PUT("/agent/sources/:source_id", h.UpdateSource,
		swagger.WithTags(tag),
		swagger.WithDescription("Update a source"),
		swagger.WithRequestModel(SourceRequest{}),
		swagger.WithResponseModel(managedagent.Source{}))
	group.DELETE("/agent/sources/:source_id", h.DeleteSource,
		swagger.WithTags(tag),
		swagger.WithDescription("Delete a source that has no live workspaces"))

	// Environments
	group.GET("/agent/environments", h.ListEnvironments,
		swagger.WithTags(tag),
		swagger.WithDescription("List environments (default first) and the runtimes available today"),
		swagger.WithResponseModel(EnvironmentListResponse{}))
	group.POST("/agent/environments", h.CreateEnvironment,
		swagger.WithTags(tag),
		swagger.WithDescription("Create an environment"),
		swagger.WithRequestModel(EnvironmentRequest{}),
		swagger.WithResponseModel(managedagent.Environment{}))
	group.GET("/agent/environments/:environment_id", h.GetEnvironment,
		swagger.WithTags(tag),
		swagger.WithDescription("Get an environment"),
		swagger.WithResponseModel(managedagent.Environment{}))
	group.PUT("/agent/environments/:environment_id", h.UpdateEnvironment,
		swagger.WithTags(tag),
		swagger.WithDescription("Update an environment"),
		swagger.WithRequestModel(EnvironmentRequest{}),
		swagger.WithResponseModel(managedagent.Environment{}))
	group.DELETE("/agent/environments/:environment_id", h.DeleteEnvironment,
		swagger.WithTags(tag),
		swagger.WithDescription("Delete a non-default environment that has no live workspaces"))

	// Sessions
	group.GET("/agent/sessions", h.ListSessions,
		swagger.WithTags(tag),
		swagger.WithDescription("List sessions, most recently active first"),
		swagger.WithQueryModel(SessionListQuery{}),
		swagger.WithResponseModel(SessionListResponse{}))
	group.POST("/agent/sessions", h.CreateSession,
		swagger.WithTags(tag),
		swagger.WithDescription("Open a session: materialise a workspace from a source (or reuse one) and queue the prompt"),
		swagger.WithRequestModel(CreateSessionRequest{}),
		swagger.WithResponseModel(SessionDetail{}))
	group.GET("/agent/sessions/:session_id", h.GetSession,
		swagger.WithTags(tag),
		swagger.WithDescription("Get a session with its workspace"),
		swagger.WithResponseModel(SessionDetail{}))
	group.GET("/agent/sessions/:session_id/events", h.Events,
		swagger.WithTags(tag),
		swagger.WithDescription("Page a session's event log (JSON), or stream it with Accept: text/event-stream"),
		swagger.WithQueryModel(EventListQuery{}),
		swagger.WithResponseModel(EventListResponse{}))
	group.POST("/agent/sessions/:session_id/messages", h.SendMessage,
		swagger.WithTags(tag),
		swagger.WithDescription("Append a user message (steer the agent)"),
		swagger.WithRequestModel(SendMessageRequest{}))
	group.POST("/agent/sessions/:session_id/respond", h.Respond,
		swagger.WithTags(tag),
		swagger.WithDescription("Answer a pending approval or ask request"),
		swagger.WithRequestModel(RespondRequest{}))
	group.POST("/agent/sessions/:session_id/interrupt", h.Interrupt,
		swagger.WithTags(tag),
		swagger.WithDescription("Stop the current turn; the session stays resumable"))
	group.POST("/agent/sessions/:session_id/archive", h.Archive,
		swagger.WithTags(tag),
		swagger.WithDescription("End a session; its branch and log are kept"),
		swagger.WithResponseModel(SessionDetail{}))
}
