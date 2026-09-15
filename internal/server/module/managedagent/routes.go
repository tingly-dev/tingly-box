package managedagent

import (
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/swagger"
)

const tag = "agent"

// RegisterRoutes mounts /agent/* on an authenticated /api/v1 group.
//
// The surface is the local story and nothing else: folders the agent may
// work in, and sessions in them. Cloning a repository, branches and pushes
// are a later phase (.design/managed-agent.md §18).
func RegisterRoutes(group *swagger.RouteGroup, h *Handler) {
	// Folders — the directories the agent may work in, and the browse allowlist.
	group.GET("/agent/folders", h.ListFolders,
		swagger.WithTags(tag),
		swagger.WithDescription("List the folders the agent may work in, most recently used first"),
		swagger.WithResponseModel(FolderListResponse{}))
	group.POST("/agent/folders", h.AddFolder,
		swagger.WithTags(tag),
		swagger.WithDescription("Hand a folder on this host to the agent; adding the same path twice returns the existing folder"),
		swagger.WithRequestModel(AddFolderRequest{}),
		swagger.WithResponseModel(managedagent.Folder{}))
	group.DELETE("/agent/folders/:folder_id", h.RemoveFolder,
		swagger.WithTags(tag),
		swagger.WithDescription("Withdraw a folder. The directory and everything in it stay untouched; a folder with an active task is refused"))
	group.GET("/agent/fs/dirs", h.BrowseDirs,
		swagger.WithTags(tag),
		swagger.WithDescription("List sub-directories inside an added folder; an empty path lists the added folders themselves, anything outside them is 403"),
		swagger.WithQuery("path", "string", "Absolute directory path inside an added folder; empty for the list of added folders"),
		swagger.WithResponseModel(managedagent.DirListing{}))
	group.GET("/agent/permission-modes", h.PermissionModes,
		swagger.WithTags(tag),
		swagger.WithDescription("Claude Code permission modes this build offers, in display order"),
		swagger.WithResponseModel(PermissionModeListResponse{}))

	// Sessions
	group.GET("/agent/sessions", h.ListSessions,
		swagger.WithTags(tag),
		swagger.WithDescription("List sessions, most recently active first"),
		swagger.WithQuery("folder_id", "string", "Filter by folder"),
		swagger.WithQuery("status", "string", "Filter by status"),
		swagger.WithQuery("active", "boolean", "Only sessions that can still be given work"),
		swagger.WithQuery("limit", "integer", "Maximum rows"),
		swagger.WithResponseModel(SessionListResponse{}))
	group.POST("/agent/sessions", h.CreateSession,
		swagger.WithTags(tag),
		swagger.WithDescription("Start a task in a folder. Give folder_id, or a path to add the folder and start in one step. A folder runs one task at a time"),
		swagger.WithRequestModel(CreateSessionRequest{}),
		swagger.WithResponseModel(SessionDetail{}))
	group.GET("/agent/sessions/:session_id", h.GetSession,
		swagger.WithTags(tag),
		swagger.WithDescription("Get a session with its folder"),
		swagger.WithResponseModel(SessionDetail{}))
	group.GET("/agent/sessions/:session_id/events", h.Events,
		swagger.WithTags(tag),
		swagger.WithDescription("Page the session's conversation log; Accept: text/event-stream streams it instead"),
		swagger.WithQuery("after", "integer", "Return events with seq greater than this"),
		swagger.WithQuery("limit", "integer", "Maximum events"),
		swagger.WithResponseModel(EventListResponse{}))
	group.POST("/agent/sessions/:session_id/messages", h.SendMessage,
		swagger.WithTags(tag),
		swagger.WithDescription("Steer the agent: append a user turn"),
		swagger.WithRequestModel(SendMessageRequest{}))
	group.POST("/agent/sessions/:session_id/respond", h.Respond,
		swagger.WithTags(tag),
		swagger.WithDescription("Answer a pending permission request or question"),
		swagger.WithRequestModel(RespondRequest{}))
	group.PUT("/agent/sessions/:session_id/permission-mode", h.SetPermissionMode,
		swagger.WithTags(tag),
		swagger.WithDescription("Change an active session's Claude Code permission mode; applies from the next turn"),
		swagger.WithRequestModel(SetPermissionModeRequest{}),
		swagger.WithResponseModel(SessionDetail{}))
	group.POST("/agent/sessions/:session_id/interrupt", h.Interrupt,
		swagger.WithTags(tag),
		swagger.WithDescription("Stop the current turn; the session stays resumable"))
	group.POST("/agent/sessions/:session_id/archive", h.Archive,
		swagger.WithTags(tag),
		swagger.WithDescription("End the session for good. Nothing on disk is touched: the folder, its files and the log all stay"),
		swagger.WithResponseModel(SessionDetail{}))
	group.GET("/agent/sessions/:session_id/diff", h.Diff,
		swagger.WithTags(tag),
		swagger.WithDescription("What the agent changed in the folder since this session started; an empty diff when the folder is not a git work tree"),
		swagger.WithResponseModel(managedagent.Diff{}))
}
