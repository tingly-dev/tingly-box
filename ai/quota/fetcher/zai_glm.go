package fetcher

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const glmQuotaLimitURL = "https://open.bigmodel.cn/api/monitor/usage/quota/limit"

// GLMFetcher GLM (BigModel.cn) 配额获取器
// Uses: GET https://open.bigmodel.cn/api/monitor/usage/quota/limit
type GLMFetcher struct {
	logger *logrus.Logger
}

func NewGLMFetcher(logger *logrus.Logger) *GLMFetcher {
	return &GLMFetcher{logger: logger}
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
