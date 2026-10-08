package anthropic

import (
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/promptcache"
)

// PromptCacheModel is the Anthropic-protocol vmodel.PromptCacheModel. The
// virtual server replaces it per request with a MockModel carrying the
// simulation's report, so its own Handle methods only serve callers that
// bypass the server (they reply with the description).
type PromptCacheModel struct {
	*MockModel
	sim *promptcache.Simulator
}

var _ vmodel.PromptCacheModel = (*PromptCacheModel)(nil)

// NewPromptCacheModel returns the prompt-cache simulation model with its own
// simulator.
func NewPromptCacheModel() *PromptCacheModel {
	return &PromptCacheModel{
		MockModel: NewMockModel(&MockModelConfig{
			ID:          vmodel.PromptCacheModelID,
			Name:        "Virtual Prompt Cache",
			Description: vmodel.PromptCacheModelDescription,
			Content:     vmodel.PromptCacheModelDescription,
		}),
		sim: promptcache.New(),
	}
}

// PromptCache implements vmodel.PromptCacheModel.
func (m *PromptCacheModel) PromptCache() *promptcache.Simulator { return m.sim }
