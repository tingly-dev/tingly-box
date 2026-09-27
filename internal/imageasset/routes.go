package imageasset

import "github.com/tingly-dev/tingly-box/swagger"

// RoutePrefix is where the API lives within the group it is registered on.
const RoutePrefix = "/image-assets"

// RegisterRoutes registers the image assets API on router. The caller puts
// router behind whatever authentication it uses.
func RegisterRoutes(router *swagger.RouteGroup, h *Handler) {
	tags := swagger.WithTags("image-assets")
	id := swagger.WithPathParam("id", "string", "Asset id")

	router.GET(RoutePrefix+"/pieces", h.ListPieces, tags,
		swagger.WithDescription("List kept prompt pieces (whole prompts, terms, phrases), most recently edited first"),
		swagger.WithResponseModel(ImageAssetPiecesResponse{}))
	router.POST(RoutePrefix+"/pieces", h.SavePieces, tags,
		swagger.WithDescription("Create prompt pieces, or update the ones whose id is set, in one transaction"),
		swagger.WithRequestModel(ImageAssetSavePiecesRequest{}),
		swagger.WithResponseModel(ImageAssetPiecesResponse{}))
	router.DELETE(RoutePrefix+"/pieces/:id", h.DeletePiece, tags, id,
		swagger.WithDescription("Delete a prompt piece; pieces split from it are kept"),
		swagger.WithResponseModel(ImageAssetDeleteResponse{}))

	router.GET(RoutePrefix+"/references", h.ListReferences, tags,
		swagger.WithDescription("List kept reference images (metadata only), newest first"),
		swagger.WithResponseModel(ImageAssetReferencesResponse{}))
	router.POST(RoutePrefix+"/references", h.AddReferences, tags,
		swagger.WithDescription("Keep reference images sent as data URLs; an image already kept is returned with existing=true"),
		swagger.WithRequestModel(ImageAssetAddReferencesRequest{}),
		swagger.WithResponseModel(ImageAssetAddReferencesResponse{}))
	router.PUT(RoutePrefix+"/references/:id", h.RenameReference, tags, id,
		swagger.WithDescription("Rename a reference image"),
		swagger.WithRequestModel(ImageAssetRenameReferenceRequest{}),
		swagger.WithResponseModel(ImageAssetReferenceResponse{}))
	router.DELETE(RoutePrefix+"/references/:id", h.DeleteReference, tags, id,
		swagger.WithDescription("Delete a reference image and its file"),
		swagger.WithResponseModel(ImageAssetDeleteResponse{}))
	router.GET(RoutePrefix+"/references/:id/content", h.ReferenceContent, tags, id,
		swagger.WithDescription("Get a reference image's bytes"))
}
