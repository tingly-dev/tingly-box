package codeximport

import "github.com/tingly-dev/tingly-box/pkg/swagger"

func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.POST("/codex/import/openai", handler.ImportOpenAISessions,
		swagger.WithDescription("Import existing Codex OpenAI sessions into the current custom provider by rewriting session metadata and the local state SQLite index"),
		swagger.WithTags("codex"),
		swagger.WithRequestModel(ImportOpenAISessionsRequest{}),
		swagger.WithResponseModel(ImportOpenAISessionsResponse{}),
	)
}
