// Package imagegen exposes control-plane HTTP endpoints for the image
// generation scenario — separate from the OpenAI-compatible gateway routes
// under /tingly/imagegen, which only ever see requests/responses, never the
// local filesystem.
package imagegen

// OutputDirResponse is the JSON envelope for GET /api/v1/imagegen/output-dir.
// Read-only: the frontend shows Path so the user can find the folder
// themselves (copy it, paste it into their own file manager) rather than the
// server reaching for a file-manager window on their behalf, which would be
// meaningless whenever the browser and this server aren't the same machine.
type OutputDirResponse struct {
	Success bool   `json:"success" example:"true"`
	Path    string `json:"path" example:"/home/user/.tingly-box/image"`
}
