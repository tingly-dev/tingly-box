package fetcher

import (
	"context"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const glmQuotaLimitURL = "https://open.bigmodel.cn/api/monitor/usage/quota/limit"

// GLMFetcher retrieves GLM (BigModel.cn) quota data.
// Uses: GET https://open.bigmodel.cn/api/monitor/usage/quota/limit
type GLMFetcher struct{}

func NewGLMFetcher() *GLMFetcher {
	return &GLMFetcher{}
}

func (f *GLMFetcher) Name() string                     { return "glm" }
func (f *GLMFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeGLM }
func (f *GLMFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *GLMFetcher) Validate(provider *ai.Provider) error {
	return validateAPIKeyProvider(provider)
}

func (f *GLMFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	var apiResp zaiQuotaResponse
	rawResponse, err := fetchBearerJSON(ctx, provider, glmQuotaLimitURL, &apiResp)
	if err != nil {
		return nil, err
	}

	return buildZaiProviderUsage(provider, quota.ProviderTypeGLM, rawResponse, &apiResp)
}
