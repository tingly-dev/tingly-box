package imagegen

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/internal/server/module/bind"
	"github.com/tingly-dev/tingly-box/internal/server/module/paginate"
	"github.com/tingly-dev/tingly-box/internal/vision/imagestore"
)

// Handler serves /api/v1/imagegen endpoints: read-only scenario info, the
// image archive the gateway persists into, and the focus workbenches on top
// of it. It holds no state beyond the directory: every read goes to disk, so
// a file added or removed by hand is seen on the next request.
type Handler struct {
	images      *imagestore.Store
	workbenches *imagestore.Workbenches
}

// NewHandler creates a Handler rooted at configDir's image directory — the
// same directory protocolserver.persistImages writes to. An empty configDir
// falls back to the default config directory, as persistImages does.
func NewHandler(configDir string) *Handler {
	if configDir == "" {
		configDir = constant.GetTinglyConfDir()
	}
	images := imagestore.New(constant.GetImageDir(configDir))
	return &Handler{images: images, workbenches: imagestore.NewWorkbenches(images)}
}

// GetInfo handles GET /api/v1/imagegen/info: reports read-only facts about
// the imagegen scenario, currently just the directory generated/edited
// images are persisted to (~/.tingly-box/image/YYYYMMDD/*.png, see
// persistImages). Side-effect-free — this server never reaches into the
// local OS to open a file manager window; the path is handed to the
// frontend so the user can navigate there themselves, which also works
// whenever the browser isn't on the same machine as this server.
func (h *Handler) GetInfo(c *gin.Context) {
	c.JSON(http.StatusOK, ImageGenInfoResponse{Success: true, OutputDir: h.images.Root()})
}

func fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, imagestore.ErrInvalidID):
		status = http.StatusBadRequest
	case errors.Is(err, imagestore.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, imagestore.ErrRootInUse):
		status = http.StatusConflict
	}
	apierr.Failure(c, status, err.Error())
}

func toImage(img imagestore.Image) ImageGenImage {
	return ImageGenImage{
		ID:        img.ID,
		CreatedAt: img.CreatedAt.Format(time.RFC3339),
		Bytes:     img.Bytes,
		Prompt:    img.Meta.Prompt,
		Operation: img.Meta.Operation,
		Model:     img.Meta.Model,
		Size:      img.Meta.Size,
		Quality:   img.Meta.Quality,
	}
}

func toWorkbench(wb *imagestore.Workbench) ImageGenWorkbench {
	items := make([]ImageGenWorkbenchItem, 0, len(wb.Items))
	for _, it := range wb.Items {
		items = append(items, ImageGenWorkbenchItem{
			ImageID:  it.ImageID,
			ParentID: it.ParentID,
			AddedAt:  it.AddedAt.Format(time.RFC3339),
		})
	}
	return ImageGenWorkbench{
		ID:          wb.ID,
		Name:        wb.Name,
		Description: wb.Description,
		RootImageID: wb.RootImageID,
		Items:       items,
		CreatedAt:   wb.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   wb.UpdatedAt.Format(time.RFC3339),
	}
}

// ListImages handles GET /api/v1/imagegen/images?limit=&before=.
func (h *Handler) ListImages(c *gin.Context) {
	limit := paginate.Limit(c, 50, 200)
	imgs, err := h.images.List(limit, c.Query("before"))
	if err != nil {
		fail(c, err)
		return
	}
	resp := ImageGenImageListResponse{Success: true, Images: make([]ImageGenImage, 0, len(imgs))}
	for _, img := range imgs {
		resp.Images = append(resp.Images, toImage(img))
	}
	if len(imgs) == limit {
		resp.NextBefore = imgs[len(imgs)-1].ID
	}
	c.JSON(http.StatusOK, resp)
}

// GetImage handles GET /api/v1/imagegen/images/:id.
func (h *Handler) GetImage(c *gin.Context) {
	img, err := h.images.Get(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageGenImageResponse{Success: true, Image: toImage(img)})
}

