package imageasset

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Request and response bodies. Named with the package's prefix because the
// API schema keys models by Go type name across all of tingly-box.

type ImageAssetPiecesResponse struct {
	Success bool          `json:"success" example:"true"`
	Pieces  []PromptPiece `json:"pieces"`
}

type ImageAssetSavePiecesRequest struct {
	Pieces []PromptPieceInput `json:"pieces" binding:"required"`
}

type ImageAssetReferencesResponse struct {
	Success    bool             `json:"success" example:"true"`
	References []ReferenceImage `json:"references"`
}

type ImageAssetReferenceUpload struct {
	Name string `json:"name" binding:"required" example:"character-sheet.png"`
	// A base64 data: URL. The declared type is ignored; the bytes decide.
	DataURL string `json:"data_url" binding:"required" example:"data:image/png;base64,iVBORw0..."`
}

type ImageAssetAddReferencesRequest struct {
	References []ImageAssetReferenceUpload `json:"references" binding:"required"`
}

type ImageAssetAddReferencesResponse struct {
	Success bool `json:"success" example:"true"`
	// In request order. `existing` marks an image that was already kept.
	References []AddedReferenceImage `json:"references"`
}

type ImageAssetRenameReferenceRequest struct {
	Name string `json:"name" binding:"required" example:"hero.png"`
}

type ImageAssetReferenceResponse struct {
	Success   bool           `json:"success" example:"true"`
	Reference ReferenceImage `json:"reference"`
}

type ImageAssetDeleteResponse struct {
	Success bool `json:"success" example:"true"`
}

// Handler serves the image assets API over a Store.
type Handler struct {
	store *Store
}

// NewHandler returns a Handler over store.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// maxUploadBody bounds one add-references request: the per-call image count
// times the per-image limit, as base64 (4/3), plus room for the JSON around it.
const maxUploadBody = maxImagesPerCall*maxImageBytes*4/3 + 1<<20

func fail(c *gin.Context, err error) {
	status, kind := http.StatusInternalServerError, "server_error"
	switch {
	case errors.Is(err, ErrInvalid):
		status, kind = http.StatusBadRequest, "invalid_request_error"
	case errors.Is(err, ErrNotFound):
		status, kind = http.StatusNotFound, "not_found_error"
	default:
		logrus.WithError(err).Error("image assets request failed")
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"message": err.Error(), "type": kind}})
}

func badRequest(c *gin.Context, err error) {
	fail(c, invalid(err.Error()))
}

// ListPieces handles GET /image-assets/pieces.
func (h *Handler) ListPieces(c *gin.Context) {
	pieces, err := h.store.ListPieces()
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageAssetPiecesResponse{Success: true, Pieces: pieces})
}

// SavePieces handles POST /image-assets/pieces.
func (h *Handler) SavePieces(c *gin.Context) {
	var req ImageAssetSavePiecesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	pieces, err := h.store.SavePieces(req.Pieces)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageAssetPiecesResponse{Success: true, Pieces: pieces})
}

// DeletePiece handles DELETE /image-assets/pieces/:id.
func (h *Handler) DeletePiece(c *gin.Context) {
	if err := h.store.DeletePiece(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageAssetDeleteResponse{Success: true})
}

// ListReferences handles GET /image-assets/references.
func (h *Handler) ListReferences(c *gin.Context) {
	references, err := h.store.ListReferences()
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageAssetReferencesResponse{Success: true, References: references})
}

// AddReferences handles POST /image-assets/references.
func (h *Handler) AddReferences(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBody)
	var req ImageAssetAddReferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	inputs := make([]ReferenceInput, 0, len(req.References))
	for _, upload := range req.References {
		data, err := decodeDataURL(upload.DataURL)
		if err != nil {
			fail(c, err)
			return
		}
		inputs = append(inputs, ReferenceInput{Name: upload.Name, Data: data})
	}
	added, err := h.store.AddReferences(inputs)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageAssetAddReferencesResponse{Success: true, References: added})
}

// RenameReference handles PUT /image-assets/references/:id.
func (h *Handler) RenameReference(c *gin.Context) {
	var req ImageAssetRenameReferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err)
		return
	}
	reference, err := h.store.RenameReference(c.Param("id"), req.Name)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageAssetReferenceResponse{Success: true, Reference: reference})
}

// DeleteReference handles DELETE /image-assets/references/:id.
func (h *Handler) DeleteReference(c *gin.Context) {
	if err := h.store.DeleteReference(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageAssetDeleteResponse{Success: true})
}

// ReferenceContent handles GET /image-assets/references/:id/content: the
// image's bytes. A reference's bytes never change under its id, so the
// response may be cached for good.
func (h *Handler) ReferenceContent(c *gin.Context) {
	path, mime, err := h.store.ReferenceFile(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Type", mime)
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(path)
}
