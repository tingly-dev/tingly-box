// Package imagegen exposes control-plane HTTP endpoints for the image
// generation scenario — separate from the OpenAI-compatible gateway routes
// under /tingly/imagegen, which only ever see requests/responses, never the
// local filesystem.
package imagegen

// ImageGenInfoResponse is the JSON envelope for GET /api/v1/imagegen/info.
// A general info endpoint rather than one named after its first field —
// OutputDir is the only thing worth reporting today, but future fields
// (e.g. how many images are saved, or the most recent one) belong here too,
// without forcing a new endpoint per field. Named with the module's own
// prefix, not a bare "InfoResponse" — swagger's model registry keys schemas
// by Go type name, and several other modules already have their own info
// response shape.
type ImageGenInfoResponse struct {
	Success bool `json:"success" example:"true"`
	// OutputDir is the local path generated/edited images are persisted to.
	// Read-only: the frontend shows it so the user can find the folder
	// themselves (copy it, paste it into their own file manager) rather than
	// the server reaching for a file-manager window on their behalf, which
	// would be meaningless whenever the browser and this server aren't the
	// same machine.
	OutputDir string `json:"output_dir" example:"/home/user/.tingly-box/image"`
}
