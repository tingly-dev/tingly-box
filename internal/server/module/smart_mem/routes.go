package smart_mem

import (
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes wires smart_mem endpoints onto the apiV1 router group.
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// POST /api/v1/smart_mem - persist a JSON document, return UUID + preview.
	router.POST("/smart_mem", handler.Persist,
		swagger.WithTags("smart_mem"),
		swagger.WithDescription("Persist a JSON document to file storage and return its UUID handle and a short description preview."),
		swagger.WithRequestModel(PersistRequest{}),
		swagger.WithResponseModel(PersistResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request"},
			swagger.ErrorResponseConfig{Code: 503, Message: "smart_mem store not available"},
		),
	)

	// GET /api/v1/smart_mem/:uuid - retrieve a previously persisted document.
	router.GET("/smart_mem/:uuid", handler.Retrieve,
		swagger.WithTags("smart_mem"),
		swagger.WithDescription("Retrieve the persisted JSON document for a given UUID. Returned as application/json with the original payload shape."),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "Document not found"},
			swagger.ErrorResponseConfig{Code: 503, Message: "smart_mem router not available"},
		),
	)
}
