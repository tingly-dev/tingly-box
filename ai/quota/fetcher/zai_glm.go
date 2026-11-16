package fetcher

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// GLMFetcher retrieves GLM (BigModel.cn) quota data.
// Uses: GET https://open.bigmodel.cn/api/monitor/usage/quota/limit
type GLMFetcher struct {
	baseURL string // empty → production host; override in tests only
}

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
	rawResponse, err := fetchBearerJSON(ctx, provider, endpoint(f.baseURL, glmRoot, "/api/monitor/usage/quota/limit"), &apiResp)
	if err != nil {
		return nil, err
	}

	usage, err := buildZaiProviderUsage(provider, quota.ProviderTypeGLM, rawResponse, &apiResp)
	if err != nil {
		return nil, err
	}

	// Reset cards are extra: a list that cannot be read must never take the
	// windows down with it, but it is logged so a changed contract is seen.
	cards, cardsErr := f.listResetCards(ctx, provider)
	if cardsErr != nil {
		logrus.WithError(cardsErr).WithField("provider_uuid", provider.UUID).Warn("glm reset cards not read")
		return usage, nil
	}
	addGLMResetCards(usage, cards)
	return usage, nil
}
