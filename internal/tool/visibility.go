package tool

// ToolVisibility controls whether a tool is exposed to clients or kept server-side.
type ToolVisibility string

const (
	ToolVisibilityClient ToolVisibility = "client"
	ToolVisibilityServer ToolVisibility = "server"
)
