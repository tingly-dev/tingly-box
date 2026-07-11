package fetcher

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// RegisterAll registers all built-in quota fetchers into the given registrar.
func RegisterAll(r quota.FetcherRegistrar, logger *logrus.Logger) {
	fetchers := []quota.Fetcher{
		NewAnthropicFetcher(),
		NewOpenAIFetcher(),
		NewGeminiFetcher(),
		NewCursorFetcher(),
		NewCopilotFetcher(),
		NewVertexAIFetcher(),
		NewZaiFetcher(),
		NewGLMFetcher(),
		NewKimiK2Fetcher(),
		NewOpenRouterFetcher(),
		NewMiniMaxFetcher(),
		NewMiniMaxCNFetcher(),
		NewCodexFetcher(),
	}
	for _, f := range fetchers {
		if err := r.RegisterFetcher(f); err != nil {
			logger.WithError(err).Debugf("Failed to register fetcher: %s", f.Name())
		}
	}
}
