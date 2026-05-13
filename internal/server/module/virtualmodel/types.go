package virtualmodel

// AvailableModelEntry describes a single virtual model registered in the
// in-process registries. It is independent of the OpenAI Model wire shape so
// the frontend can render protocol-specific badges without parsing IDs.
type AvailableModelEntry struct {
	ID       string `json:"id" example:"vm-claude-haiku"`
	Protocol string `json:"protocol" example:"anthropic"` // "anthropic" or "openai"
}

// AvailableModelsResponse is the response body of GET /vmodel/available-models.
type AvailableModelsResponse struct {
	Success bool                  `json:"success" example:"true"`
	Data    []AvailableModelEntry `json:"data"`
}