// GetImageFile handles GET /api/v1/imagegen/images/:id/file: the PNG itself.
// Ids never change content, so the browser may cache it for good.
func (h *Handler) GetImageFile(c *gin.Context) {
	p, err := h.images.Path(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.File(p)
}

// ImportImage handles POST /api/v1/imagegen/images.
func (h *Handler) ImportImage(c *gin.Context) {
	var req ImageGenImportRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	payload := req.Data
	if i := strings.Index(payload, ";base64,"); strings.HasPrefix(payload, "data:") && i >= 0 {
		payload = payload[i+len(";base64,"):]
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		apierr.Failure(c, http.StatusBadRequest, "data is not base64 image data")
		return
	}
	if ct := http.DetectContentType(data); !strings.HasPrefix(ct, "image/") {
		apierr.Failure(c, http.StatusBadRequest, "data is not an image ("+ct+")")
		return
	}
	id, err := h.images.Save(data, imagestore.Meta{Prompt: req.Name, Operation: "import"})
	if id == "" {
		fail(c, err)
		return
	}
	img, err := h.images.Get(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageGenImageResponse{Success: true, Image: toImage(img)})
}

// DeleteImage handles DELETE /api/v1/imagegen/images/:id. Refused (409) when
// the image is a workbench's root; removed from any workbench it is an item of.
func (h *Handler) DeleteImage(c *gin.Context) {
	if err := h.workbenches.DeleteImage(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageGenOKResponse{Success: true})
}

// ListWorkbenches handles GET /api/v1/imagegen/workbenches.
func (h *Handler) ListWorkbenches(c *gin.Context) {
	all, err := h.workbenches.List()
	if err != nil {
		fail(c, err)
		return
	}
	resp := ImageGenWorkbenchListResponse{Success: true, Workbenches: make([]ImageGenWorkbench, 0, len(all))}
	for i := range all {
		resp.Workbenches = append(resp.Workbenches, toWorkbench(&all[i]))
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) respondWorkbench(c *gin.Context, wb *imagestore.Workbench, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageGenWorkbenchResponse{Success: true, Workbench: toWorkbench(wb)})
}

// GetWorkbench handles GET /api/v1/imagegen/workbenches/:id.
func (h *Handler) GetWorkbench(c *gin.Context) {
	wb, err := h.workbenches.Get(c.Param("id"))
	h.respondWorkbench(c, wb, err)
}

// CreateWorkbench handles POST /api/v1/imagegen/workbenches.
func (h *Handler) CreateWorkbench(c *gin.Context) {
	var req ImageGenWorkbenchCreateRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	wb, err := h.workbenches.Create(req.Name, req.Description, req.RootImageID)
	h.respondWorkbench(c, wb, err)
}

// UpdateWorkbench handles PUT /api/v1/imagegen/workbenches/:id.
func (h *Handler) UpdateWorkbench(c *gin.Context) {
	var req ImageGenWorkbenchUpdateRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	wb, err := h.workbenches.Update(c.Param("id"), req.Name, req.Description)
	h.respondWorkbench(c, wb, err)
}

// DeleteWorkbench handles DELETE /api/v1/imagegen/workbenches/:id. The images
// stay in the archive.
func (h *Handler) DeleteWorkbench(c *gin.Context) {
	if err := h.workbenches.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ImageGenOKResponse{Success: true})
}

// AddWorkbenchItems handles POST /api/v1/imagegen/workbenches/:id/items.
func (h *Handler) AddWorkbenchItems(c *gin.Context) {
	var req ImageGenWorkbenchAddItemsRequest
	if !bind.JSON(c, &req, apierr.Failure) {
		return
	}
	items := make([]imagestore.WorkbenchItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, imagestore.WorkbenchItem{ImageID: it.ImageID, ParentID: it.ParentID})
	}
	wb, err := h.workbenches.AddItems(c.Param("id"), items)
	h.respondWorkbench(c, wb, err)
}

// RemoveWorkbenchItem handles DELETE /api/v1/imagegen/workbenches/:id/items/:image_id.
// The image stays in the archive.
func (h *Handler) RemoveWorkbenchItem(c *gin.Context) {
	wb, err := h.workbenches.RemoveItem(c.Param("id"), c.Param("image_id"))
	h.respondWorkbench(c, wb, err)
}
