package imagegen

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

var _ module.Module = (*Handler)(nil)

// RegisterRoutes registers the imagegen control-plane routes.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	// GET /imagegen/info - Report read-only imagegen scenario info (output directory, ...)
	router.GET("/imagegen/info", h.GetInfo,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Get read-only info about the imagegen scenario, including the local output directory"),
		swagger.WithResponseModel(ImageGenInfoResponse{}))

	// --- Image archive: what the gateway persisted, newest first ---

	router.GET("/imagegen/images", h.ListImages,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("List archived generated/edited/imported images, newest first"),
		swagger.WithQuery("limit", "integer", "Page size (default 50, max 200)"),
		swagger.WithQuery("before", "string", "Return images older than this image id (from next_before)"),
		swagger.WithResponseModel(ImageGenImageListResponse{}))

	router.POST("/imagegen/images", h.ImportImage,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Import an image into the archive (e.g. to start a workbench from it)"),
		swagger.WithRequestModel(ImageGenImportRequest{}),
		swagger.WithResponseModel(ImageGenImageResponse{}))

	router.GET("/imagegen/images/:id", h.GetImage,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Get one archived image's metadata"),
		swagger.WithPathParam("id", "string", "Archived image id"),
		swagger.WithResponseModel(ImageGenImageResponse{}))

	router.GET("/imagegen/images/:id/file", h.GetImageFile,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Get an archived image's PNG bytes"),
		swagger.WithPathParam("id", "string", "Archived image id"))

	router.DELETE("/imagegen/images/:id", h.DeleteImage,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Delete an archived image; refused while it is a workbench root"),
		swagger.WithPathParam("id", "string", "Archived image id"),
		swagger.WithResponseModel(ImageGenOKResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 409, Message: "Image is the root of a workbench"},
		))

	// --- Workbenches: a focus on one image and its description ---

	router.GET("/imagegen/workbenches", h.ListWorkbenches,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("List image workbenches, most recently used first"),
		swagger.WithResponseModel(ImageGenWorkbenchListResponse{}))

	router.POST("/imagegen/workbenches", h.CreateWorkbench,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Create a workbench focused on an archived image"),
		swagger.WithRequestModel(ImageGenWorkbenchCreateRequest{}),
		swagger.WithResponseModel(ImageGenWorkbenchResponse{}))

	router.GET("/imagegen/workbenches/:id", h.GetWorkbench,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Get one workbench"),
		swagger.WithPathParam("id", "string", "Workbench id"),
		swagger.WithResponseModel(ImageGenWorkbenchResponse{}))

	router.PUT("/imagegen/workbenches/:id", h.UpdateWorkbench,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Rename a workbench or change its description"),
		swagger.WithPathParam("id", "string", "Workbench id"),
		swagger.WithRequestModel(ImageGenWorkbenchUpdateRequest{}),
		swagger.WithResponseModel(ImageGenWorkbenchResponse{}))

	router.DELETE("/imagegen/workbenches/:id", h.DeleteWorkbench,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Delete a workbench (its images stay archived)"),
		swagger.WithPathParam("id", "string", "Workbench id"),
		swagger.WithResponseModel(ImageGenOKResponse{}))

	router.POST("/imagegen/workbenches/:id/items", h.AddWorkbenchItems,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Record archived images derived inside a workbench"),
		swagger.WithPathParam("id", "string", "Workbench id"),
		swagger.WithRequestModel(ImageGenWorkbenchAddItemsRequest{}),
		swagger.WithResponseModel(ImageGenWorkbenchResponse{}))

	router.DELETE("/imagegen/workbenches/:id/items/:image_id", h.RemoveWorkbenchItem,
		swagger.WithTags("imagegen"),
		swagger.WithDescription("Take an image out of a workbench (it stays archived)"),
		swagger.WithPathParam("id", "string", "Workbench id"),
		swagger.WithPathParam("image_id", "string", "Archived image id"),
		swagger.WithResponseModel(ImageGenWorkbenchResponse{}))
}
