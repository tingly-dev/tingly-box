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

// ImageGenImage is one archived image (see internal/vision/imagestore). The
// pixels are fetched separately from GET /imagegen/images/{id}/file.
type ImageGenImage struct {
	ID        string `json:"id" example:"20260930-153012-a1b2c3"`
	CreatedAt string `json:"created_at" example:"2026-09-30T15:30:12+08:00"`
	Bytes     int64  `json:"bytes" example:"1843210"`
	Prompt    string `json:"prompt"`
	// Operation is "" for a generation, "edit" or "import".
	Operation string `json:"operation"`
	Model     string `json:"model"`
	Size      string `json:"size"`
	Quality   string `json:"quality"`
}

// ImageGenImageListResponse is one page of the archive, newest first.
// NextBefore, when set, is the `before` value for the next page.
type ImageGenImageListResponse struct {
	Success    bool            `json:"success" example:"true"`
	Images     []ImageGenImage `json:"images"`
	NextBefore string          `json:"next_before,omitempty"`
}

// ImageGenImageResponse wraps a single archived image.
type ImageGenImageResponse struct {
	Success bool          `json:"success" example:"true"`
	Image   ImageGenImage `json:"image"`
}

// ImageGenImportRequest brings an image the user already has into the
// archive, so it can be a workbench root. Data is base64 (a data: URL prefix
// is accepted and stripped).
type ImageGenImportRequest struct {
	Data string `json:"data" binding:"required"`
	// Name is kept as the sidecar's prompt line, so the archive says where
	// the image came from.
	Name string `json:"name"`
}

// ImageGenWorkbenchItem is an image derived inside a workbench.
type ImageGenWorkbenchItem struct {
	ImageID  string `json:"image_id"`
	ParentID string `json:"parent_id"`
	AddedAt  string `json:"added_at,omitempty"`
}

// ImageGenWorkbench is a focus: one root image and the description of its
// subject, plus what has been derived from it.
type ImageGenWorkbench struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	RootImageID string                  `json:"root_image_id"`
	Items       []ImageGenWorkbenchItem `json:"items"`
	CreatedAt   string                  `json:"created_at"`
	UpdatedAt   string                  `json:"updated_at"`
}

type ImageGenWorkbenchListResponse struct {
	Success     bool                `json:"success" example:"true"`
	Workbenches []ImageGenWorkbench `json:"workbenches"`
}

type ImageGenWorkbenchResponse struct {
	Success   bool              `json:"success" example:"true"`
	Workbench ImageGenWorkbench `json:"workbench"`
}

type ImageGenWorkbenchCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RootImageID string `json:"root_image_id" binding:"required"`
}

// ImageGenWorkbenchUpdateRequest changes only the fields that are present.
type ImageGenWorkbenchUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type ImageGenWorkbenchAddItemsRequest struct {
	Items []ImageGenWorkbenchItem `json:"items" binding:"required"`
}

// ImageGenOKResponse is the envelope for a delete.
type ImageGenOKResponse struct {
	Success bool   `json:"success" example:"true"`
	Error   string `json:"error,omitempty"`
}
