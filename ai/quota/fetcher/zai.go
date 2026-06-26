package fetcher

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const zaiQuotaLimitURL = "https://api.z.ai/api/monitor/usage/quota/limit"

// ZaiFetcher z.ai 配额获取器
// Uses: GET https://api.z.ai/api/monitor/usage/quota/limit (API key auth)
type ZaiFetcher struct {
	logger *logrus.Logger
}

func NewZaiFetcher(logger *logrus.Logger) *ZaiFetcher {
	return &ZaiFetcher{logger: logger}
}

func (f *ZaiFetcher) Name() string                     { return "zai" }
func (f *ZaiFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeZai }
func (f *ZaiFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *ZaiFetcher) Validate(provider *ai.Provider) error {
	return validateAPIKeyProvider(provider)
}

func (f *ZaiFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	var apiResp zaiQuotaResponse
	rawResponse, err := fetchBearerJSON(ctx, provider, zaiQuotaLimitURL, &apiResp)
	if err != nil {
		return nil, err
	}

	return buildZaiProviderUsage(provider, quota.ProviderTypeZai, rawResponse, &apiResp)
}
