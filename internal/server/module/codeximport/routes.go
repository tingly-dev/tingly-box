package codeximport

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	router.POST("/codex/import/openai", h.ImportOpenAISessions,
		swagger.WithDescription("Import existing Codex OpenAI sessions into the current custom provider by rewriting session metadata and the local state SQLite index"),
		swagger.WithTags("codex"),
		swagger.WithRequestModel(ImportOpenAISessionsRequest{}),
		swagger.WithResponseModel(ImportOpenAISessionsResponse{}),
	)
}
